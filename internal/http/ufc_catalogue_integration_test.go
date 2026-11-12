// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	upstream "github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	ufcdatabase "github.com/autobrr/autobrr/internal/ufc/database"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type catalogueIntegrationSource struct {
	release chan struct{}
	calls   atomic.Int32
}

func (s *catalogueIntegrationSource) Read(ctx context.Context, _ []catalogue.Facts) (*catalogue.Snapshot, error) {
	if s.calls.Add(1) > 1 {
		return nil, errors.New("mock source unavailable")
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	date := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	return &catalogue.Snapshot{ListRevision: 10, Discovered: 1, Facts: []catalogue.Facts{{PageID: 100, Title: "UFC 336", Name: "UFC 336: Van vs. Kape", Type: "numbered", Date: &date, Status: "scheduled", ArticleRevision: 1, ListRevision: 10, RetrievedAt: time.Now().UTC()}}}, nil
}

func TestCatalogueRefreshHTTPPersistence(t *testing.T) {
	appDB, err := upstream.NewDB(&domain.Config{DatabaseType: "sqlite", DatabaseDSN: filepath.Join(t.TempDir(), "catalogue.db")}, zerolog.Nop())
	require.NoError(t, err)
	require.NoError(t, appDB.Open())
	t.Cleanup(func() { require.NoError(t, appDB.Close()) })
	db := appDB.Handler
	repo := ufcdatabase.New(db, "sqlite", zerolog.Nop())
	require.NoError(t, repo.Migrate(t.Context()))
	source := &catalogueIntegrationSource{release: make(chan struct{})}
	service := catalogue.New(repo, source, zerolog.Nop())
	require.NoError(t, service.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, service.Stop(context.Background())) })
	server := newScopedTestServer()
	server.sessionManager.Lifetime = 30 * 24 * time.Hour
	server.foundationService = foundationStub{}
	server.catalogueService = service
	server.apiService.(*apikeyServiceStub).tokens["catalogue-read"] = &domain.APIKey{Scopes: []string{"ufc:read"}}
	server.apiService.(*apikeyServiceStub).tokens["catalogue-invoke"] = &domain.APIKey{Scopes: []string{"ufc:invoke"}}
	server.apiService.(*apikeyServiceStub).tokens["catalogue-write"] = &domain.APIKey{ID: 42, Scopes: []string{"ufc:write"}}
	router := server.sessionManager.LoadAndSave(server.apiRoutes())
	request := func(method, path, token string) []byte {
		result := serveWithToken(router, method, path, token, "")
		if method == "POST" {
			require.Equal(t, 202, result.Code)
		} else {
			require.Equal(t, 200, result.Code)
		}
		return result.Body.Bytes()
	}
	request("POST", "/ufc/catalogue/refresh", "catalogue-invoke")
	data := request("POST", "/ufc/catalogue/refresh", "catalogue-invoke")
	require.Contains(t, string(data), `"coalesced":true`)
	close(source.release)
	var status catalogue.Status
	require.Eventually(t, func() bool {
		data := request("GET", "/ufc/catalogue/status", "catalogue-read")
		require.NoError(t, json.Unmarshal(data, &status))
		return !status.Running && status.Events == 1
	}, time.Second, 5*time.Millisecond)
	require.False(t, status.PeriodicSyncEnabled)
	var listed catalogue.EventList
	require.NoError(t, json.Unmarshal(request("GET", "/ufc/events", "catalogue-read"), &listed))
	require.Len(t, listed.Upcoming, 1)
	sessions := chi.NewRouter()
	sessions.Use(server.sessionManager.LoadAndSave)
	sessions.Get("/session", func(w http.ResponseWriter, r *http.Request) {
		server.sessionManager.Put(r.Context(), "authenticated", true)
		server.sessionManager.Put(r.Context(), "username", "preferences-test")
		server.sessionManager.Put(r.Context(), "auth_method", string(domain.AuthMethodPassword))
		w.WriteHeader(204)
	})
	sessions.Mount("/api", server.apiRoutes())
	login := httptest.NewRecorder()
	sessions.ServeHTTP(login, httptest.NewRequest("GET", "/session", nil))
	userRequest := func(method, target, body string, want int) []byte {
		r := httptest.NewRequest(method, "/api"+target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		for _, cookie := range login.Result().Cookies() {
			r.AddCookie(cookie)
		}
		result := httptest.NewRecorder()
		sessions.ServeHTTP(result, r)
		require.Equal(t, want, result.Code, result.Body.String())
		return result.Body.Bytes()
	}
	var settings catalogue.Settings
	require.NoError(t, json.Unmarshal(userRequest("GET", "/ufc/catalogue/settings", "", 200), &settings))
	templates := catalogue.DefaultTemplates()
	templates["numbered"] = append(templates["numbered"], "countdown")
	body, err := json.Marshal(catalogue.SettingsEdit{Revision: settings.Revision, Templates: templates})
	require.NoError(t, err)
	userRequest("PUT", "/ufc/catalogue/settings", string(body), 200)
	userRequest("PUT", "/ufc/catalogue/settings", string(body), 409)
	userRequest("PUT", "/ufc/catalogue/settings", `{"revision":1,"periodic_sync_enabled":true}`, 400)
	userRequest("PUT", "/ufc/catalogue/settings", `null`, 400)
	userRequest("PUT", "/ufc/catalogue/settings", `{"revision":1,"templates":{"numbered":["invalid"],"fight_night":[]}}`, 400)
	require.Equal(t, "UFC 336: Van vs. Kape", listed.Upcoming[0].Name)
	require.NotEmpty(t, status.Attribution)
	require.NotNil(t, status.LastSuccess)
	path := "/ufc/events/" + listed.Upcoming[0].ID
	var detail catalogue.Event
	require.NoError(t, json.Unmarshal(request("GET", path, "catalogue-read"), &detail))
	patch := func(body string, expected int) catalogue.Event {
		result := serveWithToken(router, "PATCH", path, "catalogue-write", body)
		require.Equal(t, expected, result.Code, result.Body.String())
		var e catalogue.Event
		if expected == 200 {
			require.NoError(t, json.Unmarshal(result.Body.Bytes(), &e))
		}
		return e
	}
	saved := patch(`{"revision":1,"name":{"operation":"set","value":"Manual UFC"},"date":{"operation":"set","value":null},"headliners":{"operation":"set","names":[]},"status":{"operation":"set","value":"postponed"},"components":{"prelims":"off","embedded":"on"}}`, 200)
	require.Equal(t, "Manual UFC", saved.Values.Name)
	require.Equal(t, "api_key:42", saved.Corrections.Name.Actor)
	require.Nil(t, saved.Values.Date)
	require.Equal(t, map[string]bool{"prelims": false, "embedded": true}, saved.Components.Overrides)
	patch(`{"revision":1,"name":{"operation":"set","value":"Stale value"}}`, 409)
	require.NoError(t, json.Unmarshal(request("GET", path, "catalogue-read"), &detail))
	require.Equal(t, saved.Values, detail.Values)
	for _, body := range []string{`null`, `{} {}`, `{"revision":2,"type":{"operation":"set","value":"invalid"}}`, `{"revision":2,"name":{"operation":"set","value":""}}`, `{"revision":2,"status":{"operation":"set","value":"invalid"}}`, `{"revision":2}`} {
		patch(body, 400)
	}
	reset := patch(`{"revision":2,"name":{"operation":"reset"},"date":{"operation":"reset"},"headliners":{"operation":"reset"},"status":{"operation":"reset"}}`, 200)
	require.Equal(t, detail.Source.Name, reset.Values.Name)
	require.Equal(t, detail.Source.Date, reset.Values.Date)
	require.Equal(t, catalogue.Corrections{}, reset.Corrections)
	require.Equal(t, detail.ID, reset.ID)
	require.Equal(t, saved.Components.Overrides, reset.Components.Overrides)
	patch(`{"revision":3,"components":{"unknown":"on"}}`, 400)
	patch(`{"revision":3,"components":{"prelims":"invalid"}}`, 400)
	require.Equal(t, 404, serveWithToken(router, "GET", "/ufc/events/missing", "catalogue-read", "").Code)
	success := *status.LastSuccess
	request("POST", "/ufc/catalogue/refresh", "catalogue-invoke")
	require.Eventually(t, func() bool {
		data := request("GET", "/ufc/catalogue/status", "catalogue-read")
		require.NoError(t, json.Unmarshal(data, &status))
		return !status.Running && status.LastError != ""
	}, time.Second, 5*time.Millisecond)
	require.Equal(t, 1, status.Events)
	require.Equal(t, success, *status.LastSuccess)
	require.NoError(t, json.Unmarshal(request("GET", "/ufc/events", "catalogue-read"), &listed))
	require.Len(t, listed.Upcoming, 1)
}
