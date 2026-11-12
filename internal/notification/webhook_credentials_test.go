// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestWebhookCredentialLifecycle(t *testing.T) {
	const secret = "https://example.invalid/api/webhooks/id/fake-secret-token"
	repo := newTestNotificationRepo([]domain.Notification{{ID: 1, Name: "test", Type: domain.NotificationTypeDiscord, Webhook: secret}}, nil)
	var logs bytes.Buffer
	service := &Service{log: zerolog.New(&logs), repo: repo}
	service.state.Store(newRoutingSnapshot())
	for _, body := range []string{`{"id":1,"name":"test","type":"DISCORD","enabled":false}`, `{"id":1,"name":"test","type":"DISCORD","enabled":false,"webhook":null}`, `{"id":1,"name":"test","type":"DISCORD","webhook":"<redacted>","enabled":false}`} {
		var update domain.Notification
		require.NoError(t, json.Unmarshal([]byte(body), &update))
		require.NoError(t, service.Update(context.Background(), &update))
		stored, err := repo.FindByID(t.Context(), 1)
		require.NoError(t, err)
		require.Equal(t, secret, stored.Webhook)
		data, err := json.Marshal(stored)
		require.NoError(t, err)
		require.NotContains(t, string(data), secret)
		var response map[string]any
		require.NoError(t, json.Unmarshal(data, &response))
		require.Equal(t, "<redacted>", response["webhook"])
	}
	replacement := domain.Notification{ID: 1, Name: "test", Type: domain.NotificationTypeDiscord, Webhook: "https://example.invalid/new-fake-token"}
	require.NoError(t, service.Update(t.Context(), &replacement))
	stored, err := repo.FindByID(t.Context(), 1)
	require.NoError(t, err)
	require.Equal(t, replacement.Webhook, stored.Webhook)
	var invalidClear domain.Notification
	require.NoError(t, json.Unmarshal([]byte(`{"id":1,"name":"test","type":"DISCORD","enabled":true,"webhook":""}`), &invalidClear))
	require.ErrorIs(t, service.Update(t.Context(), &invalidClear), domain.ErrNotificationInvalid)
	stored, err = repo.FindByID(t.Context(), 1)
	require.NoError(t, err)
	require.Equal(t, replacement.Webhook, stored.Webhook)
	var cleared domain.Notification
	require.NoError(t, json.Unmarshal([]byte(`{"id":1,"name":"test","type":"DISCORD","enabled":false,"webhook":""}`), &cleared))
	require.NoError(t, service.Update(t.Context(), &cleared))
	stored, err = repo.FindByID(t.Context(), 1)
	require.NoError(t, err)
	require.Empty(t, stored.Webhook)
	require.NotContains(t, logs.String(), secret)
	require.NotContains(t, logs.String(), "new-fake-token")
}
