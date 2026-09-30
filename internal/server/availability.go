package server

import (
	"errors"
	"net/http"

	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/upstream"
)

func (h *groupHTTP) availability(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_group_input"})
		return
	}
	readAvailability(w, r, h.gateway, sessionUser(r).ID, id, false, nil)
}
func (h *keyHTTP) availability(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		keyError(w, apikey.ErrInput)
		return
	}
	userID := sessionUser(r).ID
	check := func() error { _, err := h.service.CatalogGroup(r.Context(), userID, id); return err }
	groupID, err := h.service.CatalogGroup(r.Context(), userID, id)
	if err != nil {
		keyError(w, err)
		return
	}
	readAvailability(w, r, h.gateway, userID, groupID, true, check)
}
func readAvailability(w http.ResponseWriter, r *http.Request, service *gateway.Service, userID, groupID int64, personal bool, recheck func() error) {
	if service == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	value, err := service.Availability(r.Context(), userID, groupID, r.URL.Query().Get("model"))
	if err != nil {
		if errors.Is(err, upstream.ErrInput) {
			writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		} else {
			groupError(w, err)
		}
		return
	}
	if recheck != nil {
		if err := recheck(); err != nil {
			keyError(w, err)
			return
		}
	}
	// A personal key never discloses subscription identities, even for an administrator.
	if personal {
		value.Accounts = nil
	}
	writeJSON(w, 200, value)
}
