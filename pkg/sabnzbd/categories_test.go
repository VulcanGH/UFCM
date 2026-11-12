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

func TestCategoriesReadOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "get_cats", r.URL.Query().Get("mode"))
		require.Equal(t, "fake", r.URL.Query().Get("apikey"))
		_, _ = w.Write([]byte(`{"categories":["ufc","*","sports","ufc"]}`))
	}))
	defer server.Close()
	client := New(Options{Addr: server.URL, ApiKey: "fake", Log: zerolog.Nop()}).Bounded(time.Second)
	values, err := client.Categories(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"sports", "ufc"}, values)
}
