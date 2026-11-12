// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package acquisition

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/autobrr/autobrr/pkg/sabnzbd"

	"github.com/rs/zerolog"
)

type repository interface {
	Acquisition(context.Context, string) (Job, error)
	BindAcquisition(context.Context, Job, string, string) error
	DismissAcquisition(context.Context, string) error
	ReserveAcquisition(context.Context, Job) (Job, bool, error)
	SaveAcquisition(context.Context, Job) error
	Acquisitions(context.Context, bool) ([]Job, error)
}
type candidates interface {
	CandidateFor(string, string) (search.Candidate, error)
	Sources(context.Context) ([]search.Source, error)
}
type catalogueReader interface {
	Event(context.Context, string) (catalogue.Event, error)
	MatchingEvents(context.Context) ([]catalogue.Event, error)
	Settings(context.Context) (*catalogue.Settings, error)
}
type completionHandoff interface {
	CompletedDownload(context.Context, Job) error
}

// Accepted distinguishes an idempotent duplicate from a new explicit submission.
type Accepted struct {
	Job      Job  `json:"job"`
	Existing bool `json:"existing"`
}

// Service owns manual submission and read-only remote reconciliation in the app lifecycle.
type Service struct {
	repo       repository
	candidates candidates
	catalogue  catalogueReader
	gateway    gateway
	handoff    completionHandoff
	log        zerolog.Logger
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
}

// SetCompletionHandoff connects confirmed SAB completions to native processing.
func (s *Service) SetCompletionHandoff(h completionHandoff) { s.handoff = h }

// New constructs manual acquisition without starting requests or background work.
func New(r repository, c candidates, events catalogueReader, g gateway, log zerolog.Logger) *Service {
	return &Service{repo: r, candidates: c, catalogue: events, gateway: g, log: log}
}

// Clients lists existing enabled SAB clients without exposing configuration secrets.
func (s *Service) Clients(ctx context.Context) ([]ClientOption, error) { return s.gateway.Clients(ctx) }

// Jobs reads local tracking history without contacting SABnzbd.
func (s *Service) Jobs(ctx context.Context) ([]Job, error) { return s.repo.Acquisitions(ctx, false) }

// Remove hides a card while retaining reconciliation history and duplicate protection.
func (s *Service) Remove(ctx context.Context, id string) error {
	if len(id) == 0 || len(id) > 128 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid download ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repo.DismissAcquisition(ctx, id)
}

