// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/processing"
	"github.com/stretchr/testify/require"
)

type processingSettingsStub struct{}

func (processingSettingsStub) Settings(context.Context) (*processing.Settings, error) {
	return &processing.Settings{Revision: 1, ConflictPolicy: "review"}, nil
}
func (processingSettingsStub) SaveSettings(_ context.Context, input processing.Settings) (*processing.Settings, error) {
	return &input, nil
}
func (processingSettingsStub) Categories(context.Context, int32) ([]string, error) {
	return []string{"ufc"}, nil
}
func (processingSettingsStub) Preview(context.Context, string) (*processing.Preview, error) {
	return &processing.Preview{Status: "ready", Files: []processing.FilePlan{}}, nil
}
func (processingSettingsStub) Queue(context.Context, string) (processing.Job, error) {
	return processing.Job{ID: "job", State: "queued"}, nil
}
func (processingSettingsStub) Job(context.Context, string) (processing.Job, error) {
	return processing.Job{ID: "job", State: "queued"}, nil
}
func (processingSettingsStub) Jobs(context.Context) ([]processing.Job, error) {
	return []processing.Job{}, nil
}
func (processingSettingsStub) Review(context.Context, string) (processing.ReviewOptions, error) {
	return processing.ReviewOptions{}, nil
}
func (processingSettingsStub) Retry(context.Context, string) (processing.Job, error) {
	return processing.Job{}, nil
}
func (processingSettingsStub) RetryCleanup(context.Context, string) (processing.Job, error) {
	return processing.Job{}, nil
}
func (processingSettingsStub) KeepBoth(context.Context, string, string) (processing.Job, error) {
	return processing.Job{}, nil
}
func (processingSettingsStub) Skip(context.Context, string) (processing.Job, error) {
	return processing.Job{}, nil
}
func (processingSettingsStub) Cancel(context.Context, string) (processing.Job, error) {
	return processing.Job{}, nil
}

func TestProcessingSettingsScope(t *testing.T) {
	server := newScopedTestServer()
	server.foundationService = foundationStub{}
	server.ufcProcessingService = processingSettingsStub{}
	server.apiService.(*apikeyServiceStub).tokens["ufc-read"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	server.apiService.(*apikeyServiceStub).tokens["ufc-write"] = &domain.APIKey{Scopes: []string{"ufc:write"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 403}, {"config-read", 403}, {"ufc-read", 200}, {"ufc-write", 200}} {
		require.Equal(t, tc.code, serveWithToken(router, "GET", "/ufc/processing/settings", tc.token, "").Code)
	}
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/processing/categories/1", "ufc-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "GET", "/ufc/processing/categories/1", "config-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "PUT", "/ufc/processing/settings", "ufc-read", `{}`).Code)
	require.Equal(t, 403, serveWithToken(router, "PUT", "/ufc/processing/settings", "ufc-write", `{}`).Code)
	require.Equal(t, 200, serveWithToken(router, "POST", "/ufc/processing/preview", "ufc-read", `{"acquisition_id":"owned"}`).Code)
	require.Equal(t, 403, serveWithToken(router, "POST", "/ufc/processing/preview", "config-read", `{"acquisition_id":"owned"}`).Code)
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/processing/jobs", "ufc-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "GET", "/ufc/processing/jobs", "config-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "POST", "/ufc/processing/jobs", "ufc-read", `{"acquisition_id":"owned"}`).Code)
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/processing/jobs/job/review", "ufc-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "POST", "/ufc/processing/jobs/job/retry", "ufc-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "POST", "/ufc/processing/jobs/job/cleanup/retry", "ufc-read", "").Code)
}
