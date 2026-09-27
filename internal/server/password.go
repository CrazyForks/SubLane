package server

import (
	"net/http"
	"time"

	"github.com/murongg/SubLane/internal/auth"
)

func (h *authHTTP) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Current  string `json:"current_password"`
		Password string `json:"new_password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.service.ChangePassword(r.Context(), sessionUser(r).ID, input.Current, input.Password); err != nil {
		authError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.secure(r), MaxAge: -1, Expires: time.Unix(1, 0)})
	writeJSON(w, 200, auth.State{Initialized: true})
}

func (h *authHTTP) resetMemberPassword(w http.ResponseWriter, r *http.Request) {
	if !platformOwner(h.tenantID, sessionUser(r)) {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id, err := pathID(r)
	if err != nil || id <= 0 {
		authError(w, auth.ErrInput)
		return
	}
	var input struct {
		Password string `json:"new_password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.service.ResetMemberPassword(r.Context(), id, input.Password); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