// Submit revalidates a selected result, reserves durably and sends it at most once.
func (s *Service) Submit(ctx context.Context, input Submit) (*Accepted, error) {
	ctx, release, ok := maintenance.Application.EnterContext(ctx)
	if !ok {
		return nil, errors.New("application is preparing an update")
	}
	defer release()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(input.Category) > 128 || strings.IndexFunc(input.Category, unicode.IsControl) >= 0 || input.ClientID < 1 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "invalid download client or category")
	}
	c, err := s.candidates.CandidateFor(input.CandidateID, input.Actor)
	if err != nil {
		return nil, err
	}
	sources, err := s.candidates.Sources(ctx)
	if err != nil {
		return nil, errors.New("could not verify indexer connection")
	}
	enabled := false
	for _, source := range sources {
		if source.ID == c.FeedID && source.Type == "NEWZNAB" && source.ConfigKey == c.ConfigKey {
			enabled = true
		}
	}
	if !enabled {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "indexer connection is disabled; search again")
	}
	event, err := s.catalogue.Event(ctx, c.EventID)
	if err != nil {
		return nil, err
	}
	all, err := s.catalogue.MatchingEvents(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.catalogue.Settings(ctx)
	if err != nil {
		return nil, err
	}
	preview := matching.MatchWithTerms(c.Title, all, settings.MainCardTerms)
	manual := strings.TrimSpace(input.ManualReason) != ""
	if manual && (len(strings.TrimSpace(input.ManualReason)) < 8 || len(input.ManualReason) > 500 || strings.IndexFunc(input.ManualReason, unicode.IsControl) >= 0 || input.EventRevision != event.Revision || input.SettingsRevision != settings.Revision) {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "manual assignment requires a reason and current event/settings revisions; search again if they changed")
	}
	if event.Quality == nil || !matching.CheckQuality(preview.Quality, event.Quality.Effective).Matched {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "resolution does not match current preferences")
	}
	if manual && event.Values.Status != "scheduled" && event.Values.Status != "completed" {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "event status requires catalogue review before downloading")
	}
	if !manual && (preview.Status != "matched" || len(preview.Candidates) != 1 || preview.Candidates[0].ID != event.ID || len(preview.Components.Values) != 1 || preview.Components.Values[0] != c.Component || (c.Component == "main_card" && preview.MainCard.Status == "excluded") || !matching.CheckQuality(preview.Quality, event.Quality.Effective).Matched) {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "event or matching preferences changed; search again before downloading")
	}
	target, client, err := s.gateway.Target(ctx, input.ClientID)
	if err != nil {
		if input.Automated {
			return nil, ErrAutomaticDeferred
		}
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "configured SABnzbd client is unavailable")
	}
	resolution, mediaSource := "", ""
	codecs := []string{}
	for _, value := range preview.Quality.Values {
		switch value {
		case "2160p", "1080p", "1080i", "720p", "480p":
			resolution = value
		case "WEB-DL", "WEBRip", "HDTV":
			mediaSource = value
		case "H.264", "H.265", "AV1":
			codecs = append(codecs, value)
		}
	}
	decision := &Decision{Automated: input.Automated, Manual: manual, Reason: strings.TrimSpace(input.ManualReason), Actor: input.Actor, At: time.Now().UTC().Format(time.RFC3339Nano), EventID: event.ID, Component: c.Component, EventRevision: event.Revision, SettingsRevision: settings.Revision, Automatic: preview}
	j := Job{Decision: decision, EventRevision: event.Revision, SettingsRevision: event.Quality.SettingsRevision, ID: c.ID, EventID: c.EventID, Component: c.Component, Title: c.Title, Source: c.Source, Indexer: c.Indexer, Size: c.Size, Resolution: resolution, MediaSource: mediaSource, Codec: strings.Join(codecs, ","), ClientID: input.ClientID, ClientName: target.Name, Category: strings.TrimSpace(input.Category), Key: c.Key, Endpoint: target.Endpoint, Actor: input.Actor, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	j, created, err := s.repo.ReserveAcquisition(ctx, j)
	if err != nil {
		if errors.Is(err, catalogue.ErrInvalidEdit) {
			return nil, err
		}
		if errors.Is(err, catalogue.ErrRevisionConflict) {
			if input.Automated {
				return nil, err
			}
			return nil, errors.Wrap(catalogue.ErrInvalidEdit, "event or preferences changed; search again before downloading")
		}
		return nil, errors.New("could not safely reserve download; nothing was submitted")
	}
	if !created {
		return &Accepted{Job: j, Existing: true}, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	response, sendErr := client.AddFromUrl(requestCtx, sabnzbd.AddNzbRequest{Url: c.URL, Category: j.Category, Name: c.Title + " UFCM-" + j.ID})
	j.State = "uncertain"
	j.Message = "Submission outcome uncertain. Reconciliation will check SABnzbd; no automatic resubmission."
	if errors.Is(sendErr, sabnzbd.ErrRequestRejected) {
		j.State = "failed"
		j.Message = "SABnzbd rejected the request. Check its connection, then run a fresh search to retry"
	}
	if sendErr == nil && response != nil {
		if response.Status && len(response.NzoIDs) == 1 && len(response.NzoIDs[0]) > 0 && len(response.NzoIDs[0]) <= 256 {
			j.State = "queued"
			j.RemoteID = response.NzoIDs[0]
			j.Message = "Sent to SABnzbd; awaiting queue status"
		} else if !response.Status && response.ErrorMsg != "" {
			j.State = "failed"
			j.Message = "SABnzbd rejected the submission. Check its category/connection, then run a fresh search to retry"
		}
	}
	persistCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if err := s.repo.SaveAcquisition(persistCtx, j); err != nil {
		j.State = "uncertain"
		j.Message = "Submission state could not be saved. The durable reservation prevents another push; check Downloads."
	} else {
		j.Revision++
	}
	return &Accepted{Job: j}, nil
}

// Start begins bounded queue/history reconciliation for tracked records only.
func (s *Service) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			if err := s.Reconcile(runCtx); err != nil && runCtx.Err() == nil {
				s.log.Warn().Msg("UFC download reconciliation unavailable")
			}
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

// Stop cancels reconciliation and joins it before database closure.
func (s *Service) Stop(ctx context.Context) error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Reconcile reads remote state without ever retrying submission or touching media.
func (s *Service) Reconcile(ctx context.Context) error {
	ctx, release, ok := maintenance.Application.EnterContext(ctx)
	if !ok {
		return nil
	}
	defer release()
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	jobs, err := s.repo.Acquisitions(ctx, true)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		target, client, err := s.gateway.Target(ctx, j.ClientID)
		if err != nil || target.Endpoint != j.Endpoint {
			j.Message = "Saved SABnzbd endpoint is unavailable or changed; check its settings"
			if err := s.repo.SaveAcquisition(ctx, j); err != nil {
				return err
			}
			continue
		}
		remote, err := client.Lookup(ctx, j.RemoteID, "UFCM-"+j.ID)
		if err != nil {
			j.Message = "SABnzbd queue/history could not be read; retaining the last known state"
		} else if remote.Ambiguous {
			j.State = "needs_review"
			j.Message = "Multiple remote jobs share the submission marker; manual review required"
		} else if !remote.Found {
			if j.RemoteID == "" {
				j.State = "uncertain"
			} else {
				j.State = "missing"
			}
			j.Message = "Job not found in SABnzbd queue/history; not assumed complete and not resubmitted"
		} else {
			j = applyRemote(j, remote)
		}
		if err := s.repo.SaveAcquisition(ctx, j); err != nil {
			return err
		}
		if j.State == "completed" && s.handoff != nil {
			if err := s.handoff.CompletedDownload(ctx, j); err != nil {
				s.log.Warn().Err(err).Str("acquisition_id", j.ID).Msg("UFC processing handoff needs review")
			}
		}
	}
	return nil
}

