// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"net/http"

	"github.com/autobrr/autobrr/internal/ufc/updater"

	"github.com/go-chi/chi/v5"
)

type ufcUpdateService interface {
	Status(context.Context) (updater.Status, error)
	Check(context.Context) (updater.Status, error)
	Install(context.Context, string) (updater.Status, error)
	Preferences(context.Context, int64, bool) (updater.Status, error)
}

func (s *Server) ufcUpdateRoutes(r chi.Router, encoder encoder) {
	if s.ufcUpdateService == nil {
		return
	}
	r.Route("/updates", func(r chi.Router) {
		r.Use(requireUser)
		respond := func(w http.ResponseWriter, status updater.Status, err error) {
			if err != nil {
				catalogueError(encoder, w, err)
				return
			}
			encoder.StatusResponse(w, http.StatusOK, status)
		}
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			status, err := s.ufcUpdateService.Status(r.Context())
			respond(w, status, err)
		})
		r.Post("/check", func(w http.ResponseWriter, r *http.Request) {
			status, err := s.ufcUpdateService.Check(r.Context())
			respond(w, status, err)
		})
		r.Post("/install", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				ID string `json:"id"`
			}
			if err := libraryJSON(w, r, &input); err != nil {
				encoder.BadRequestErr(w, err)
				return
			}
			status, err := s.ufcUpdateService.Install(r.Context(), input.ID)
			respond(w, status, err)
		})
		r.Put("/settings", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Revision int64 `json:"revision"`
				Auto     bool  `json:"auto_install"`
			}
			if err := libraryJSON(w, r, &input); err != nil {
				encoder.BadRequestErr(w, err)
				return
			}
			status, err := s.ufcUpdateService.Preferences(r.Context(), input.Revision, input.Auto)
			respond(w, status, err)
		})
	})
}
