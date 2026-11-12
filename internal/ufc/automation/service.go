// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/internal/ufc/processing"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/rs/zerolog"
)

type repository interface {
	AutomationSettings(context.Context) (*Settings, error)
	SaveAutomationSettings(context.Context, Settings) (*Settings, error)
	AutomationEvents(context.Context, time.Time) ([]catalogue.Event, error)
	EnqueueAutomationItem(context.Context, Item) error
	AutomationItems(context.Context, time.Time) ([]Item, error)
	FinishAutomationItem(context.Context, string, string, string, time.Time) error
	AutomationNotices(context.Context, time.Time) ([]Notice, error)
	FinishAutomationNotice(context.Context, string, bool, time.Time) error
	ProcessingSettings(context.Context) (*processing.Settings, error)
}
type catalogueReader interface {
	MatchingEvents(context.Context) ([]catalogue.Event, error)
	Settings(context.Context) (*catalogue.Settings, error)
}
type selector interface {
	Sources(context.Context) ([]search.Source, error)
	RememberFeedCandidate(string, string, search.Source, feed.SearchItem) string
}
type submitter interface {
	SubmitAutomatic(context.Context, string, int32, string) (*acquisition.Accepted, error)
}
type notifier interface {
	Deliver(context.Context, domain.NotificationPayload) error
}

// Service orchestrates existing services and owns no feed scheduler or media operations.
type Service struct {
	repo          repository
	catalogue     catalogueReader
	search        selector
	acquisition   submitter
	notifications notifier
	log           zerolog.Logger
	cancel        context.CancelFunc
	done          chan struct{}
}

// New connects the existing acquisition and notification paths without starting work.
func New(repo repository, catalogue catalogueReader, search selector, acquisition submitter, notifications notifier, log zerolog.Logger) *Service {
	return &Service{repo: repo, catalogue: catalogue, search: search, acquisition: acquisition, notifications: notifications, log: log}
}

// Settings reports persistent enablement only.
func (s *Service) Settings(ctx context.Context) (*Settings, error) {
	return s.repo.AutomationSettings(ctx)
}

// SaveSettings persists explicit enablement without running remote requests.
func (s *Service) SaveSettings(ctx context.Context, input Settings) (*Settings, error) {
	return s.repo.SaveAutomationSettings(ctx, input)
}

func eligible(title string, events, active []catalogue.Event, terms []string) (catalogue.Event, string, bool) {
	p := matching.MatchWithTerms(title, events, terms)
	if p.Status != "matched" || len(p.Candidates) != 1 || len(p.Components.Values) != 1 {
		return catalogue.Event{}, "", false
	}
	component := p.Components.Values[0]
	if component == "main_card" && p.MainCard.Status == "excluded" {
		return catalogue.Event{}, "", false
	}
	for _, event := range active {
		if event.ID == p.Candidates[0].ID && Active(event, time.Now()) && event.Components != nil && slices.Contains(event.Components.Wanted, component) && event.Quality != nil && len(event.Review) == 0 && matching.CheckQuality(p.Quality, event.Quality.Effective).Matched {
			return event, component, true
		}
	}
	return catalogue.Event{}, "", false
}

// ObserveFeed durably records confident candidates before a feed page is acknowledged.
func (s *Service) ObserveFeed(ctx context.Context, f *domain.Feed, items []feed.SearchItem) error {
	settings, err := s.repo.AutomationSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled || !f.Enabled || f.Type != string(domain.FeedTypeNewznab) || (f.Indexer.ID != 0 && !f.IndexerEnabled) {
		return nil
	}
	active, err := s.repo.AutomationEvents(ctx, time.Now())
	if err != nil {
		return err
	}
	if len(active) == 0 {
		return nil
	}
	events, err := s.catalogue.MatchingEvents(ctx)
	if err != nil {
		return err
	}
	preferences, err := s.catalogue.Settings(ctx)
	if err != nil {
		return err
	}
	source := search.SourceForFeed(*f)
	for _, item := range items {
		if len(item.Title) > 2048 || len(item.URL) > 8192 || len(item.Reference) > 8192 || item.URL == "" {
			continue
		}
		event, component, ok := eligible(item.Title, events, active, preferences.MainCardTerms)
		if !ok {
			continue
		}
		key := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s", f.ID, source.ConfigKey, event.ID, component, item.Reference)))
		if err := s.repo.EnqueueAutomationItem(ctx, Item{ID: hex.EncodeToString(key[:]), ConfigKey: source.ConfigKey, EventID: event.ID, Component: component, FeedID: f.ID, Release: item}); err != nil {
			return err
		}
	}
	return nil
}

