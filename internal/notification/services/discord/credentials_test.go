// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package discord

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestWebhookCredentialsNeverReturnedOrLogged(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprintf(w, `{"message":"request %s token fake-discord-secret"}`, r.URL.String())
	}))
	defer server.Close()
	endpoint := server.URL + "/api/webhooks/id/fake-discord-secret"
	client := NewSender(zerolog.New(&logs).Level(zerolog.TraceLevel), Config{WebHookURL: endpoint})
	err := client.SendMessage(t.Context(), testMessage())
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
	require.NotContains(t, err.Error(), "fake-discord-secret")
	require.NotContains(t, err.Error(), endpoint)
	require.NotContains(t, logs.String(), "fake-discord-secret")
	server.Close()
	err = client.SendMessage(t.Context(), testMessage())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "fake-discord-secret")
	require.NotContains(t, err.Error(), endpoint)
}
