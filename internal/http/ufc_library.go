// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/go-chi/chi/v5"
)

type ufcLibraryService interface {
	Clear(context.Context, int64) error
	Settings(context.Context) (*library.Settings, error)
	SaveSettings(context.Context, library.SettingsEdit) (*library.Settings, error)
	View(context.Context) (*library.View, error)
	Files(context.Context, string, int) (*library.FilePage, error)
	Events(context.Context) ([]library.EventOption, error)
	Edit(context.Context, string, library.FileEdit, string) (library.File, error)
	Scan(context.Context) (*library.Accepted, error)
}

func libraryJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return errors.New("invalid library request")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected one library request")
	}
	return nil
}

func (s *Server) ufcLibraryRoutes(r chi.Router, encoder encoder) {
	if s.ufcLibraryService == nil {
		return
	}
	read := requireScope(domain.APIResourceUFC, domain.APIAccessRead)
	write := requireScope(domain.APIResourceUFC, domain.APIAccessWrite)
	invoke := requireScope(domain.APIResourceUFC, domain.APIAccessInvoke)
	r.With(write).Post("/library/clear", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision int64 `json:"revision"`
		}
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		if err := s.ufcLibraryService.Clear(r.Context(), input.Revision); err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, struct {
			Cleared bool `json:"cleared"`
		}{true})
	})
	r.With(read).Get("/library", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.ufcLibraryService.View(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, v)
	})
	r.With(read).Get("/library/settings", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.ufcLibraryService.Settings(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, v)
	})
	r.With(requireUser).Put("/library/settings", func(w http.ResponseWriter, r *http.Request) {
		var input library.SettingsEdit
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		v, err := s.ufcLibraryService.SaveSettings(r.Context(), input)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, v)
	})
	r.With(invoke).Post("/library/scan", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.ufcLibraryService.Scan(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusAccepted, v)
	})
	r.With(read).Get("/library/files", func(w http.ResponseWriter, r *http.Request) {
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil {
				encoder.BadRequestErr(w, errors.New("invalid file page"))
				return
			}
		}
		v, err := s.ufcLibraryService.Files(r.Context(), r.URL.Query().Get("group"), offset)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, v)
	})
	r.With(read).Get("/library/events", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.ufcLibraryService.Events(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, v)
	})
	r.With(write).Patch("/library/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input library.FileEdit
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		v, err := s.ufcLibraryService.Edit(r.Context(), chi.URLParam(r, "id"), input, catalogueActor(r.Context()))
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, v)
	})
	r.With(read).Get("/library/coverage", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.ufcLibraryService.View(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		type coverage struct {
			Wanted      map[string][]string          `json:"wanted"`
			Configured  bool                         `json:"configured"`
			Running     bool                         `json:"running"`
			Unavailable bool                         `json:"unavailable"`
			Review      int                          `json:"review"`
			Events      map[string]map[string]string `json:"events"`
		}
		out := coverage{Configured: v.Configured, Running: v.Settings.Running, Wanted: v.Wanted, Events: map[string]map[string]string{}}
		for _, root := range v.Settings.Roots {
			if root.Enabled && root.State != "available" {
				out.Unavailable = true
			}
		}
		for _, g := range v.Groups {
			out.Review += g.Review
			if g.EventID != "" {
				out.Events[g.EventID] = g.Components
			}
		}
		encoder.StatusResponse(w, http.StatusOK, out)
	})
}
