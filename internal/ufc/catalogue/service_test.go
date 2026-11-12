// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type serviceRepository struct {
	attempts atomic.Int32
	applies  atomic.Int32
	failures atomic.Int32
	conflict bool
	status   *Status
}

func (r *serviceRepository) ClearCatalogueEvents(context.Context) error { return nil }
func (r *serviceRepository) ResetCatalogueEvents(context.Context) error { return nil }

func (r *serviceRepository) CatalogueMatchingEvents(context.Context) ([]Event, error) {
	return nil, nil
}

func (r *serviceRepository) CreateCatalogueEvent(context.Context, EventCreate, string) (Event, error) {
	return Event{}, nil
}

func (r *serviceRepository) ClaimCatalogueRefresh(ctx context.Context, at time.Time, scheduled bool) (bool, error) {
	if scheduled {
		return false, nil
	}
	return true, r.RecordCatalogueAttempt(ctx, at)
}

func (r *serviceRepository) CataloguePreferences(context.Context) (*Settings, error) {
	return &Settings{Revision: 1, Templates: DefaultTemplates()}, nil
}

func (r *serviceRepository) SaveCataloguePreferences(context.Context, SettingsEdit) (*Settings, error) {
	return &Settings{Revision: 2, Templates: DefaultTemplates()}, nil
}

func (r *serviceRepository) LoadCatalogueEvent(context.Context, string) (Event, error) {
	return Event{}, ErrEventNotFound
}

func (r *serviceRepository) EditCatalogueEvent(context.Context, string, EventEdit, string) (Event, error) {
	return Event{}, ErrEventNotFound
}

func (r *serviceRepository) CatalogueEvents(context.Context, string) (*EventList, error) {
	return &EventList{Upcoming: []EventSummary{}, RecentCompleted: []EventSummary{}}, nil
}

func (r *serviceRepository) CatalogueStatus(context.Context) (*Status, error) {
	if r.status != nil {
		copy := *r.status
		return &copy, nil
	}
	return &Status{Events: 1}, nil
}

func TestRestartReportsInterruptedAttemptWithoutAutomaticWork(t *testing.T) {
	attempt := time.Now().UTC().Format(time.RFC3339Nano)
	repo := &serviceRepository{status: &Status{Events: 3, LastAttempt: &attempt}}
	s := New(repo, serviceSource{read: func(context.Context) (*Snapshot, error) {
		t.Error("restart must not start periodic or automatic refresh")
		return nil, errors.New("unexpected source call")
	}}, zerolog.Nop())
	require.NoError(t, s.Start(t.Context()))
	status, err := s.Status(t.Context())
	require.NoError(t, err)
	require.True(t, status.Interrupted)
	require.Equal(t, 3, status.Events)
	require.Zero(t, repo.attempts.Load())
	require.NoError(t, s.Stop(t.Context()))
}
func (r *serviceRepository) KnownCatalogueSources(context.Context) ([]Facts, error) { return nil, nil }
func (r *serviceRepository) ApplyCatalogue(context.Context, *Snapshot) (Report, error) {
	count := r.applies.Add(1)
	if r.conflict && count == 1 {
		return Report{}, ErrRevisionConflict
	}
	return Report{}, nil
}
func (r *serviceRepository) RecordCatalogueAttempt(context.Context, time.Time) error {
	r.attempts.Add(1)
	return nil
}
func (r *serviceRepository) RecordCatalogueFailure(context.Context, string) error {
	r.failures.Add(1)
	return nil
}

type serviceSource struct {
	read func(context.Context) (*Snapshot, error)
}

func (s serviceSource) Read(ctx context.Context, _ []Facts) (*Snapshot, error) { return s.read(ctx) }

func TestRefreshCoalescingAndRevisionRetry(t *testing.T) {
	repo := &serviceRepository{conflict: true}
	entered := make(chan struct{})
	release := make(chan struct{})
	source := serviceSource{read: func(ctx context.Context) (*Snapshot, error) {
		close(entered)
		select {
		case <-release:
			return &Snapshot{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	s := New(repo, source, zerolog.Nop())
	require.NoError(t, s.Start(t.Context()))
	accepted, err := s.Refresh(t.Context())
	require.NoError(t, err)
	require.False(t, accepted.Coalesced)
	<-entered
	for range 20 {
		accepted, err = s.Refresh(t.Context())
		require.NoError(t, err)
		require.True(t, accepted.Coalesced)
	}
	status, err := s.Status(t.Context())
	require.NoError(t, err)
	require.True(t, status.Running)
	require.Equal(t, 1, status.Events)
	close(release)
	require.Eventually(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return !s.running }, time.Second, time.Millisecond)
	require.EqualValues(t, 1, repo.attempts.Load())
	require.EqualValues(t, 2, repo.applies.Load())
	require.NoError(t, s.Stop(t.Context()))
}

func TestShutdownCancelsAndJoinsRefresh(t *testing.T) {
	repo := &serviceRepository{}
	entered := make(chan struct{})
	source := serviceSource{read: func(ctx context.Context) (*Snapshot, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() }}
	s := New(repo, source, zerolog.Nop())
	require.NoError(t, s.Start(t.Context()))
	_, err := s.Refresh(t.Context())
	require.NoError(t, err)
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
	require.EqualValues(t, 1, repo.failures.Load())
	require.Zero(t, repo.applies.Load())
	_, err = s.Refresh(t.Context())
	require.Error(t, err)
}

func TestFailedDiscoveryDoesNotApply(t *testing.T) {
	repo := &serviceRepository{}
	s := New(repo, serviceSource{read: func(context.Context) (*Snapshot, error) { return nil, errors.New("mock incomplete discovery") }}, zerolog.Nop())
	require.NoError(t, s.Start(t.Context()))
	_, err := s.Refresh(t.Context())
	require.NoError(t, err)
	require.Eventually(t, func() bool { return repo.failures.Load() == 1 }, time.Second, time.Millisecond)
	require.NoError(t, s.Stop(t.Context()))
	require.Zero(t, repo.applies.Load())
}
