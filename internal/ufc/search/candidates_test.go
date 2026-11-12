// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package search

import (
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/stretchr/testify/require"
)

func TestCandidateOwnershipAndExpiry(t *testing.T) {
	s := New(nil, nil)
	id := s.remember(Request{Actor: "user:owner", Component: "main_card"}, catalogue.Event{ID: "event"}, Source{ID: 1}, feed.SearchItem{Title: "UFC.300.1080p", URL: "http://invalid.test/nzb?apikey=fake-private", Reference: "stable-guid"})
	require.NotEmpty(t, id)
	c, err := s.CandidateFor(id, "user:owner")
	require.NoError(t, err)
	require.Equal(t, "event", c.EventID)
	_, err = s.CandidateFor(id, "user:other")
	require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
	require.NotContains(t, err.Error(), "fake-private")
	c.Expires = time.Now().Add(-time.Second)
	s.candidates[id] = c
	_, err = s.CandidateFor(id, "user:owner")
	require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
}
