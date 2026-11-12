// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestUFCWebEntrypoint(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html>foundation application</html>")}, "assets/app.js": {Data: []byte("export {}")}}
	for _, legacy := range []bool{false, true} {
		router := chi.NewRouter()
		if legacy {
			newWebLegacyHandler(zerolog.Nop(), assets, "test", "/", "./").RegisterRoutes(router)
		} else {
			newWebHandler(zerolog.Nop(), assets, "test", "/", "./").RegisterRoutes(router)
		}
		for _, path := range []string{"/ufc", "/filters", "/releases", "/settings", "/logs"} {
			result := httptest.NewRecorder()
			router.ServeHTTP(result, httptest.NewRequest("GET", path, nil))
			require.Equal(t, 200, result.Code, "legacy=%v path=%s", legacy, path)
			require.Contains(t, result.Body.String(), "foundation application")
		}
	}
}
