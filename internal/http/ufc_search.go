// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/search"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/go-chi/chi/v5"
)

type ufcSearchService interface {
	Sources(context.Context) ([]search.Source, error)
	Search(context.Context, search.Request) (*search.Response, error)
}

func (s *Server) ufcSearchRoutes(r chi.Router, encoder encoder) {
	if s.ufcSearchService == nil {
		return
	}
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/search/sources", func(w http.ResponseWriter, r *http.Request) {
		sources, err := s.ufcSearchService.Sources(r.Context())
		if err != nil {
			encoder.Error(w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, sources)
	})
	r.With(requireScope(domain.APIResourceUFC, domain.APIAccessInvoke)).Post("/events/{id}/search", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		var input search.Request
		if err := decoder.Decode(&input); err != nil {
			encoder.BadRequestErr(w, errors.New("invalid event search request"))
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			encoder.BadRequestErr(w, errors.New("expected one search request"))
			return
		}
		input.EventID = chi.URLParam(r, "id")
		input.Actor = catalogueActor(r.Context())
		result, err := s.ufcSearchService.Search(r.Context(), input)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, result)
	})
}
