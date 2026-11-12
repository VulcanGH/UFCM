// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package notification

import (
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUFCCompletionDestinations(t *testing.T) {
	for _, event := range []domain.NotificationEvent{domain.NotificationEventUFCDownloadCompleted, domain.NotificationEventUFCCopyCompleted, domain.NotificationEventUFCAttention} {
		payload := domain.NotificationPayload{Event: event, Message: "Fixture completion", ReleaseName: "UFC fixture", Timestamp: time.Now()}
		discord, err := buildDiscordMessage(payload)
		require.NoError(t, err)
		require.NotNil(t, discord)
		webhook := buildWebhookMessage(payload)
		require.Contains(t, webhook.Event, "ufc.")
		require.NotNil(t, webhook.Payload)
		require.NotEqual(t, "New Event", BuildTitle(event))
	}
}
