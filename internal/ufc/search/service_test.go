// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package search

import (
	"context"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/stretchr/testify/require"
)

type catalogueFake struct{ event catalogue.Event }

func (f catalogueFake) Event(context.Context, string) (catalogue.Event, error) { return f.event, nil }
func (f catalogueFake) MatchingEvents(context.Context) ([]catalogue.Event, error) {
	return []catalogue.Event{f.event}, nil
}

type feedsFake struct {
	expectedQuery string
	lastQuery     string
	calls         int
	all           []domain.Feed
}

func (f *feedsFake) Find(context.Context) ([]domain.Feed, error) {
	if f.all != nil {
		return f.all, nil
	}
	return []domain.Feed{{ID: 1, Name: "Enabled", Type: "NEWZNAB", Enabled: true}, {ID: 2, Type: "NEWZNAB", Enabled: false}, {ID: 3, Type: "RSS", Enabled: true}, {ID: 4, Name: "Failure", Type: "TORZNAB", Enabled: true}}, nil
}
func (f *feedsFake) Search(_ context.Context, id int, query string) (*feed.SearchPage, error) {
	f.calls++
	f.lastQuery = query
	if id == 4 {
		return nil, errors.New("credential-bearing upstream error must stay private")
	}
	expected := f.expectedQuery
	if expected == "" {
		expected = "UFC 300"
	}
	if query != expected {
		return nil, errors.New("unexpected query")
	}
	return &feed.SearchPage{Items: []feed.SearchItem{{Title: "UFC.300.Main.Card.1080p", Size: 1234, URL: "http://mock.invalid/nzb?apikey=fake-private", Reference: "stable-guid"}, {Title: "UFC.300.Pre-Show.1080p"}, {Title: "UFC.300.Prelims.1080p"}, {Title: "UFC.999.Main.Card.1080p"}, {Title: "UFC.300.1080p.WEB-DL"}, {Title: "UFC.300.720p"}, {Title: "UFC.300.1080i"}, {Title: "UFC.300.WEB-DL"}, {Title: "UFC.300.1080p.720p"}, {Title: "UFC.300.Early.Prelims.1080p"}}, More: true}, nil
}

func TestFightNightSearchQuery(t *testing.T) {
	for _, tc := range []struct {
		name, custom, namespace string
		current, verified       bool
		want                    string
	}{
		{"verified number", "", "ufc.fight_night", true, true, "UFC Fight Night 293"},
		{"manual query", "UFC FN 293", "ufc.fight_night", true, true, "UFC FN 293"},
		{"unverified number", "", "ufc.fight_night", true, false, "UFC Fight Night: Cortes-Acosta vs. Walker"},
		{"former number", "", "ufc.fight_night", false, true, "UFC Fight Night: Cortes-Acosta vs. Walker"},
		{"different numbering", "", "ufc.vegas", true, true, "UFC Fight Night: Cortes-Acosta vs. Walker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &feedsFake{expectedQuery: tc.want}
			e := catalogue.Event{ID: "night", Values: catalogue.Values{Name: "UFC Fight Night: Cortes-Acosta vs. Walker", Type: "fight_night", Status: "completed"}, Source: catalogue.Facts{OverallSequence: "797"}, Aliases: []catalogue.Alias{{Namespace: tc.namespace, Value: "293", Current: tc.current, Verified: tc.verified}}, Quality: &catalogue.QualityPreferences{Effective: []string{"1080p"}}}
			result, err := New(catalogueFake{event: e}, f).Search(t.Context(), Request{EventID: e.ID, Component: "main_card", FeedIDs: []int{1}, Query: tc.custom})
			require.NoError(t, err)
			require.Equal(t, tc.want, result.Query)
			require.Equal(t, tc.want, f.lastQuery)
			require.Empty(t, result.Sources[0].Error)
		})
	}
}

func TestEventSearchPreview(t *testing.T) {
	f := &feedsFake{}
	s := New(catalogueFake{event: catalogue.Event{Quality: &catalogue.QualityPreferences{Defaults: []string{"1080p"}, Effective: []string{"1080p"}}, ID: "local", Values: catalogue.Values{Name: "Display name", Type: "numbered", Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "300", Current: true, Verified: true}}}}, f)
	response, err := s.Search(context.Background(), Request{EventID: "local", Component: "main_card", FeedIDs: []int{1, 4}})
	require.NoError(t, err)
	require.Len(t, response.Results, 10)
	require.Equal(t, "UFC 300", response.Query)
	require.Equal(t, "match", response.Results[0].Assessment)
	for i, r := range response.Results {
		if i == 0 || i == 4 {
			require.True(t, r.Matched)
			require.Equal(t, "match", r.Assessment)
		} else {
			require.False(t, r.Matched)
			require.Equal(t, "needs review", r.Assessment)
			require.NotEmpty(t, r.Reasons)
		}
	}
	require.Contains(t, response.Results[1].Reasons[0], "Excluded for Main Card")
	require.True(t, response.Sources[0].More)
	require.NotContains(t, response.Sources[1].Error, "credential-bearing")
	require.Equal(t, 2, f.calls)
	for _, req := range []Request{{Component: "main_card", FeedIDs: []int{2}}, {Component: "main_card", FeedIDs: []int{3}}, {Component: "invalid", FeedIDs: []int{1}}, {Component: "prelims", FeedIDs: []int{1, 1}}} {
		_, err := s.Search(context.Background(), req)
		require.Error(t, err)
	}
	require.Equal(t, 2, f.calls)
}

func TestAllConfiguredSources(t *testing.T) {
	f := &feedsFake{}
	ids := []int{}
	for i := 1; i <= 6; i++ {
		f.all = append(f.all, domain.Feed{ID: i, Type: "NEWZNAB", Enabled: true})
		ids = append(ids, i)
	}
	s := New(catalogueFake{event: catalogue.Event{ID: "local", Values: catalogue.Values{Name: "UFC 300"}, Quality: &catalogue.QualityPreferences{Effective: []string{"1080p"}}}}, f)
	response, err := s.Search(context.Background(), Request{EventID: "local", Component: "main_card", FeedIDs: ids})
	require.NoError(t, err)
	require.Len(t, response.Sources, 6)
	require.Equal(t, 6, f.calls)
}

func (f catalogueFake) Settings(context.Context) (*catalogue.Settings, error) {
	return &catalogue.Settings{Revision: 1}, nil
}
