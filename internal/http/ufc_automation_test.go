// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/automation"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type automationStub struct{}

func (automationStub) Settings(context.Context) (*automation.Settings, error) {
	return &automation.Settings{Revision: 1}, nil
}
func (automationStub) SaveSettings(_ context.Context, input automation.Settings) (*automation.Settings, error) {
	return &input, nil
}

func TestAutomationSettingsScope(t *testing.T) {
	server := newScopedTestServer()
	server.foundationService = foundationStub{}
	server.ufcAutomationService = automationStub{}
	server.apiService.(*apikeyServiceStub).tokens["ufc-read"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	server.apiService.(*apikeyServiceStub).tokens["ufc-write"] = &domain.APIKey{Scopes: []string{"ufc:write"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	require.Equal(t, 200, serveWithToken(router, "GET", "/ufc/automation/settings", "ufc-read", "").Code)
	require.Equal(t, 403, serveWithToken(router, "GET", "/ufc/automation/settings", "config-read", "").Code)
	for _, token := range []string{"", "ufc-read", "ufc-write"} {
		require.Equal(t, 403, serveWithToken(router, "PUT", "/ufc/automation/settings", token, `{"revision":1,"enabled":true}`).Code)
	}
	sessionRouter := chi.NewRouter()
	sessionRouter.Use(server.sessionManager.LoadAndSave)
	sessionRouter.Get("/session", func(w http.ResponseWriter, r *http.Request) {
		server.sessionManager.Put(r.Context(), "authenticated", true)
		server.sessionManager.Put(r.Context(), "username", "automation-test")
		server.sessionManager.Put(r.Context(), "auth_method", string(domain.AuthMethodPassword))
		w.WriteHeader(204)
	})
	sessionRouter.Mount("/api", server.apiRoutes())
	login := httptest.NewRecorder()
	sessionRouter.ServeHTTP(login, httptest.NewRequest("GET", "/session", nil))
	for _, tc := range []struct {
		body string
		code int
	}{{`{"revision":1,"enabled":true}`, 200}, {`{"revision":1,"enabled":false}`, 200}} {
		login = httptest.NewRecorder()
		sessionRouter.ServeHTTP(login, httptest.NewRequest("GET", "/session", nil))
		request := httptest.NewRequest("PUT", "/api/ufc/automation/settings", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		for _, cookie := range login.Result().Cookies() {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		sessionRouter.ServeHTTP(response, request)
		require.Equal(t, tc.code, response.Code, response.Body.String())
	}
}
