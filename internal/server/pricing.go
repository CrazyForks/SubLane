package server

import (
	"net/http"

	"github.com/murongg/SubLane/internal/pricing"
)

type pricingHTTP struct {
	service *pricing.Service
}

func (h *pricingHTTP) catalog(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"unit":   "micro_usd_per_million_tokens",
		"prices": h.service.List(),
	})
}
