// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/foundation"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type foundationStub struct{ err error }

type catalogueStub struct{ err error }

func (s catalogueStub) RefreshScope(ctx context.Context, _ string) (*catalogue.RefreshAccepted, error) {
	return s.Refresh(ctx)
}

func (s catalogueStub) Clear(context.Context) error { return s.err }
func (s catalogueStub) Reset(context.Context) error { return s.err }

func (s catalogueStub) MatchingEvents(context.Context) ([]catalogue.Event, error) { return nil, s.err }

func TestMatchingPreviewHTTP(t *testing.T) {
	server := newScopedTestServer()
	server.foundationService = foundationStub{}
	server.catalogueService = catalogueStub{}
	server.apiService.(*apikeyServiceStub).tokens["ufc-read"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 403}, {"config-read", 403}, {"ufc-read", 200}} {
		require.Equal(t, tc.code, serveWithToken(router, "POST", "/ufc/matching/preview", tc.token, `{"title":"UFC.300.Prelims.1080p"}`).Code)
	}
	for _, body := range []string{`{}`, `{"title":" "}`, `{"title":"UFC.300","unknown":true}`, `{"title":"UFC.300"} {}`, `{"title":"` + strings.Repeat("a", 1025) + `"}`} {
		require.Equal(t, 400, serveWithToken(router, "POST", "/ufc/matching/preview", "ufc-read", body).Code)
	}
}

func (s catalogueStub) Create(context.Context, catalogue.EventCreate, string) (catalogue.Event, error) {
	return catalogue.Event{ID: "manual-local", Revision: 1}, s.err
}

func (s catalogueStub) Settings(context.Context) (*catalogue.Settings, error) {
	return &catalogue.Settings{Revision: 1, Templates: catalogue.DefaultTemplates()}, s.err
}

func (s catalogueStub) SaveSettings(context.Context, catalogue.SettingsEdit) (*catalogue.Settings, error) {
	return &catalogue.Settings{Revision: 2, Templates: catalogue.DefaultTemplates()}, s.err
}

func (s catalogueStub) Event(context.Context, string) (catalogue.Event, error) {
	return catalogue.Event{ID: "local-event", Revision: 1}, s.err
}

func (s catalogueStub) Edit(context.Context, string, catalogue.EventEdit, string) (catalogue.Event, error) {
	return catalogue.Event{ID: "local-event", Revision: 2}, s.err
}

func (s catalogueStub) Events(context.Context) (*catalogue.EventList, error) {
	return &catalogue.EventList{Upcoming: []catalogue.EventSummary{}, RecentCompleted: []catalogue.EventSummary{}}, s.err
}

func (s catalogueStub) Status(context.Context) (*catalogue.Status, error) {
	return &catalogue.Status{Source: "wikipedia.en"}, s.err
}
func (s catalogueStub) Refresh(context.Context) (*catalogue.RefreshAccepted, error) {
	return &catalogue.RefreshAccepted{}, s.err
}

