// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"sync"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/rs/zerolog"
)

// ErrRevisionConflict requires re-reading protected values before retrying a transaction.
var ErrRevisionConflict = errors.Sentinel("catalogue revision conflict")

// Report contains bounded diagnostics rather than a second catalogue copy.
type Report struct {
	AliasTotal         int          `json:"alias_total,omitempty"`
	AliasProcessed     int          `json:"alias_processed,omitempty"`
	DiagnosticsOmitted int          `json:"diagnostics_omitted"`
	ListRevision       int64        `json:"list_revision"`
	Discovered         int          `json:"discovered"`
	Accepted           int          `json:"accepted"`
	NeedsReview        int          `json:"needs_review"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
}

// Status exposes durable refresh health independently of source availability.
type Status struct {
	Source              string  `json:"source"`
	Attribution         string  `json:"attribution"`
	License             string  `json:"license"`
	PeriodicSyncEnabled bool    `json:"periodic_sync_enabled"`
	Running             bool    `json:"running"`
	Interrupted         bool    `json:"interrupted"`
	Events              int     `json:"events"`
	ReviewEvents        int     `json:"review_events"`
	LastAttempt         *string `json:"last_attempt"`
	LastSuccess         *string `json:"last_success"`
	LastError           string  `json:"last_error"`
	Report              Report  `json:"report"`
}

// RefreshAccepted acknowledges a metadata-only request or an existing refresh.
type RefreshAccepted struct {
	Coalesced bool `json:"coalesced"`
}

type repository interface {
	ResetCatalogueEvents(context.Context) error
	CatalogueMatchingEvents(context.Context) ([]Event, error)
	ClearCatalogueEvents(context.Context) error
	CreateCatalogueEvent(context.Context, EventCreate, string) (Event, error)
	ClaimCatalogueRefresh(context.Context, time.Time, bool) (bool, error)
	CataloguePreferences(context.Context) (*Settings, error)
	SaveCataloguePreferences(context.Context, SettingsEdit) (*Settings, error)
	LoadCatalogueEvent(context.Context, string) (Event, error)
	EditCatalogueEvent(context.Context, string, EventEdit, string) (Event, error)
	CatalogueEvents(context.Context, string) (*EventList, error)
	CatalogueStatus(context.Context) (*Status, error)
	KnownCatalogueSources(context.Context) ([]Facts, error)
	ApplyCatalogue(context.Context, *Snapshot) (Report, error)
	RecordCatalogueAttempt(context.Context, time.Time) error
	RecordCatalogueFailure(context.Context, string) error
}

// MatchingEvents reads effective local facts for the read-only matcher.
func (s *Service) MatchingEvents(ctx context.Context) ([]Event, error) {
	return s.repo.CatalogueMatchingEvents(ctx)
}

// Clear removes all local catalogue records while excluding concurrent refreshes.
func (s *Service) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return errors.Wrap(ErrInvalidEdit, "wait for the current refresh to finish before clearing events")
	}
	return s.repo.ClearCatalogueEvents(ctx)
}

// Reset removes catalogue metadata only while excluding concurrent metadata work.
func (s *Service) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return errors.Wrap(ErrInvalidEdit, "wait for the current catalogue operation to finish before resetting")
	}
	return s.repo.ResetCatalogueEvents(ctx)
}

// Events reads effective local events without contacting Wikipedia.
func (s *Service) Events(ctx context.Context) (*EventList, error) {
	return s.repo.CatalogueEvents(ctx, time.Now().Format("2006-01-02"))
}

type eventSource interface {
	Read(context.Context, []Facts) (*Snapshot, error)
}

// Service coalesces manual refreshes within the existing application lifecycle.
type Service struct {
	refreshScope string
	repo         repository
	source       eventSource
	log          zerolog.Logger
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	scheduleDone chan struct{}
	wake         chan struct{}
	running      bool
}

// New returns a catalogue service; it performs no network work at construction.
func New(repo repository, source eventSource, log zerolog.Logger) *Service {
	return &Service{repo: repo, source: source, wake: make(chan struct{}, 1), log: log.With().Str("module", "ufc.catalogue").Logger()}
}

// Start binds manual and opt-in periodic metadata work to application cancellation.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return errors.New("catalogue service already started")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	close(s.done)
	s.scheduleDone = make(chan struct{})
	go s.scheduleLoop()
	return nil
}

// Stop cancels and joins refresh work before the shared database closes.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel == nil {
		s.mu.Unlock()
		return nil
	}
	s.cancel()
	done := s.done
	scheduleDone := s.scheduleDone
	s.mu.Unlock()
	select {
	case <-scheduleDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status reads last-good local state, even while Wikipedia is unavailable.
func (s *Service) Status(ctx context.Context) (*Status, error) {
	status, err := s.repo.CatalogueStatus(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	status.Running = s.running
	s.mu.Unlock()
	if !status.Running && status.LastAttempt != nil && status.LastError == "" {
		attempt, err := time.Parse(time.RFC3339Nano, *status.LastAttempt)
		if err != nil {
			return nil, errors.Wrap(err, "invalid persisted refresh attempt")
		}
		status.Interrupted = status.LastSuccess == nil
		if status.LastSuccess != nil {
			success, err := time.Parse(time.RFC3339Nano, *status.LastSuccess)
			if err != nil {
				return nil, errors.Wrap(err, "invalid persisted refresh success")
			}
			status.Interrupted = attempt.After(success)
		}
	}
	return status, nil
}

// Refresh starts one metadata refresh and coalesces concurrent requests.
func (s *Service) Refresh(ctx context.Context) (*RefreshAccepted, error) {
	return s.startRefresh(ctx, false)
}

// RefreshScope starts an explicit upcoming update, history update or recovery rebuild.
func (s *Service) RefreshScope(ctx context.Context, scope string) (*RefreshAccepted, error) {
	if scope != "upcoming" && scope != "past" && scope != "rebuild" {
		return nil, errors.Wrap(ErrInvalidEdit, "invalid refresh scope")
	}
	return s.startScopedRefresh(ctx, false, scope)
}

func (s *Service) startRefresh(ctx context.Context, scheduled bool) (*RefreshAccepted, error) {
	return s.startScopedRefresh(ctx, scheduled, "upcoming")
}

func (s *Service) startScopedRefresh(ctx context.Context, scheduled bool, scope string) (*RefreshAccepted, error) {
	ctx, release, allowed := maintenance.Application.EnterContext(ctx)
	if !allowed {
		return nil, errors.New("application is preparing an update")
	}
	transferred := false
	defer func() {
		if !transferred {
			release()
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.ctx.Err() != nil {
		return nil, errors.New("catalogue service is not running")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.running {
		if !scheduled && s.refreshScope != scope {
			return nil, errors.Wrap(ErrInvalidEdit, "another catalogue operation is running; wait for it to finish")
		}
		return &RefreshAccepted{Coalesced: true}, nil
	}
	var claimed bool
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if scope != "upcoming" {
			err = s.repo.RecordCatalogueAttempt(ctx, time.Now().UTC())
			claimed = err == nil
			break
		}
		claimed, err = s.repo.ClaimCatalogueRefresh(ctx, time.Now().UTC(), scheduled)
		if !errors.Is(err, ErrRevisionConflict) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if !claimed {
		return &RefreshAccepted{Coalesced: true}, nil
	}
	s.running = true
	s.refreshScope = scope
	s.done = make(chan struct{})
	done := s.done
	transferred = true
	go func() {
		defer release()
		defer func() { s.mu.Lock(); s.running = false; close(done); s.mu.Unlock() }()
		workCtx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		defer cancel()
		if err := s.refreshScoped(workCtx, scope); err != nil {
			s.log.Error().Err(err).Msg("catalogue refresh failed")
			failureCtx, failureCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer failureCancel()
			if err := s.repo.RecordCatalogueFailure(failureCtx, "refresh_failed; last good catalogue retained"); err != nil {
				s.log.Error().Err(err).Msg("could not record catalogue refresh failure")
			}
		}
	}()
	return &RefreshAccepted{}, nil
}

func (s *Service) scheduleLoop() {
	defer close(s.scheduleDone)
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		_, err := s.startRefresh(ctx, true)
		cancel()
		if err != nil && s.ctx.Err() == nil {
			s.log.Error().Err(err).Msg("could not check metadata refresh schedule")
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-timer.C:
		}
	}
}

func (s *Service) refresh(ctx context.Context) error {
	return s.refreshScoped(ctx, "upcoming")
}

func (s *Service) refreshScoped(ctx context.Context, scope string) error {
	known, err := s.repo.KnownCatalogueSources(ctx)
	if err != nil {
		return err
	}
	var snapshot *Snapshot
	if source, ok := s.source.(interface {
		ReadScope(context.Context, []Facts, string) (*Snapshot, error)
	}); ok {
		snapshot, err = source.ReadScope(ctx, known, scope)
	} else {
		snapshot, err = s.source.Read(ctx, known)
	}
	if err != nil {
		return err
	}
	if err := s.applySnapshot(ctx, snapshot); err != nil {
		return err
	}
	if source, ok := s.source.(interface {
		EnrichMissing(context.Context, *Snapshot, func(context.Context, *Snapshot) error) error
	}); ok {
		return source.EnrichMissing(ctx, snapshot, s.applySnapshot)
	}
	return nil
}

func (s *Service) applySnapshot(ctx context.Context, snapshot *Snapshot) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		_, err = s.repo.ApplyCatalogue(ctx, snapshot)
		if !errors.Is(err, ErrRevisionConflict) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
}
