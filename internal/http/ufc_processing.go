// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/processing"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/go-chi/chi/v5"
)

type ufcProcessingService interface {
	Settings(context.Context) (*processing.Settings, error)
	SaveSettings(context.Context, processing.Settings) (*processing.Settings, error)
	Categories(context.Context, int32) ([]string, error)
	Preview(context.Context, string) (*processing.Preview, error)
	Queue(context.Context, string) (processing.Job, error)
	Job(context.Context, string) (processing.Job, error)
	Jobs(context.Context) ([]processing.Job, error)
	Review(context.Context, string) (processing.ReviewOptions, error)
	Retry(context.Context, string) (processing.Job, error)
	RetryCleanup(context.Context, string) (processing.Job, error)
	KeepBoth(context.Context, string, string) (processing.Job, error)
	Skip(context.Context, string) (processing.Job, error)
	Cancel(context.Context, string) (processing.Job, error)
}

func (s *Server) ufcProcessingRoutes(r chi.Router, encoder encoder) {
	if s.ufcProcessingService == nil {
		return
	}
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Post("/processing/preview", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			AcquisitionID string `json:"acquisition_id"`
		}
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		preview, err := s.ufcProcessingService.Preview(r.Context(), input.AcquisitionID)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, preview)
	})
	r.With(requireUser).Post("/processing/jobs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			AcquisitionID string `json:"acquisition_id"`
		}
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		job, err := s.ufcProcessingService.Queue(r.Context(), input.AcquisitionID)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusAccepted, job)
	})
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/processing/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := s.ufcProcessingService.Jobs(r.Context())
		if err != nil {
			encoder.Error(w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, jobs)
	})
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/processing/jobs/{jobID}", func(w http.ResponseWriter, r *http.Request) {
		job, err := s.ufcProcessingService.Job(r.Context(), chi.URLParam(r, "jobID"))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, job)
	})
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/processing/jobs/{jobID}/review", func(w http.ResponseWriter, r *http.Request) {
		options, err := s.ufcProcessingService.Review(r.Context(), chi.URLParam(r, "jobID"))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, options)
	})
	r.With(requireUser).Post("/processing/jobs/{jobID}/retry", func(w http.ResponseWriter, r *http.Request) {
		job, err := s.ufcProcessingService.Retry(r.Context(), chi.URLParam(r, "jobID"))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, job)
	})
	r.With(requireUser).Post("/processing/jobs/{jobID}/cleanup/retry", func(w http.ResponseWriter, r *http.Request) {
		job, err := s.ufcProcessingService.RetryCleanup(r.Context(), chi.URLParam(r, "jobID"))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusAccepted, job)
	})
	r.With(requireUser).Post("/processing/jobs/{jobID}/keep-both", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Destination string `json:"destination"`
		}
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		job, err := s.ufcProcessingService.KeepBoth(r.Context(), chi.URLParam(r, "jobID"), input.Destination)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, job)
	})
	r.With(requireUser).Post("/processing/jobs/{jobID}/skip", func(w http.ResponseWriter, r *http.Request) {
		job, err := s.ufcProcessingService.Skip(r.Context(), chi.URLParam(r, "jobID"))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, job)
	})
	r.With(requireUser).Post("/processing/jobs/{jobID}/cancel", func(w http.ResponseWriter, r *http.Request) {
		job, err := s.ufcProcessingService.Cancel(r.Context(), chi.URLParam(r, "jobID"))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, job)
	})
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/processing/settings", func(w http.ResponseWriter, r *http.Request) {
		settings, err := s.ufcProcessingService.Settings(r.Context())
		if err != nil {
			encoder.Error(w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, settings)
	})
	r.With(requireUser).Put("/processing/settings", func(w http.ResponseWriter, r *http.Request) {
		var input processing.Settings
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		settings, err := s.ufcProcessingService.SaveSettings(r.Context(), input)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, settings)
	})
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/processing/categories/{clientID}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "clientID"), 10, 32)
		if err != nil || id < 1 {
			encoder.BadRequestErr(w, errors.New("invalid SAB client"))
			return
		}
		categories, err := s.ufcProcessingService.Categories(r.Context(), int32(id))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, categories)
	})
}
