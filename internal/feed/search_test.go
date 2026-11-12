// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/stretchr/testify/require"
)

type manualSearchRepo struct {
	feedRepo
	value *domain.Feed
}

func (r manualSearchRepo) FindByID(context.Context, int) (*domain.Feed, error) { return r.value, nil }

func TestManualSearchIsolation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "saved-cookie", r.Header.Get("Cookie"))
		require.Equal(t, "saved-agent", r.Header.Get("User-Agent"))
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><limits max="50" default="50"/><searching><search available="yes" supportedParams="q"/></searching></caps>`))
			return
		}
		requests++
		require.Equal(t, "UFC 300", r.URL.Query().Get("q"))
		require.Equal(t, "5000", r.URL.Query().Get("cat"))
		require.Equal(t, "fake-secret", r.URL.Query().Get("apikey"))
		_, _ = w.Write([]byte(`<rss><channel><title>Mock</title><item><title>UFC.300.Main.Card.1080p</title><guid>secret-link</guid><link>https://invalid.test/?apikey=fake-secret</link><size>1234</size><pubDate>Tue, 10 Nov 2026 12:00:00 +0000</pubDate></item></channel></rss>`))
	}))
	defer server.Close()
	f := &domain.Feed{ID: 1, Type: "NEWZNAB", Enabled: true, URL: server.URL, ApiKey: "fake-secret", Cookie: "saved-cookie", UserAgent: "saved-agent", Categories: []int{5000}}
	s := &Service{repo: manualSearchRepo{value: f}}
	page, err := s.Search(context.Background(), 1, "UFC 300")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, uint64(1234), page.Items[0].Size)
	require.NotNil(t, page.Items[0].PublishedAt)
	require.Equal(t, "2026-11-10", page.Items[0].PublishedAt.Format("2006-01-02"))
	require.Equal(t, 1, requests)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "fake-secret")
	require.NotContains(t, string(encoded), "secret-link")
	f.Enabled = false
	_, err = s.Search(context.Background(), 1, "UFC 300")
	require.Error(t, err)
	require.Equal(t, 1, requests)
	f.Enabled = true
	f.URL = "http://127.0.0.1:0/?apikey=fake-secret"
	_, err = s.Search(context.Background(), 1, "UFC 300")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "fake-secret")
	require.NotContains(t, err.Error(), "127.0.0.1")
}
