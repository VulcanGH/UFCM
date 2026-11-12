// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"net/http"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/automation"
	"github.com/go-chi/chi/v5"
)

type ufcAutomationService interface {
	Settings(context.Context) (*automation.Settings, error)
	SaveSettings(context.Context, automation.Settings) (*automation.Settings, error)
}

func (s *Server) ufcAutomationRoutes(r chi.Router, encoder encoder) {
	if s.ufcAutomationService == nil {
		return
	}
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/automation/settings", func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.ufcAutomationService.Settings(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, settings)
	})
	r.With(requireUser).Put("/automation/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision int  `json:"revision"`
			Enabled  bool `json:"enabled"`
		}
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		settings, err := s.ufcAutomationService.SaveSettings(r.Context(), automation.Settings{Revision: input.Revision, Enabled: input.Enabled})
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, settings)
	})
}
