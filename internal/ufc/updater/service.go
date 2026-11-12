// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"

	"github.com/Masterminds/semver/v3"
)

// Service owns authenticated release discovery and durable update intent.
type Service struct {
	mu                                      sync.Mutex
	repo                                    repository
	client                                  *Client
	current, driver, database, config, root string
	shutdown                                func()
	safe                                    func(context.Context, string) error
	cancel                                  context.CancelFunc
	done                                    chan struct{}
	wake                                    chan struct{}
	drainStarted                            time.Time
	checking                                atomic.Bool
}

// New keeps unconfigured and unmanaged builds usable without trusting unsigned releases.
func New(repo repository, current, driver, database, config string, safe func(context.Context, string) error) *Service {
	if driver == "sqlite" {
		if absolute, err := filepath.Abs(database); err == nil {
			database = absolute
		}
	}
	if absolute, err := filepath.Abs(config); err == nil {
		config = absolute
	}
	s := &Service{repo: repo, current: current, driver: driver, database: database, config: config, root: ManagedRoot(), safe: safe, wake: make(chan struct{}, 1)}
	if trust, err := ProductionTrust(); err == nil {
		s.client = NewClient(trust)
	}
	return s
}

// SetShutdown connects the desktop host's orderly exit to a verified handover request.
func (s *Service) SetShutdown(shutdown func()) { s.mu.Lock(); s.shutdown = shutdown; s.mu.Unlock() }

func (s *Service) reason() string {
	if s.driver != "sqlite" {
		return "PostgreSQL updates are installed manually"
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return "In-app installation requires Windows amd64"
	}
	if s.client == nil {
		return "The release signing key is not configured"
	}
	if s.root == "" || s.shutdown == nil {
		return "Install the launcher-based package to enable in-app updates"
	}
	if !versionPattern.MatchString(s.current) {
		return "Development builds are updated manually"
	}
	return ""
}

// Status reads saved progress without initiating network or filesystem work.
func (s *Service) Status(ctx context.Context) (Status, error) {
	status, err := s.status(ctx)
	if s.checking.Load() && status.Phase != "waiting_for_idle" && status.Phase != "quiescing" {
		status.Phase = "checking"
		status.Message = "Checking for signed releases"
	}
	return status, err
}
func (s *Service) status(ctx context.Context) (Status, error) {
	state, err := s.repo.UpdateState(ctx)
	return Status{State: state, Current: s.current, Supported: s.reason() == "", Reason: s.reason(), ReleaseURL: "https://github.com/" + Repository + "/releases"}, err
}

// Preferences changes only the automatic-install opt-in using revision protection.
func (s *Service) Preferences(ctx context.Context, revision int64, automatic bool) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.repo.UpdateState(ctx)
	if err != nil {
		return Status{}, err
	}
	if state.Revision != revision {
		return Status{}, catalogue.ErrRevisionConflict
	}
	if automatic && s.reason() != "" {
		return Status{}, fmt.Errorf("%s", s.reason())
	}
	state.AutoInstall = automatic
	if _, err = s.repo.SaveUpdateState(ctx, state); err != nil {
		return Status{}, err
	}
	return s.status(ctx)
}

// Check coalesces requests onto the joined background coordinator.
func (s *Service) Check(ctx context.Context) (Status, error) {
	if s.client == nil {
		return Status{}, fmt.Errorf("update checks unavailable: the release signing key is not configured")
	}
	s.checking.Store(true)
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return s.Status(ctx)
}