func applyRemote(j Job, remote sabnzbd.Job) Job {
	j.RemoteID = remote.ID
	j.Percent = remote.Percent
	if math.IsNaN(j.Percent) || math.IsInf(j.Percent, 0) {
		j.Percent = 0
	}
	j.Percent = max(0, min(100, j.Percent))
	j.Message = ""
	switch strings.ToLower(remote.State) {
	case "completed":
		if remote.History {
			j.State = "completed"
			j.Percent = 100
			j.Storage = remote.Storage
			j.Message = "SABnzbd reports completion"
		} else {
			j.State = "post_processing"
			j.Message = "Awaiting completed history entry"
		}
	case "failed":
		if remote.History {
			j.State = "failed"
			j.Message = "SABnzbd reports failure; inspect its history"
		} else {
			j.State = "needs_review"
			j.Message = "Awaiting definitive SAB failure history; reservation retained"
		}
	case "downloading", "fetching":
		j.State = "downloading"
	case "paused":
		j.State = "paused"
	case "queued", "propagating":
		j.State = "queued"
	case "quickcheck", "verifying", "repairing", "extracting", "moving", "running":
		j.State = "post_processing"
	default:
		j.State = "needs_review"
		j.Message = "Unrecognised remote state; manual review required"
	}
	return j
}

// SubmitAutomatic enters the existing submission path with server-owned candidate evidence.
func (s *Service) SubmitAutomatic(ctx context.Context, token string, clientID int32, category string) (*Accepted, error) {
	return s.Submit(ctx, Submit{CandidateID: token, ClientID: clientID, Category: category, Actor: search.AutomationActor, Automated: true})
}
