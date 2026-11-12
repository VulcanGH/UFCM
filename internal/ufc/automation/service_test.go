// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package automation

import (
	"context"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/processing"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	enabled                          bool
	events                           []catalogue.Event
	items                            map[string]Item
	notices                          []Notice
	submissions, deliveries          int
	failedNotification, failedSource bool
	states                           []string
}

func (f *fixture) AutomationSettings(context.Context) (*Settings, error) {
	return &Settings{Revision: 1, Enabled: f.enabled}, nil
}
func (f *fixture) SaveAutomationSettings(_ context.Context, s Settings) (*Settings, error) {
	f.enabled = s.Enabled
	return &s, nil
}
func (f *fixture) AutomationEvents(context.Context, time.Time) ([]catalogue.Event, error) {
	return f.events, nil
}
func (f *fixture) EnqueueAutomationItem(_ context.Context, item Item) error {
	if _, ok := f.items[item.ID]; !ok {
		f.items[item.ID] = item
	}
	return nil
}
func (f *fixture) AutomationItems(context.Context, time.Time) ([]Item, error) {
	result := []Item{}
	for _, item := range f.items {
		result = append(result, item)
	}
	return result, nil
}
func (f *fixture) FinishAutomationItem(_ context.Context, id, state, _ string, _ time.Time) error {
	f.states = append(f.states, state)
	if state != "pending" {
		delete(f.items, id)
	}
	return nil
}
func (f *fixture) AutomationNotices(context.Context, time.Time) ([]Notice, error) {
	return f.notices, nil
}
func (f *fixture) FinishAutomationNotice(_ context.Context, _ string, done bool, _ time.Time) error {
	if done {
		f.notices = nil
	}
	return nil
}
func (f *fixture) ProcessingSettings(context.Context) (*processing.Settings, error) {
	return &processing.Settings{SABClientID: 1}, nil
}
func (f *fixture) MatchingEvents(context.Context) ([]catalogue.Event, error) { return f.events, nil }
func (f *fixture) Settings(context.Context) (*catalogue.Settings, error) {
	return &catalogue.Settings{}, nil
}
func (f *fixture) Sources(context.Context) ([]search.Source, error) {
	if f.failedSource {
		return nil, errors.New("offline")
	}
	return []search.Source{search.SourceForFeed(domain.Feed{ID: 1, Type: "NEWZNAB", Enabled: true})}, nil
}
func (f *fixture) RememberFeedCandidate(string, string, search.Source, feed.SearchItem) string {
	return "server-owned"
}
func (f *fixture) SubmitAutomatic(context.Context, string, int32, string) (*acquisition.Accepted, error) {
	f.submissions++
	return &acquisition.Accepted{Job: acquisition.Job{State: "uncertain"}}, nil
}
func (f *fixture) Deliver(context.Context, domain.NotificationPayload) error {
	f.deliveries++
	if f.failedNotification {
		return errors.New("offline")
	}
	return nil
}

func newFixture() *fixture {
	date := "2030-01-01"
	return &fixture{enabled: true, items: map[string]Item{}, events: []catalogue.Event{{ID: "event", Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Status: "scheduled", Date: &date}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "999", Current: true, Verified: true}}, Components: &catalogue.ComponentPreferences{Wanted: []string{"main_card"}}, Quality: &catalogue.QualityPreferences{Effective: []string{"1080p"}}}}}
}

func TestFeedAutomationDurabilityAndIndependentNotifications(t *testing.T) {
	f := newFixture()
	s := New(f, f, f, f, f, zerolog.Nop())
	source := &domain.Feed{ID: 1, Type: "NEWZNAB", Enabled: true}
	items := []feed.SearchItem{{Title: "UFC.999.1080p", Reference: "one", URL: "https://indexer.invalid/nzb"}, {Title: "UFC.999.Prelims.1080p", Reference: "off", URL: "https://indexer.invalid/off"}, {Title: "UFC.999.720p", Reference: "quality", URL: "https://indexer.invalid/quality"}}
	f.enabled = false
	require.NoError(t, s.ObserveFeed(t.Context(), source, items))
	require.Empty(t, f.items)
	f.enabled = true
	require.NoError(t, s.ObserveFeed(t.Context(), source, items))
	require.NoError(t, s.ObserveFeed(t.Context(), source, items))
	require.Len(t, f.items, 1)
	f.notices = []Notice{{ID: "notice", Event: "UFC_COPY_COMPLETED"}}
	f.failedNotification = true
	s = New(f, f, f, f, f, zerolog.Nop())
	require.NoError(t, s.RunOnce(t.Context()))
	require.Equal(t, 1, f.submissions)
	require.Empty(t, f.items)
	require.Len(t, f.notices, 1)
	f.failedNotification = false
	require.NoError(t, s.RunOnce(t.Context()))
	require.Equal(t, 1, f.submissions)
	require.Empty(t, f.notices)
	require.Equal(t, 2, f.deliveries)
	f.failedSource = true
	require.NoError(t, s.ObserveFeed(t.Context(), source, []feed.SearchItem{{Title: "UFC.999.1080p", Reference: "two", URL: "https://indexer.invalid/two"}}))
	f.notices = []Notice{{ID: "independent", Event: "UFC_COPY_COMPLETED"}}
	require.Error(t, s.RunOnce(t.Context()))
	require.Empty(t, f.notices)
	require.Equal(t, 3, f.deliveries)
}

func TestAutomationWindowAndMatchingPolicy(t *testing.T) {
	f := newFixture()
	event := f.events[0]
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	require.True(t, CanAdmit(event, now))
	deadline, ok := Deadline(event)
	require.True(t, ok)
	require.Equal(t, time.Date(2030, 1, 4, 0, 0, 0, 0, time.UTC), deadline)
	event.Values.Status = "completed"
	require.False(t, CanAdmit(event, now))
	require.True(t, Active(event, deadline.Add(-time.Nanosecond)))
	require.False(t, Active(event, deadline))
	event.Values.Status = "postponed"
	require.False(t, Active(event, now))
	require.False(t, ClosedWindow(event, now))
	event.Values.Date = nil
	require.False(t, ClosedWindow(event, deadline))
	for _, title := range []string{"UFC.999.Prelims.1080p", "UFC.999.720p", "UFC.999.1080p.Pre.Show"} {
		_, _, ok := eligible(title, f.events, f.events, []string{"pre show"})
		require.False(t, ok, title)
	}
	f.events = append(f.events, f.events[0])
	f.events[1].ID = "ambiguous"
	_, _, ok = eligible("UFC.999.1080p", f.events, f.events, nil)
	require.False(t, ok)
}
