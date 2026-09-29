package server

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/alerts"
	"net/http"
)

type alertHTTP struct {
	service  *alerts.Service
	tenantID int64
}

func (h *alertHTTP) register(router chi.Router) {
	routeErrors(router)
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h.service == nil {
				writeJSON(w, 503, map[string]string{"error": "unavailable"})
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		value, err := h.service.State(r.Context(), h.tenantID)
		h.respond(w, value, err)
	})
	router.Put("/", func(w http.ResponseWriter, r *http.Request) {
		var input alerts.Input
		if !decodeJSONLimit(w, r, &input, 4096) {
			return
		}
		value, err := h.service.Update(r.Context(), h.tenantID, input)
		h.respond(w, value, err)
	})
}
func (h *alertHTTP) respond(w http.ResponseWriter, value alerts.State, err error) {
	if err == nil {
		writeJSON(w, 200, value)
		return
	}
	if errors.Is(err, alerts.ErrInput) {
		writeJSON(w, 400, map[string]string{"error": "invalid_alert_settings"})
		return
	}
	writeJSON(w, 503, map[string]string{"error": "unavailable"})
}
