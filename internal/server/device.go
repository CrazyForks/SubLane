package server

import "net/http"

func (h *accountHTTP) pollOAuth(w http.ResponseWriter, r *http.Request) {
	if h.oauth == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	var input struct {
		State string `json:"state"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.oauth.PollDevice(r.Context(), token(r), input.State)
	if err != nil {
		accountError(w, err)
		return
	}
	writeJSON(w, 200, result)
	if result.Account != nil && h.gateway != nil {
		_, _ = h.gateway.AccountCatalog(r.Context(), result.Account.ID, false)
	}
}
