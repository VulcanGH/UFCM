// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/foundation"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/pkg/errors"

	"github.com/go-chi/chi/v5"
)

type foundationService interface {
	Status(context.Context) (*foundation.Status, error)
}

type catalogueService interface {
	Reset(context.Context) error
	RefreshScope(context.Context, string) (*catalogue.RefreshAccepted, error)
	MatchingEvents(context.Context) ([]catalogue.Event, error)
	Clear(context.Context) error
	Create(context.Context, catalogue.EventCreate, string) (catalogue.Event, error)
	Settings(context.Context) (*catalogue.Settings, error)
	SaveSettings(context.Context, catalogue.SettingsEdit) (*catalogue.Settings, error)
	Event(context.Context, string) (catalogue.Event, error)
	Edit(context.Context, string, catalogue.EventEdit, string) (catalogue.Event, error)
	Events(context.Context) (*catalogue.EventList, error)
	Status(context.Context) (*catalogue.Status, error)
	Refresh(context.Context) (*catalogue.RefreshAccepted, error)
}

func (s *Server) ufcRoutes(encoder encoder) func(chi.Router) {
	return func(r chi.Router) {
		s.ufcUpdateRoutes(r, encoder)
		s.ufcAutomationRoutes(r, encoder)
		s.ufcLibraryRoutes(r, encoder)
		s.ufcProcessingRoutes(r, encoder)
		s.ufcDownloadRoutes(r, encoder)
		s.ufcSearchRoutes(r, encoder)
		if s.catalogueService != nil {
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessWrite)).Post("/catalogue/reset", func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Confirm bool `json:"confirm"`
				}
				if err := libraryJSON(w, r, &input); err != nil || !input.Confirm {
					encoder.BadRequestErr(w, errors.New("catalogue reset requires confirmation"))
					return
				}
				if err := s.catalogueService.Reset(r.Context()); err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, struct {
					Reset bool `json:"reset"`
				}{true})
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/catalogue/events", func(w http.ResponseWriter, r *http.Request) {
				search := r.URL.Query().Get("search")
				offset := 0
				limit := 20
				var err error
				if value := r.URL.Query().Get("offset"); value != "" {
					offset, err = strconv.Atoi(value)
				}
				if value := r.URL.Query().Get("limit"); value != "" && err == nil {
					limit, err = strconv.Atoi(value)
				}
				if err != nil || offset < 0 || offset > 100000 || limit < 1 || limit > 2500 || len(search) > 256 {
					encoder.BadRequestErr(w, errors.New("invalid catalogue page"))
					return
				}
				events, err := s.catalogueService.MatchingEvents(r.Context())
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, catalogue.BrowseWindow(events, search, offset, limit, r.URL.Query().Get("past") == "1", time.Now().Format("2006-01-02")))
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/matching/terms", func(w http.ResponseWriter, r *http.Request) {
				encoder.StatusResponse(w, http.StatusOK, matching.MainCardTerms())
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Post("/matching/preview", func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
				decoder.DisallowUnknownFields()
				var input struct {
					Title string `json:"title"`
				}
				if err := decoder.Decode(&input); err != nil || len(input.Title) > 1024 || len(strings.TrimSpace(input.Title)) == 0 {
					encoder.BadRequestErr(w, errors.New("provide a release title of 1 to 1024 bytes"))
					return
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					encoder.BadRequestErr(w, errors.New("expected one preview request"))
					return
				}
				events, err := s.catalogueService.MatchingEvents(r.Context())
				if err != nil {
					encoder.Error(w, err)
					return
				}
				settings, err := s.catalogueService.Settings(r.Context())
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, matching.MatchWithTerms(input.Title, events, settings.MainCardTerms))
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessWrite)).Delete("/events", func(w http.ResponseWriter, r *http.Request) {
				if err := s.catalogueService.Clear(r.Context()); err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, struct {
					Cleared bool `json:"cleared"`
				}{true})
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessWrite)).Post("/events", func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
				decoder.DisallowUnknownFields()
				var input *catalogue.EventCreate
				if err := decoder.Decode(&input); err != nil || input == nil {
					encoder.BadRequestErr(w, errors.New("invalid manual event JSON"))
					return
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					encoder.BadRequestErr(w, errors.New("expected one manual event"))
					return
				}
				actor := catalogueActor(r.Context())
				event, err := s.catalogueService.Create(r.Context(), *input, actor)
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusCreated, event)
			})
			r.With(requireUser).Get("/catalogue/settings", func(w http.ResponseWriter, r *http.Request) {
				settings, err := s.catalogueService.Settings(r.Context())
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, settings)
			})
			r.With(requireUser).Put("/catalogue/settings", func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
				decoder.DisallowUnknownFields()
				var edit *catalogue.SettingsEdit
				if err := decoder.Decode(&edit); err != nil || edit == nil {
					encoder.BadRequestErr(w, errors.New("invalid component settings JSON"))
					return
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					encoder.BadRequestErr(w, errors.New("expected one settings edit"))
					return
				}
				settings, err := s.catalogueService.SaveSettings(r.Context(), *edit)
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, settings)
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/events/{id}", func(w http.ResponseWriter, r *http.Request) {
				event, err := s.catalogueService.Event(r.Context(), chi.URLParam(r, "id"))
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, event)
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessWrite)).Patch("/events/{id}", func(w http.ResponseWriter, r *http.Request) {
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
				decoder.DisallowUnknownFields()
				var edit *catalogue.EventEdit
				if err := decoder.Decode(&edit); err != nil || edit == nil {
					encoder.BadRequestErr(w, errors.New("invalid event edit JSON"))
					return
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					encoder.BadRequestErr(w, errors.New("expected one event edit"))
					return
				}
				caller, ok := callerFromContext(r.Context())
				if !ok {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				actor := "user:" + caller.Username
				if caller.APIKey != nil {
					actor = "api_key:" + strconv.Itoa(caller.APIKey.ID)
				}
				event, err := s.catalogueService.Edit(r.Context(), chi.URLParam(r, "id"), *edit, actor)
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, event)
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/events", func(w http.ResponseWriter, r *http.Request) {
				events, err := s.catalogueService.Events(r.Context())
				if err != nil {
					encoder.Error(w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, events)
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/catalogue/status", func(w http.ResponseWriter, r *http.Request) {
				status, err := s.catalogueService.Status(r.Context())
				if err != nil {
					encoder.Error(w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusOK, status)
			})
			r.With(requireScope(domain.APIResourceUFC, domain.APIAccessInvoke)).Post("/catalogue/refresh", func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Scope   string `json:"scope"`
					Confirm bool   `json:"confirm"`
				}
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&input); err != nil && err != io.EOF {
					encoder.BadRequestErr(w, errors.New("invalid refresh request"))
					return
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					encoder.BadRequestErr(w, errors.New("expected one refresh request"))
					return
				}
				if input.Scope == "" {
					input.Scope = "upcoming"
				}
				if input.Scope == "rebuild" && !input.Confirm {
					encoder.BadRequestErr(w, errors.New("historical rebuild requires confirmation"))
					return
				}
				accepted, err := s.catalogueService.RefreshScope(r.Context(), input.Scope)
				if err != nil {
					catalogueError(encoder, w, err)
					return
				}
				encoder.StatusResponse(w, http.StatusAccepted, accepted)
			})
		}
		r.With(requireScope(domain.APIResourceUFC, domain.APIAccessRead)).Get("/status", func(w http.ResponseWriter, r *http.Request) {
			status, err := s.foundationService.Status(r.Context())
			if err != nil {
				encoder.Error(w, err)
				return
			}
			encoder.StatusResponse(w, http.StatusOK, status)
		})
	}
}

func catalogueActor(ctx context.Context) string {
	caller, ok := callerFromContext(ctx)
	if !ok {
		return ""
	}
	if caller.APIKey != nil {
		return "api_key:" + strconv.Itoa(caller.APIKey.ID)
	}
	return "user:" + caller.Username
}

func catalogueError(encoder encoder, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalogue.ErrEventNotFound):
		encoder.NotFoundErr(w, catalogue.ErrEventNotFound)
	case errors.Is(err, catalogue.ErrRevisionConflict):
		encoder.StatusResponse(w, http.StatusConflict, errorResponse{Code: "REVISION_CONFLICT", Message: "This record changed. Review the latest revision before saving again."})
	case errors.Is(err, catalogue.ErrInvalidEdit):
		encoder.BadRequestErr(w, err)
	default:
		encoder.Error(w, err)
	}
}
