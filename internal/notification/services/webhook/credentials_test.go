// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package webhook

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestWebhookCredentialsProtected(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, "fake-generic-secret "+r.URL.String())
	}))
	defer server.Close()
	endpoint := server.URL + "/hook/fake-generic-secret"
	client := NewSender(zerolog.New(&logs), Config{URL: endpoint, Method: http.MethodPost})
	err := client.SendMessage(t.Context(), &Message{Event: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
	require.NotContains(t, err.Error(), "fake-generic-secret")
	require.NotContains(t, err.Error(), endpoint)
	require.NotContains(t, logs.String(), "fake-generic-secret")
}