func TestCatalogueScopeIsolation(t *testing.T) {
	server := newScopedTestServer()
	server.foundationService = foundationStub{}
	server.catalogueService = catalogueStub{}
	server.apiService.(*apikeyServiceStub).tokens["ufc-read"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	server.apiService.(*apikeyServiceStub).tokens["ufc-invoke"] = &domain.APIKey{Scopes: []string{"ufc:invoke"}}
	server.apiService.(*apikeyServiceStub).tokens["ufc-write"] = &domain.APIKey{Scopes: []string{"ufc:write"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, tc := range []struct {
		token         string
		read, refresh int
	}{{"", 403, 403}, {"config-read", 403, 403}, {"ufc-read", 200, 403}, {"ufc-invoke", 403, 202}, {"ufc-write", 200, 202}} {
		require.Equal(t, tc.read, serveWithToken(router, "GET", "/ufc/catalogue/status", tc.token, "").Code)
		require.Equal(t, tc.read, serveWithToken(router, "GET", "/ufc/events", tc.token, "").Code)
		require.Equal(t, tc.read, serveWithToken(router, "GET", "/ufc/events/local-event", tc.token, "").Code)
		require.Equal(t, tc.refresh, serveWithToken(router, "POST", "/ufc/catalogue/refresh", tc.token, "").Code)
		wantEdit := 403
		if tc.token == "ufc-write" {
			wantEdit = 200
		}
		require.Equal(t, wantEdit, serveWithToken(router, "PATCH", "/ufc/events/local-event", tc.token, `{"revision":1,"name":{"operation":"set","value":"Manual"}}`).Code)
		wantCreate := 403
		if tc.token == "ufc-write" {
			wantCreate = 201
		}
		require.Equal(t, wantCreate, serveWithToken(router, "POST", "/ufc/events", tc.token, `{"values":{"name":"Local","type":"numbered","status":"scheduled"},"aliases":[]}`).Code)
		require.Equal(t, 403, serveWithToken(router, "GET", "/ufc/catalogue/settings", tc.token, "").Code)
		require.Equal(t, 403, serveWithToken(router, "PUT", "/ufc/catalogue/settings", tc.token, `{"revision":1,"templates":{"numbered":[],"fight_night":[]}}`).Code)
	}
	server.catalogueService = catalogueStub{err: errors.New("mock database unavailable")}
	require.Equal(t, 500, serveWithToken(router, "GET", "/ufc/catalogue/status", "ufc-read", "").Code)
	key := &domain.APIKey{Scopes: []string{"ufc:invoke"}}
	require.NoError(t, key.Validate())
}

func (s foundationStub) Status(context.Context) (*foundation.Status, error) {
	return &foundation.Status{ProductSlug: "ufc-manager", ProductName: "UFC Manager", SchemaReady: true, SettingsRevision: 1}, s.err
}

func TestUFCStatusAccess(t *testing.T) {
	server := newScopedTestServer()
	server.sessionManager.Lifetime = 30 * 24 * time.Hour
	server.foundationService = foundationStub{}
	server.catalogueService = catalogueStub{}
	server.apiService.(*apikeyServiceStub).tokens["ufc"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, tc := range []struct {
		token string
		code  int
	}{{"", 403}, {"config-read", 403}, {"ufc", 200}} {
		result := serveWithToken(router, "GET", "/ufc/status", tc.token, "")
		require.Equal(t, tc.code, result.Code)
		if tc.code == 200 {
			require.Contains(t, result.Body.String(), `"processing_enabled":false`)
			require.Contains(t, result.Body.String(), `"automatic_acquisition_enabled":false`)
		}
	}
	sessionRouter := chi.NewRouter()
	sessionRouter.Use(server.sessionManager.LoadAndSave)
	sessionRouter.Get("/session", func(w http.ResponseWriter, r *http.Request) {
		server.sessionManager.Put(r.Context(), "authenticated", true)
		server.sessionManager.Put(r.Context(), "username", "foundation-user")
		server.sessionManager.Put(r.Context(), "auth_method", string(domain.AuthMethodPassword))
		w.WriteHeader(204)
	})
	sessionRouter.Mount("/api", server.apiRoutes())
	login := httptest.NewRecorder()
	sessionRouter.ServeHTTP(login, httptest.NewRequest("GET", "/session", nil))
	request := httptest.NewRequest("GET", "/api/ufc/status", nil)
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	result := httptest.NewRecorder()
	sessionRouter.ServeHTTP(result, request)
	require.Equal(t, 200, result.Code)
	request = httptest.NewRequest("PATCH", "/api/ufc/events/local-event", strings.NewReader(`{"revision":1,"name":{"operation":"set","value":"Session edit"}}`))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	result = httptest.NewRecorder()
	sessionRouter.ServeHTTP(result, request)
	require.Equal(t, 200, result.Code)
	for _, method := range []string{"GET", "PUT"} {
		request = httptest.NewRequest(method, "/api/ufc/catalogue/settings", strings.NewReader(`{"revision":1,"templates":{"numbered":[],"fight_night":[]}}`))
		request.Header.Set("Content-Type", "application/json")
		for _, cookie := range login.Result().Cookies() {
			request.AddCookie(cookie)
		}
		result = httptest.NewRecorder()
		sessionRouter.ServeHTTP(result, request)
		require.Equal(t, 200, result.Code)
	}
	request = httptest.NewRequest("POST", "/api/ufc/catalogue/refresh", nil)
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	result = httptest.NewRecorder()
	sessionRouter.ServeHTTP(result, request)
	require.Equal(t, 202, result.Code)
	request = httptest.NewRequest("GET", "/api/ufc/catalogue/status", nil)
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	result = httptest.NewRecorder()
	sessionRouter.ServeHTTP(result, request)
	require.Equal(t, 200, result.Code)
	server.foundationService = foundationStub{err: errors.New("database unavailable")}
	require.Equal(t, 500, serveWithToken(router, "GET", "/ufc/status", "ufc", "").Code)
}

func TestLegacyDoesNotMountUFC(t *testing.T) {
	server := newScopedTestServer()
	result := serveWithToken(server.apiRoutes(), "GET", "/ufc/status", "config-read", "")
	require.Equal(t, 404, result.Code)
	require.Equal(t, 404, serveWithToken(server.apiRoutes(), "GET", "/ufc/events", "config-read", "").Code)
	key := &domain.APIKey{Scopes: []string{"ufc:read"}}
	require.NoError(t, key.Validate())
	key.Scopes = []string{"ufc:write"}
	require.NoError(t, key.Validate())
	require.Equal(t, 404, serveWithToken(server.apiRoutes(), "PATCH", "/ufc/events/local", "config-read", "{}").Code)
}

func TestCatalogueRetainsBrowserGuards(t *testing.T) {
	server := newScopedTestServer()
	server.foundationService = foundationStub{}
	server.catalogueService = catalogueStub{}
	server.config.Config.AuthDisabled = true
	server.config.Config.AuthDisabledAcknowledgement = domain.AuthDisabledAcknowledgementValue
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	for _, tc := range []struct {
		origin string
		code   int
	}{{"http://example.com", 202}, {"https://untrusted.example", 403}} {
		r := httptest.NewRequest("POST", "http://example.com/ufc/catalogue/refresh", nil)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", tc.origin)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, r)
		require.Equal(t, tc.code, rec.Code)
		r = httptest.NewRequest("PATCH", "http://example.com/ufc/events/local-event", strings.NewReader(`{"revision":1,"date":{"operation":"set"}}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", tc.origin)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, r)
		want := 200
		if tc.code == 403 {
			want = 403
		}
		require.Equal(t, want, rec.Code)
	}
}
