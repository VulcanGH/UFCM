// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package sabnzbd

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNamedSubmissionAndTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "fake-api-key", r.URL.Query().Get("apikey"))
		switch r.URL.Query().Get("mode") {
		case "addurl":
			require.Equal(t, "UFC.300.1080p UFCM-owned", r.URL.Query().Get("nzbname"))
			require.Equal(t, "sports", r.URL.Query().Get("cat"))
			_, _ = w.Write([]byte(`{"status":true,"nzo_ids":["remote-1"]}`))
		case "queue":
			_, _ = w.Write([]byte(`{"queue":{"slots":[]}}`))
		case "history":
			_, _ = w.Write([]byte(`{"history":{"slots":[{"nzo_id":"remote-1","name":"UFC.300.1080p UFCM-owned.nzb","status":"Completed","storage":"mock-path"},{"nzo_id":"remote-legacy","name":"UFCM-legacy UFC.300.1080p","status":"Completed"}]}}`))
		}
	}))
	defer server.Close()
	c := New(Options{Addr: server.URL, ApiKey: "fake-api-key", Log: zerolog.Nop()}).Bounded(time.Second)
	response, err := c.AddFromUrl(t.Context(), AddNzbRequest{Url: "http://mock.invalid/nzb", Name: "UFC.300.1080p UFCM-owned", Category: "sports"})
	require.NoError(t, err)
	require.True(t, response.Status)
	job, err := c.Lookup(t.Context(), "", "UFCM-owned")
	require.NoError(t, err)
	require.True(t, job.Found)
	require.True(t, job.History)
	require.Equal(t, "remote-1", job.ID)
	legacy, err := c.Lookup(t.Context(), "", "UFCM-legacy")
	require.NoError(t, err)
	require.True(t, legacy.Found)
	require.Equal(t, "remote-legacy", legacy.ID)
	job, err = c.Lookup(t.Context(), "", "UFCM-other")
	require.NoError(t, err)
	require.False(t, job.Found)
}

func TestExplicitHTTPRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	c := New(Options{Addr: server.URL, ApiKey: "fake-private", Log: zerolog.Nop()})
	_, err := c.AddFromUrl(t.Context(), AddNzbRequest{Url: "http://invalid.test/?apikey=fake-indexer"})
	require.ErrorIs(t, err, ErrRequestRejected)
	require.NotContains(t, err.Error(), "fake-private")
	require.NotContains(t, err.Error(), "fake-indexer")
}