func backoff(attempts int) time.Duration {
	return time.Minute * time.Duration(1<<min(max(attempts, 0), 6))
}

func (s *Service) acquire(ctx context.Context, now time.Time) error {
	active, err := s.repo.AutomationEvents(ctx, now)
	if err != nil {
		return err
	}
	settings, err := s.repo.AutomationSettings(ctx)
	if err != nil {
		return err
	}
	if settings.Enabled && len(active) > 0 {
		items, err := s.repo.AutomationItems(ctx, now)
		if err != nil {
			return err
		}
		if len(items) > 0 {
			sources, err := s.search.Sources(ctx)
			if err != nil {
				return err
			}
			events, err := s.catalogue.MatchingEvents(ctx)
			if err != nil {
				return err
			}
			preferences, err := s.catalogue.Settings(ctx)
			if err != nil {
				return err
			}
			target, err := s.repo.ProcessingSettings(ctx)
			if err != nil {
				return err
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].FeedID != items[j].FeedID {
					return items[i].FeedID < items[j].FeedID
				}
				a, b := strings.ToLower(items[i].Release.Title), strings.ToLower(items[j].Release.Title)
				if a != b {
					return a < b
				}
				return items[i].ID < items[j].ID
			})
			for _, item := range items[:min(len(items), 12)] {
				if err := ctx.Err(); err != nil {
					return err
				}
				event, component, ok := eligible(item.Release.Title, events, active, preferences.MainCardTerms)
				var source search.Source
				for _, value := range sources {
					if value.ID == item.FeedID && value.ConfigKey == item.ConfigKey && value.Type == string(domain.FeedTypeNewznab) {
						source = value
					}
				}
				state, message := "rejected", "Current event, component, quality or feed settings no longer permit this candidate"
				if ok && event.ID == item.EventID && component == item.Component && source.ID != 0 {
					token := s.search.RememberFeedCandidate(event.ID, component, source, item.Release)
					if token != "" {
						accepted, submitErr := s.acquisition.SubmitAutomatic(ctx, token, target.SABClientID, target.SABCategory)
						if submitErr == nil && accepted != nil {
							state, message = "submitted", "Existing acquisition reservation accepted"
						} else if submitErr != nil && (errors.Is(submitErr, catalogue.ErrRevisionConflict) || !errors.Is(submitErr, catalogue.ErrInvalidEdit)) {
							state, message = "pending", "Acquisition unavailable; retrying after backoff"
						}
					}
				}

				if err := s.repo.FinishAutomationItem(ctx, item.ID, state, message, now.Add(backoff(item.Attempts))); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// RunOnce resumes acquisitions and independent notification delivery.
func (s *Service) RunOnce(ctx context.Context) error {
	ctx, release, ok := maintenance.Application.EnterContext(ctx)
	if !ok {
		return nil
	}
	defer release()
	now := time.Now().UTC()
	acquisitionErr := s.acquire(ctx, now)
	notices, err := s.repo.AutomationNotices(ctx, now)
	if err != nil {
		return err
	}
	for _, notice := range notices {
		deliveryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		payload := domain.NotificationPayload{Event: domain.NotificationEvent(notice.Event), Subject: "UFC Manager", Message: notice.Message, ReleaseName: notice.Title, Timestamp: now, Sender: "UFC Manager"}
		var deliveryErr error
		if payload.Event == domain.NotificationEventAppUpdateAvailable {
			deliveryErr = json.Unmarshal([]byte(notice.Message), &payload)
			payload.Timestamp = now
			payload.Sender = "UFC Manager"
			payload.Event = domain.NotificationEventAppUpdateAvailable
		}
		if deliveryErr == nil {
			deliveryErr = s.notifications.Deliver(deliveryCtx, payload)
		}
		cancel()
		if err := s.repo.FinishAutomationNotice(ctx, notice.ID, deliveryErr == nil, now.Add(backoff(notice.Attempts))); err != nil {
			return err
		}
	}
	return acquisitionErr
}

// Start begins bounded durable work, leaving scheduling to the existing feed service.
func (s *Service) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			if err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn().Msg("UFC automation work is waiting for recovery")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

// Stop joins the worker before database shutdown.
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
