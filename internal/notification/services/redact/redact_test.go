// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package redact

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestError(t *testing.T) {
	endpoint := "https://example.invalid/api/webhooks/id/fake-secret-token?key=fake-query-key"
	err := Error(fmt.Errorf("%s fake-secret-token fake-query-key: %w", endpoint, context.Canceled), endpoint)
	require.NotContains(t, err.Error(), endpoint)
	require.NotContains(t, err.Error(), "fake-secret-token")
	require.NotContains(t, err.Error(), "fake-query-key")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, Error(nil, endpoint))
}

func TestShortCredential(t *testing.T) {
	require.Equal(t, "request token <redacted>", Text("request token abc", "https://example.invalid/hook/abc"))
}

func TestEncodedAndOverlappingCredentials(t *testing.T) {
	endpoint := "https://example.invalid/hook/secret-token?key=some%26secret"
	require.Equal(t, `{"key":"<redacted>"}`, Text(`{"key":"some\u0026secret"}`, endpoint))
	require.Equal(t, "<redacted>", Text(endpoint, endpoint))
	require.Equal(t, "<redacted>", Text("redacted", "https://example.invalid/hook/redacted"))
}
