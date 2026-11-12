// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/stretchr/testify/require"
)

type eventSearchStub struct{ calls int }

func (*eventSearchStub) Sources(context.Context) ([]search.Source, error) {
	return []search.Source{}, nil
}
func (s *eventSearchStub) Search(_ context.Context, r search.Request) (*search.Response, error) {
	s.calls++
	return &search.Response{EventID: r.EventID, Component: r.Component, Results: []search.Result{}, Sources: []search.SourceResult{}}, nil
}

func TestEventSearchScopes(t *testing.T) {
	server := newScopedTestServer()
	server.catalogueService = catalogueStub{}
	server.foundationService = foundationStub{}
	stub := &eventSearchStub{}
	server.ufcSearchService = stub
	keys := server.apiService.(*apikeyServiceStub).tokens
	keys["ufc-read"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	keys["ufc-invoke"] = &domain.APIKey{Scopes: []string{"ufc:invoke"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 403}, {"config-read", 403}, {"ufc-read", 403}, {"ufc-invoke", 200}} {
		require.Equal(t, tc.code, serveWithToken(router, "POST", "/ufc/events/local/search", tc.token, `{"component":"main_card","feed_ids":[1]}`).Code)
	}
	require.Equal(t, 1, stub.calls)
	require.Equal(t, 403, serveWithToken(router, "GET", "/ufc/search/sources", "config-read", "").Code)
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/search/sources", "ufc-read", "").Code)
	require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/events/local/search", "ufc-invoke", `{"component":"main_card","url":"http://invalid.test"}`).Code)
	require.Equal(t, 1, stub.calls)
}
