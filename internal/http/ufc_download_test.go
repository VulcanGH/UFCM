// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/stretchr/testify/require"
)

type downloadStub struct {
	calls int
	actor string
}

func (*downloadStub) Clients(context.Context) ([]acquisition.ClientOption, error) {
	return []acquisition.ClientOption{{ID: 1, Name: "Saved SAB"}}, nil
}
func (*downloadStub) Jobs(context.Context) ([]acquisition.Job, error) {
	return []acquisition.Job{}, nil
}
func (s *downloadStub) Submit(_ context.Context, r acquisition.Submit) (*acquisition.Accepted, error) {
	s.calls++
	s.actor = r.Actor
	return &acquisition.Accepted{Job: acquisition.Job{ID: "owned"}}, nil
}
func (*downloadStub) Reconcile(context.Context) error      { return nil }
func (*downloadStub) Remove(context.Context, string) error { return nil }
func TestManualDownloadScopes(t *testing.T) {
	s := newScopedTestServer()
	s.foundationService = foundationStub{}
	s.catalogueService = catalogueStub{}
	stub := &downloadStub{}
	s.ufcDownloadService = stub
	keys := s.apiService.(*apikeyServiceStub).tokens
	keys["ufc-read"] = &domain.APIKey{ID: 1, Scopes: []string{"ufc:read"}}
	keys["ufc-invoke"] = &domain.APIKey{ID: 2, Scopes: []string{"ufc:invoke"}}
	keys["ufc-write"] = &domain.APIKey{ID: 3, Scopes: []string{"ufc:write"}}
	router := s.sessionManager.LoadAndSave(s.apiRoutes())
	for _, tc := range []struct {
		key  string
		code int
	}{{"", 403}, {"config-read", 403}, {"ufc-read", 403}, {"ufc-invoke", 202}} {
		require.Equal(t, tc.code, serveWithToken(router, "POST", "/ufc/downloads", tc.key, `{"candidate_id":"owned","client_id":1}`).Code)
	}
	require.Equal(t, 1, stub.calls)
	for _, tc := range []struct {
		key  string
		code int
	}{{"", 403}, {"ufc-read", 403}, {"ufc-write", 200}, {"ufc-invoke", 200}} {
		require.Equal(t, tc.code, serveWithToken(router, "POST", "/ufc/downloads/owned/recover", tc.key, `{"revision":1,"remote_id":"sab-id","confirm":true}`).Code)
	}
	require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/downloads/owned/recover", "ufc-invoke", `{"revision":1,"unexpected":true}`).Code)

	for _, tc := range []struct {
		key  string
		code int
	}{{"", 403}, {"ufc-read", 403}, {"ufc-invoke", 403}, {"ufc-write", 200}} {
		require.Equal(t, tc.code, serveWithToken(router, "DELETE", "/ufc/downloads/owned", tc.key, "").Code)
	}
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/catalogue/events?offset=0", "ufc-read", "").Code)
	require.Equal(t, 400, serveWithToken(router, "GET", "/ufc/catalogue/events?offset=-1", "ufc-read", "").Code)
	require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/catalogue/refresh", "ufc-invoke", `{"scope":"rebuild"}`).Code)
	require.Equal(t, 202, serveWithToken(router, "POST", "/ufc/catalogue/refresh", "ufc-invoke", `{"scope":"rebuild","confirm":true}`).Code)
	require.Equal(t, 403, serveWithToken(router, "POST", "/ufc/catalogue/reset", "ufc-read", `{"confirm":true}`).Code)
	require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/catalogue/reset", "ufc-write", `{"confirm":false}`).Code)
	require.Equal(t, 200, serveWithToken(router, "POST", "/ufc/catalogue/reset", "ufc-write", `{"confirm":true}`).Code)
	require.Equal(t, "api_key:2", stub.actor)
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/downloads", "ufc-read", "").Code)
	require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/downloads", "ufc-invoke", `{"candidate_id":"owned","client_id":1,"url":"http://invalid.test"}`).Code)
	require.Equal(t, 1, stub.calls)
	for _, tc := range []struct {
		key  string
		code int
	}{{"", 403}, {"ufc-read", 403}, {"ufc-write", 200}, {"ufc-invoke", 200}} {
		require.Equal(t, tc.code, serveWithToken(router, "POST", "/ufc/downloads/owned/recover", tc.key, `{"revision":1,"remote_id":"sab-id","confirm":true}`).Code)
	}
	require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/downloads/owned/recover", "ufc-invoke", `{"revision":1,"unexpected":true}`).Code)

}

func (*downloadStub) Recover(context.Context, string, acquisition.Recovery) (*acquisition.Job, error) {
	return &acquisition.Job{ID: "owned"}, nil
}
