// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package acquisition

import (
	"context"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/autobrr/autobrr/pkg/sabnzbd"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type rejectedCandidate struct{ testCandidates }

func (rejectedCandidate) CandidateFor(id, actor string) (search.Candidate, error) {
	c, _ := (testCandidates{}).CandidateFor(id, actor)
	c.Title = "UFC.999.Prelims.1080p"
	return c, nil
}

func TestManualAssignmentSafety(t *testing.T) {
	repo := &testRepository{}
	remote := &testRemote{}
	events := &testCatalogue{resolution: "1080p"}
	s := New(repo, rejectedCandidate{}, events, testGateway{remote}, zerolog.Nop())
	input := Submit{CandidateID: "selection", ClientID: 1, Actor: "user:test"}
	_, err := s.Submit(t.Context(), input)
	require.Error(t, err)
	require.Zero(t, remote.calls)
	input.ManualReason = "Reviewed this incorrectly labelled release"
	input.EventRevision = 1
	input.SettingsRevision = 1
	stale := input
	stale.EventRevision = 2
	_, err = s.Submit(t.Context(), stale)
	require.Error(t, err)
	require.Zero(t, remote.calls)
	events.resolution = "720p"
	_, err = s.Submit(t.Context(), input)
	require.Error(t, err)
	require.Zero(t, remote.calls)
	events.resolution = "1080p"
	result, err := s.Submit(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, 1, remote.calls)
	require.True(t, result.Job.Decision.Manual)
	require.Equal(t, input.ManualReason, result.Job.Decision.Reason)
	require.Equal(t, "unmatched", result.Job.Decision.Automatic.Status)
	_, err = s.Submit(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, 1, remote.calls)
}

func TestVerifiedBindingSafety(t *testing.T) {
	repo := &testRepository{job: &Job{ID: "intent", Title: "UFC.300.1080p", Revision: 2, State: "uncertain", ClientID: 1, Endpoint: "saved-endpoint"}}
	remote := &testRemote{}
	s := New(repo, testCandidates{}, &testCatalogue{}, testGateway{remote}, zerolog.Nop())
	input := Recovery{Revision: 2, RemoteID: "sab-verified", Confirm: true, Actor: "user:test"}
	_, err := s.Recover(t.Context(), "intent", input)
	require.Error(t, err)
	remote.found = sabnzbd.Job{Found: true, ID: input.RemoteID, State: "Completed", History: true, Title: "UFC.999.1080p"}
	_, err = s.Recover(t.Context(), "intent", input)
	require.Error(t, err)
	require.Equal(t, "uncertain", repo.job.State)
	remote.found.Title = "UFC.300.1080p UFCM-intent"
	stale := input
	stale.Revision = 1
	_, err = s.Recover(t.Context(), "intent", stale)
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	remote.found.Ambiguous = true
	_, err = s.Recover(t.Context(), "intent", input)
	require.Error(t, err)
	remote.found.Ambiguous = false
	result, err := s.Recover(t.Context(), "intent", input)
	require.NoError(t, err)
	require.Equal(t, "completed", result.State)
	require.Equal(t, input.RemoteID, result.RemoteID)
	require.Zero(t, remote.calls)
	_, err = s.Recover(context.Background(), "intent", Recovery{Revision: result.Revision, RemoteID: input.RemoteID, Confirm: true})
	require.Error(t, err)
}
