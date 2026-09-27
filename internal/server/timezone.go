package server

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/timezone"
)

type timeZoneHTTP struct{ service *timezone.Service }

func (h *timeZoneHTTP) register(router chi.Router) {
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
	router.Get("/", h.get)
	router.Patch("/", h.save)
}

func (h *timeZoneHTTP) get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"time_zone": h.service.Name()})
}

func (h *timeZoneHTTP) save(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TimeZone *string `json:"time_zone"`
	}
	if !decodeJSONLimit(w, r, &input, 512) {
		return
	}
	if input.TimeZone == nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_time_zone"})
		return
	}
	name, err := h.service.Save(r.Context(), *input.TimeZone)
	if err != nil {
		if errors.Is(err, timezone.ErrInput) {
			writeJSON(w, 400, map[string]string{"error": "invalid_time_zone"})
			return
		}
		writeJSON(w, 503, map[string]string{"error": "time_zone_storage_failed"})
		return
	}
	writeJSON(w, 200, map[string]string{"time_zone": name})
}