func (s *Service) check(ctx context.Context) (Status, error) {
	s.checking.Store(true)
	defer s.checking.Store(false)
	state, err := s.repo.UpdateState(ctx)
	if err != nil {
		return Status{}, err
	}
	if s.client == nil {
		return s.status(ctx)
	}
	if state.Phase == "waiting_for_idle" || state.Phase == "quiescing" {
		return s.status(ctx)
	}
	envelope, m, err := s.client.Latest(ctx, state.Highwater)
	s.checking.Store(false)
	if err != nil {
		state.Message = "Update check failed; the current version remains available"
		if errors.Is(err, errNoRelease) {
			state.Message = "No public stable release is available yet"
		}
		if state.Phase == "idle" || state.Phase == "current" || state.Phase == "check_failed" || state.Phase == "no_release" {
			state.Phase = "check_failed"
			if errors.Is(err, errNoRelease) {
				state.Phase = "no_release"
			}
		}
		_, saveErr := s.repo.SaveUpdateState(ctx, state)
		if saveErr != nil {
			return Status{}, saveErr
		}
		return s.status(ctx)
	}
	current, currentErr := semver.NewVersion(s.current)
	candidate, candidateErr := semver.NewVersion(m.Version)
	if currentErr != nil || candidateErr != nil || !candidate.GreaterThan(current) {
		state.Phase = "current"
		state.Message = "You are using the latest available version"
		if _, err = s.repo.SaveUpdateState(ctx, state); err != nil {
			return Status{}, err
		}
		return s.status(ctx)
	}
	id := ID(envelope)
	if state.Highwater == m.Sequence && state.ID != "" && state.ID != id {
		return Status{}, fmt.Errorf("release sequence was reused")
	}
	if id == state.FailedID {
		return s.status(ctx)
	}
	if state.ID == id && (state.Phase == "verified" || state.Phase == "deferred") {
		return s.status(ctx)
	}
	state.ID = id
	state.Version = m.Version
	state.Envelope = envelope
	state.Highwater = m.Sequence
	state.Phase = "available"
	state.Message = ""
	state.Notice = &domain.NotificationPayload{Event: domain.NotificationEventAppUpdateAvailable, Subject: "UFC Manager Update Available", Message: m.Version + " is available", CurrentVersion: s.current, NewVersion: m.Version, URL: "https://github.com/" + Repository + "/releases/tag/" + m.Version, Sender: "UFC Manager"}
	state, err = s.repo.SaveUpdateState(ctx, state)
	if err != nil {
		return Status{}, err
	}
	if s.reason() != "" {
		return s.status(ctx)
	}
	if s.safe != nil {
		if err = s.safe(ctx, s.root); err != nil {
			return Status{}, err
		}
	}
	state.Phase = "downloading"
	state, err = s.repo.SaveUpdateState(ctx, state)
	if err != nil {
		return Status{}, err
	}
	_, err = s.client.Stage(ctx, filepath.Join(s.root, "versions"), envelope, state.Highwater)
	if err != nil {
		state.Phase = "available"
		state.Message = "Download verification failed; the current version remains available"
	} else {
		state.Phase = "verified"
		state.Message = "Ready to update"
	}
	if _, saveErr := s.repo.SaveUpdateState(ctx, state); saveErr != nil {
		return Status{}, saveErr
	}
	return s.status(ctx)
}

// Install records one idempotent request; the coordinator waits for a safe idle boundary.
func (s *Service) Install(ctx context.Context, id string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason() != "" {
		return Status{}, fmt.Errorf("%s", s.reason())
	}
	state, err := s.repo.UpdateState(ctx)
	if err != nil {
		return Status{}, err
	}
	if id == "" || state.ID != id {
		return Status{}, fmt.Errorf("update is no longer available")
	}
	if state.Phase == "waiting_for_idle" || state.Phase == "quiescing" {
		return s.status(ctx)
	}
	if (state.Phase != "verified" && state.Phase != "deferred") || state.FailedID == id {
		return Status{}, fmt.Errorf("update is not verified")
	}
	state.Phase = "waiting_for_idle"
	state.Message = "Waiting for current work to finish"
	if _, err = s.repo.SaveUpdateState(ctx, state); err != nil {
		return Status{}, err
	}
	return s.status(ctx)
}

