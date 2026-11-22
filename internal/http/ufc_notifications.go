package http

import (
	"context"
	"net/http"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/notifications"
	"github.com/go-chi/chi/v5"
)

type ufcNotificationService interface {
	Settings(context.Context) (notifications.Settings, error)
	Save(context.Context, notifications.Edit) (notifications.Settings, error)
	History(context.Context) ([]notifications.Delivery, error)
	Test(context.Context, domain.NotificationEvent) error
}

func (s *Server) ufcNotificationRoutes(r chi.Router, encoder encoder) {
	service := s.ufcNotificationService
	if service == nil {
		return
	}
	r.With(requireUser).Get("/notifications", func(w http.ResponseWriter, r *http.Request) {
		value, err := service.Settings(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, value)
	})
	r.With(requireUser).Put("/notifications", func(w http.ResponseWriter, r *http.Request) {
		var edit notifications.Edit
		if err := libraryJSON(w, r, &edit); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		value, err := service.Save(r.Context(), edit)
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, value)
	})
	r.With(requireUser).Get("/notifications/history", func(w http.ResponseWriter, r *http.Request) {
		value, err := service.History(r.Context())
		if err != nil {
			catalogueError(encoder, w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, value)
	})
	r.With(requireUser).Post("/notifications/test", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Event domain.NotificationEvent `json:"event"`
		}
		if err := libraryJSON(w, r, &input); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := service.Test(ctx, input.Event); err != nil {
			encoder.BadRequestErr(w, err)
			return
		}
		encoder.StatusResponse(w, http.StatusOK, map[string]bool{"sent": true})
	})
}
