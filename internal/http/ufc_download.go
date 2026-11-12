// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/go-chi/chi/v5"
)

type ufcDownloadService interface {
	Recover(context.Context, string, acquisition.Recovery) (*acquisition.Job, error)
	Remove(context.Context, string) error
	Clients(context.Context) ([]acquisition.ClientOption, error)
	Jobs(context.Context) ([]acquisition.Job, error)
	Submit(context.Context, acquisition.Submit) (*acquisition.Accepted, error)
	Reconcile(context.Context) error
}

func (s *Server) ufcDownloadRoutes(r chi.Router, encoder encoder) {
	if s.ufcDownloadService == nil {
		return
	}
	read := requireScope(domain.APIResourceUFC, domain.APIAccessRead)
	invoke := requireScope(domain.APIResourceUFC, domain.APIAccessInvoke)
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessWrite)).Delete("/downloads/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.ufcDownloadService.Remove(r.Context(), chi.URLParam(r, "id")); err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, struct {
			Removed bool `json:"removed"`
		}{true})
	})
	r.With(read).Get("/downloads/clients", func(w http.ResponseWriter, r *http.Request) {
		clients, err := s.ufcDownloadService.Clients(r.Context())
		if err != nil {
			encoder.Error(w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, clients)
	})
	r.With(read).Get("/downloads", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := s.ufcDownloadService.Jobs(r.Context())
		if err != nil {
			encoder.Error(w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, jobs)
	})
	r.With(invoke).Post("/downloads", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		var input acquisition.Submit
		if err := decoder.Decode(&input); err != nil || len(input.CandidateID) > 256 || input.CandidateID == "" {
			encoder.BadRequestErr(w, errors.New("invalid selected download"))
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			encoder.BadRequestErr(w, errors.New("expected one download request"))
			return
		}
		input.Actor = catalogueActor(r.Context())
		accepted, err := s.ufcDownloadService.Submit(r.Context(), input)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusAccepted, accepted)
	})
	r.With(invoke).Post("/downloads/{id}/recover", func(w http.ResponseWriter, r *http.Request) {
		var input acquisition.Recovery
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, errors.New("invalid recovery request"))
			return
		}
		input.Actor = catalogueActor(r.Context())
		job, err := s.ufcDownloadService.Recover(r.Context(), chi.URLParam(r, "id"), input)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, job)
	})
	r.With(invoke).Post("/downloads/reconcile", func(w http.ResponseWriter, r *http.Request) {
		if err := s.ufcDownloadService.Reconcile(r.Context()); err != nil {
			encoder.Error(w, errors.New("download status refresh could not complete"))
			return
		}
		encoder.StatusResponse(w, http.StatusOK, struct {
			Refreshed bool `json:"refreshed"`
		}{true})
	})
}
