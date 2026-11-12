// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/updater"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type updateStub struct{ installs int }

func (s *updateStub) Status(context.Context) (updater.Status, error) {
	return updater.Status{Current: "1.0.0"}, nil
}
func (s *updateStub) Check(ctx context.Context) (updater.Status, error) { return s.Status(ctx) }
func (s *updateStub) Install(ctx context.Context, id string) (updater.Status, error) {
	s.installs++
	return s.Status(ctx)
}
func (s *updateStub) Preferences(ctx context.Context, revision int64, enabled bool) (updater.Status, error) {
	return s.Status(ctx)
}

func TestUpdateRoutesRequireUserAndRejectBrowserPaths(t *testing.T) {
	server := newScopedTestServer()
	server.foundationService = foundationStub{}
	service := &updateStub{}
	server.ufcUpdateService = service
	server.apiService.(*apikeyServiceStub).tokens["ufc-write"] = &domain.APIKey{Scopes: []string{"ufc:write", "ufc:read"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, path := range []string{"/ufc/updates/", "/ufc/updates/check", "/ufc/updates/install", "/ufc/updates/settings"} {
		method := "POST"
		if strings.HasSuffix(path, "/") {
			method = "GET"
		}
		if strings.HasSuffix(path, "settings") {
			method = "PUT"
		}
		require.Equal(t, 403, serveWithToken(router, method, path, "ufc-write", `{"id":"release"}`).Code)
	}
	session := chi.NewRouter()
	session.Use(server.sessionManager.LoadAndSave)
	session.Get("/session", func(w http.ResponseWriter, r *http.Request) {
		server.sessionManager.Put(r.Context(), "authenticated", true)
		server.sessionManager.Put(r.Context(), "username", "update-test")
		server.sessionManager.Put(r.Context(), "auth_method", string(domain.AuthMethodPassword))
		w.WriteHeader(204)
	})
	session.Mount("/api", server.apiRoutes())
	for _, tc := range []struct {
		body string
		code int
	}{{`{"id":"release","url":"https://evil.invalid/app.exe"}`, 400}, {`{"id":"release","path":"C:/evil.exe"}`, 400}, {`{"id":"release"}`, 200}} {
		login := httptest.NewRecorder()
		session.ServeHTTP(login, httptest.NewRequest("GET", "/session", nil))
		request := httptest.NewRequest("POST", "/api/ufc/updates/install", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		for _, cookie := range login.Result().Cookies() {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		session.ServeHTTP(response, request)
		require.Equal(t, tc.code, response.Code, response.Body.String())
	}
	require.Equal(t, 1, service.installs)
}
