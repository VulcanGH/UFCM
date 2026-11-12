// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package search previews manual event searches without creating acquisitions.
package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/pkg/errors"
)

type catalogueReader interface {
	Event(context.Context, string) (catalogue.Event, error)
	MatchingEvents(context.Context) ([]catalogue.Event, error)
	Settings(context.Context) (*catalogue.Settings, error)
}
type feedSearcher interface {
	Find(context.Context) ([]domain.Feed, error)
	Search(context.Context, int, string) (*feed.SearchPage, error)
}

// Source identifies an existing enabled search connection without exposing credentials.
type Source struct {
	ConfigKey string `json:"-"`
	Indexer   string `json:"-"`
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

// Request selects an event, component and explicitly chosen existing connections.
type Request struct {
	Actor     string `json:"-"`
	EventID   string `json:"-"`
	Component string `json:"component"`
	FeedIDs   []int  `json:"feed_ids"`
	Query     string `json:"query"`
}

// Result explains one release in the context of the selected event/component.
type Result struct {
	PublishedAt  *time.Time            `json:"published_at"`
	CandidateID  string                `json:"candidate_id,omitempty"`
	QualityCheck matching.QualityCheck `json:"quality_check"`
	Matched      bool                  `json:"matched"`
	Source       Source                `json:"source"`
	Title        string                `json:"title"`
	Size         uint64                `json:"size"`
	Assessment   string                `json:"assessment"`
	Reasons      []string              `json:"reasons"`
	Match        matching.Preview      `json:"match"`
}

// SourceResult exposes partial failures and truncation independently per connection.
type SourceResult struct {
	Source Source `json:"source"`
	Error  string `json:"error,omitempty"`
	More   bool   `json:"more"`
	Count  int    `json:"count"`
}

// Response is a transient preview, never a download submission.
type Response struct {
	EventRevision    int64          `json:"event_revision"`
	SettingsRevision int64          `json:"settings_revision"`
	Resolutions      []string       `json:"resolutions"`
	EventID          string         `json:"event_id"`
	Component        string         `json:"component"`
	Query            string         `json:"query"`
	Results          []Result       `json:"results"`
	Sources          []SourceResult `json:"sources"`
}

// Service reuses catalogue and feed services without background work.
type Service struct {
	mu         sync.Mutex
	candidates map[string]Candidate
	catalogue  catalogueReader
	feeds      feedSearcher
}

// New returns an event search service using existing configured connections.
func New(c catalogueReader, f feedSearcher) *Service {
	return &Service{catalogue: c, feeds: f, candidates: map[string]Candidate{}}
}

// Sources lists enabled Newznab/Torznab connections only.
func (s *Service) Sources(ctx context.Context) ([]Source, error) {
	feeds, err := s.feeds.Find(ctx)
	if err != nil {
		return nil, errors.New("could not read configured search connections")
	}
	result := []Source{}
	for _, f := range feeds {
		if f.Enabled && (f.Indexer.ID == 0 || f.IndexerEnabled) && (f.Type == string(domain.FeedTypeNewznab) || f.Type == string(domain.FeedTypeTorznab)) {
			result = append(result, SourceForFeed(f))
		}
	}
	return result, nil
}

// Search reads one bounded page per selected source and explains all candidates.
func (s *Service) Search(ctx context.Context, input Request) (*Response, error) {
	if err := catalogue.ValidateOverrides(map[string]bool{input.Component: true}); err != nil || input.Component == "" {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose a supported component")
	}
	if len(input.FeedIDs) < 1 || len(input.Query) > 160 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "select configured connections and a query up to 160 bytes")
	}
	event, err := s.catalogue.Event(ctx, input.EventID)
	if err != nil {
		return nil, err
	}
	if event.Quality == nil {
		return nil, errors.New("event quality preferences are unavailable")
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		query = event.Values.Name
		for _, a := range event.Aliases {
			if a.Current && a.Verified && a.Namespace == "ufc.fight_night" && event.Values.Type == "fight_night" {
				query = "UFC Fight Night " + a.Value
				break
			}
			if a.Current && a.Verified && a.Namespace == "ufc.numbered" && event.Values.Type == "numbered" {
				query = "UFC " + a.Value
				break
			}
		}
	}
	if len(query) > 160 || query == "" {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "provide a shorter search query")
	}
	sources, err := s.Sources(ctx)
	if err != nil {
		return nil, err
	}
	chosen := []Source{}
	seen := map[int]bool{}
	for _, id := range input.FeedIDs {
		if seen[id] {
			return nil, errors.Wrap(catalogue.ErrInvalidEdit, "duplicate connection")
		}
		seen[id] = true
		found := false
		for _, source := range sources {
			if source.ID == id {
				chosen = append(chosen, source)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Wrap(catalogue.ErrInvalidEdit, "a selected connection is disabled or unavailable")
		}
	}
	settings, err := s.catalogue.Settings(ctx)
	if err != nil {
		return nil, err
	}
	events, err := s.catalogue.MatchingEvents(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response := &Response{EventRevision: event.Revision, SettingsRevision: settings.Revision, Resolutions: event.Quality.Effective, EventID: event.ID, Component: input.Component, Query: query, Results: []Result{}, Sources: []SourceResult{}}
	for _, source := range chosen {
		page, err := s.feeds.Search(ctx, source.ID, query)
		state := SourceResult{Source: source}
		if err != nil {
			state.Error = "Search failed or timed out. Check the existing connection settings and search support."
			response.Sources = append(response.Sources, state)
			continue
		}
		state.More = page.More
		state.Count = len(page.Items)
		for _, item := range page.Items {
			if len(response.Results) >= 250 || ctx.Err() != nil {
				state.More = true
				state.Error = "Preview result or time limit reached. Narrow the query or select fewer connections."
				break
			}
			if len(item.Title) > 1024 || strings.TrimSpace(item.Title) == "" {
				state.More = true
				continue
			}
			preview := matching.MatchWithTerms(item.Title, events, settings.MainCardTerms)
			qualityCheck := matching.CheckQuality(preview.Quality, event.Quality.Effective)
			reasons := []string{}
			if preview.Status != "matched" || len(preview.Candidates) != 1 || preview.Candidates[0].ID != event.ID {
				reasons = append(reasons, "Event identity is uncertain or differs from the selected event")
			}
			if input.Component == "main_card" && preview.MainCard.Status == "excluded" {
				reasons = append(reasons, "Excluded for Main Card: "+strings.Join(preview.MainCard.MatchedTerms, ", "))
			}
			if len(preview.Components.Values) != 1 || preview.Components.Values[0] != input.Component {
				reasons = append(reasons, "Component is unknown, bundled or differs from the selected component")
			}
			if !qualityCheck.Matched {
				reasons = append(reasons, qualityCheck.Reason)
			}
			assessment := "match"
			if len(reasons) > 0 {
				assessment = "needs review"
			}
			candidateID := ""
			if source.Type == string(domain.FeedTypeNewznab) {
				candidateID = s.remember(input, event, source, item)
			}
			response.Results = append(response.Results, Result{PublishedAt: item.PublishedAt, CandidateID: candidateID, QualityCheck: qualityCheck, Matched: len(reasons) == 0, Source: source, Title: item.Title, Size: item.Size, Assessment: assessment, Reasons: reasons, Match: preview})
		}
		response.Sources = append(response.Sources, state)
	}
	return response, nil
}

// SourceForFeed fingerprints a configured connection without exposing its credentials.
func SourceForFeed(f domain.Feed) Source {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%t|%t|%d", f.URL, f.ApiKey, f.Cookie, f.UserAgent, f.TLSSkipVerify, f.UseProxy, f.ProxyID)))
	return Source{ConfigKey: hex.EncodeToString(hash[:]), ID: f.ID, Name: f.Name, Type: f.Type, Indexer: f.Indexer.Identifier}
}
