// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package acquisition

import (
	"context"
	"sync"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/autobrr/autobrr/pkg/sabnzbd"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type testRepository struct{ job *Job }

func (*testRepository) DismissAcquisition(context.Context, string) error { return nil }

func (r *testRepository) ReserveAcquisition(_ context.Context, j Job) (Job, bool, error) {
	if r.job != nil {
		return *r.job, false, nil
	}
	j.Revision = 1
	j.State = "submitting"
	j.UpdatedAt = j.CreatedAt
	r.job = &j
	return j, true, nil
}
func (r *testRepository) SaveAcquisition(_ context.Context, j Job) error {
	j.Revision++
	r.job = &j
	return nil
}
func (r *testRepository) Acquisitions(_ context.Context, active bool) ([]Job, error) {
	if r.job == nil || (active && (r.job.State == "completed" || r.job.State == "failed")) {
		return []Job{}, nil
	}
	return []Job{*r.job}, nil
}

type testCandidates struct{}

func (testCandidates) CandidateFor(id, actor string) (search.Candidate, error) {
	return search.Candidate{ID: id, Actor: actor, EventID: "event", Component: "main_card", Title: "UFC.300.1080p", URL: "http://invalid.test/nzb?apikey=fake-private", FeedID: 1, Key: "stable-key"}, nil
}
func (testCandidates) Sources(context.Context) ([]search.Source, error) {
	return []search.Source{{ID: 1, Type: "NEWZNAB"}}, nil
}

type testCatalogue struct{ resolution string }

func (c *testCatalogue) Event(context.Context, string) (catalogue.Event, error) {
	return catalogue.Event{ID: "event", Revision: 1, Values: catalogue.Values{Status: "completed"}, Quality: &catalogue.QualityPreferences{Effective: []string{c.resolution}}}, nil
}
func (c *testCatalogue) MatchingEvents(context.Context) ([]catalogue.Event, error) {
	return []catalogue.Event{{ID: "event", Values: catalogue.Values{Name: "UFC 300", Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "300", Current: true, Verified: true}}}}, nil
}

type testRemote struct {
	calls     int
	uncertain bool
	found     sabnzbd.Job
	marker    string
}

func (r *testRemote) AddFromUrl(_ context.Context, input sabnzbd.AddNzbRequest) (*sabnzbd.AddFileResponse, error) {
	r.calls++
	r.marker = input.Name
	if r.uncertain {
		return nil, context.DeadlineExceeded
	}
	return &sabnzbd.AddFileResponse{Status: true, NzoIDs: []string{"sab-1"}}, nil
}
func (r *testRemote) Lookup(context.Context, string, string) (sabnzbd.Job, error) {
	return r.found, nil
}

type testGateway struct{ remote *testRemote }

func (g testGateway) Clients(context.Context) ([]ClientOption, error) {
	return []ClientOption{{ID: 1, Name: "Saved SAB"}}, nil
}
func (g testGateway) Target(context.Context, int32) (target, remote, error) {
	return target{Name: "Saved SAB", Endpoint: "saved-endpoint"}, g.remote, nil
}

func TestManualSubmitAndRecovery(t *testing.T) {
	ctx := t.Context()
	repo := &testRepository{}
	remote := &testRemote{uncertain: true}
	events := &testCatalogue{resolution: "1080p"}
	s := New(repo, testCandidates{}, events, testGateway{remote}, zerolog.Nop())
	input := Submit{CandidateID: "selected", ClientID: 1, Actor: "user:test"}
	result, err := s.Submit(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "uncertain", result.Job.State)
	require.Equal(t, 1, remote.calls)
	require.NotContains(t, result.Job.Message, "fake-private")
	require.Contains(t, remote.marker, "UFCM-"+result.Job.ID)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Submit(ctx, input); require.NoError(t, err) }()
	}
	wg.Wait()
	require.Equal(t, 1, remote.calls)
	restarted := New(repo, testCandidates{}, events, testGateway{remote}, zerolog.Nop())
	remote.found = sabnzbd.Job{Found: true, ID: "sab-recovered", State: "Downloading", Percent: 25}
	require.NoError(t, restarted.Reconcile(ctx))
	require.Equal(t, "downloading", repo.job.State)
	require.Equal(t, "sab-recovered", repo.job.RemoteID)
	require.Equal(t, 1, remote.calls)
	remote.found = sabnzbd.Job{Found: true, ID: "sab-recovered", State: "Completed"}
	require.NoError(t, restarted.Reconcile(ctx))
	require.Equal(t, "post_processing", repo.job.State)
	remote.found = sabnzbd.Job{}
	require.NoError(t, restarted.Reconcile(ctx))
	require.Equal(t, "missing", repo.job.State)
	remote.found = sabnzbd.Job{Found: true, History: true, ID: "sab-recovered", State: "Completed", Storage: "mock-complete-path"}
	require.NoError(t, restarted.Reconcile(ctx))
	require.Equal(t, "completed", repo.job.State)
	require.Equal(t, "mock-complete-path", repo.job.Storage)
	require.Equal(t, 1, remote.calls)
	events.resolution = "720p"
	_, err = restarted.Submit(ctx, input)
	require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
	require.Equal(t, 1, remote.calls)
}

func TestAutomaticSubmissionUsesExistingUncertaintyReservation(t *testing.T) {
	repo := &testRepository{}
	remote := &testRemote{uncertain: true}
	s := New(repo, testCandidates{}, &testCatalogue{resolution: "1080p"}, testGateway{remote}, zerolog.Nop())
	for i := 0; i < 2; i++ {
		result, err := s.SubmitAutomatic(t.Context(), "feed-candidate", 1, "ufc")
		require.NoError(t, err)
		require.Equal(t, "uncertain", result.Job.State)
		require.True(t, result.Job.Decision.Automated)
		require.Equal(t, search.AutomationActor, result.Job.Decision.Actor)
	}
	require.Equal(t, 1, remote.calls)
}

func (c *testCatalogue) Settings(context.Context) (*catalogue.Settings, error) {
	return &catalogue.Settings{Revision: 1}, nil
}

func (r *testRepository) Acquisition(context.Context, string) (Job, error) { return *r.job, nil }
func (r *testRepository) BindAcquisition(ctx context.Context, j Job, _, _ string) error {
	if r.job.Revision != j.Revision {
		return catalogue.ErrRevisionConflict
	}
	return r.SaveAcquisition(ctx, j)
}