func (s *Service) handover(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason() != "" {
		return nil
	}
	state, err := s.repo.UpdateState(ctx)
	if err != nil {
		return err
	}
	if state.AutoInstall && state.Phase == "verified" && state.FailedID != state.ID {
		state.Phase = "waiting_for_idle"
		state, err = s.repo.SaveUpdateState(ctx, state)
		if err != nil {
			return err
		}
	}
	if state.Phase != "waiting_for_idle" {
		return nil
	}
	if s.drainStarted.IsZero() {
		s.drainStarted = time.Now()
	}
	maintenance.Application.Drain()
	if !maintenance.Application.Freeze() {
		if time.Since(s.drainStarted) >= 30*time.Minute {
			state.Phase = "deferred"
			state.Message = "Current work is still active; the update was postponed without cancelling it"
			_, err = s.repo.SaveUpdateState(ctx, state)
			maintenance.Application.Resume()
			s.drainStarted = time.Time{}
			return err
		}
		return nil
	}
	s.drainStarted = time.Time{}
	handed := false
	defer func() {
		if !handed {
			maintenance.Application.Resume()
		}
	}()
	if s.safe != nil {
		if err = s.safe(ctx, s.root); err != nil {
			return err
		}
	}
	if _, err = s.client.Trust.Verify(state.Envelope, time.Now(), state.Highwater, true); err != nil {
		return err
	}
	if err = s.client.VerifyRuntime(filepath.Join(s.root, "versions", state.ID), state.Envelope, state.Highwater); err != nil {
		return err
	}
	state.Phase = "quiescing"
	state.Message = "Restarting to install the verified update"
	if _, err = s.repo.SaveUpdateState(ctx, state); err != nil {
		return err
	}
	if err = WriteJSON(filepath.Join(s.root, "request.json"), Request{Envelope: state.Envelope, Database: s.database, Config: s.config}); err != nil {
		return err
	}
	handed = true
	s.shutdown()
	return nil
}

// Start recovers interrupted checks, suppresses failed versions and starts bounded polling.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	state, err := s.repo.UpdateState(ctx)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if state.Version != "" && strings.TrimPrefix(state.Version, "v") == strings.TrimPrefix(s.current, "v") {
		state.Phase = "completed"
		state.Message = "Update installed"
		_, err = s.repo.SaveUpdateState(ctx, state)
	} else if state.Phase == "quiescing" {
		var bootstrap Bootstrap
		if s.root != "" && ReadJSON(filepath.Join(s.root, "bootstrap-state.json"), &bootstrap) == nil && bootstrap.DeferredID == state.ID {
			state.Phase = "deferred"
			state.Message = "Update postponed because its backup could not be completed; check disk space and backup access before retrying"
		} else {
			state.Phase = "failed"
			state.FailedID = state.ID
			state.Message = "Update did not activate; the previous version was retained"
		}
		_, err = s.repo.SaveUpdateState(ctx, state)
	} else if state.Phase == "downloading" {
		state.Phase = "available"
		_, err = s.repo.SaveUpdateState(ctx, state)
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	run, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()
	go func() {
		defer close(s.done)
		check := time.NewTimer(30 * time.Second)
		defer check.Stop()
		idle := time.NewTicker(2 * time.Second)
		defer idle.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-check.C:
				_, _ = s.check(run)
				check.Reset(4 * time.Hour)
			case <-s.wake:
				_, _ = s.check(run)
			case <-idle.C:
				if err := s.handover(run); err != nil && run.Err() == nil {
					s.mu.Lock()
					state, loadErr := s.repo.UpdateState(run)
					if loadErr == nil {
						state.Phase = "failed"
						state.Message = "Update deferred: " + err.Error()
						state.FailedID = state.ID
						_, _ = s.repo.SaveUpdateState(run, state)
					}
					s.mu.Unlock()
				}
			}
		}
	}()
	return nil
}

// Stop joins the coordinator before the database closes.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Disjoint rejects updater state inside, above or equal to a media root.
func Disjoint(updateRoot, mediaRoot string) error {
	a, err := filepath.Abs(updateRoot)
	if err != nil {
		return err
	}
	b, err := filepath.Abs(mediaRoot)
	if err != nil {
		return err
	}
	a, err = resolveExistingAncestor(a)
	if err != nil {
		return err
	}
	b, err = resolveExistingAncestor(b)
	if err != nil {
		return err
	}
	a = strings.ToLower(filepath.Clean(a))
	b = strings.ToLower(filepath.Clean(b))
	sep := string(os.PathSeparator)
	if a == b || strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep) {
		return fmt.Errorf("update storage overlaps a configured media root")
	}
	return nil
}

func resolveExistingAncestor(path string) (string, error) {
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || filepath.Dir(path) == path {
			return "", fmt.Errorf("root cannot be resolved for update safety")
		}
		suffix = append(suffix, filepath.Base(path))
		path = filepath.Dir(path)
	}
}
