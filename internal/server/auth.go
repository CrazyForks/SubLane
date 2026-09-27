package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/tenants"
	"github.com/murongg/SubLane/internal/timezone"
)

const sessionCookie = "sublane_session"

type authHTTP struct {
	audit     *audit.Service
	service   *auth.Service
	tenants   *tenants.Service
	timeZone  *timezone.Service
	tenantID  int64
	publicURL string
	limiter   *loginLimiter
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupCredentials struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	WorkspaceName string `json:"workspace_name"`
}

func (h *authHTTP) register(router chi.Router) {
	routeErrors(router)
	router.NotFound(h.requireAdmin(http.HandlerFunc(notFound)).ServeHTTP)
	router.Group(func(public chi.Router) {
		public.Use(h.requireAvailable)
		public.Get("/state", h.state)
		public.With(h.requireOrigin, h.throttleLogin).Post("/setup", func(w http.ResponseWriter, r *http.Request) { h.authenticate(w, r, true) })
		public.With(h.requireOrigin, h.throttleLogin).Post("/login", func(w http.ResponseWriter, r *http.Request) { h.authenticate(w, r, false) })
		public.With(h.requireOrigin, h.throttleLogin).Post("/register", h.registerInvitation)
		public.With(h.requireOrigin).Post("/logout", h.logout)
	})
}

func (h *authHTTP) requireAvailable(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.available(w) {
			next.ServeHTTP(w, r)
		}
	})
}

func (h *authHTTP) requireOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.sameOrigin(w, r) {
			next.ServeHTTP(w, r)
		}
	})
}

func (h *authHTTP) throttleLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retry := h.limiter.allow(peerAddress(r.RemoteAddr)); retry > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeJSON(w, 429, map[string]string{"error": "rate_limited"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *authHTTP) available(w http.ResponseWriter) bool {
	if h.service != nil {
		return true
	}
	writeJSON(w, 503, map[string]string{"error": "unavailable"})
	return false
}

func token(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *authHTTP) secure(r *http.Request) bool {
	return r.TLS != nil || strings.HasPrefix(h.publicURL, "https://")
}

func (h *authHTTP) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	expected := h.publicURL
	if expected == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	if r.Header.Get("Origin") == expected && r.Header.Get("Sec-Fetch-Site") != "cross-site" {
		return true
	}
	writeJSON(w, 403, map[string]string{"error": "origin_rejected"})
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	return decodeJSONLimit(w, r, into, 4096)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, into any, limitBytes int64) bool {
	// Authentication bodies are small; a slow sender must not hold a handler indefinitely.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		writeJSON(w, 415, map[string]string{"error": "json_required"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limitBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(into)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		writeJSON(w, 413, map[string]string{"error": "body_too_large"})
	} else {
		writeJSON(w, 400, map[string]string{"error": "invalid_input"})
	}
	return false
}

func (h *authHTTP) state(w http.ResponseWriter, r *http.Request) {
	state, err := h.stateForTenant(r.Context(), token(r))
	if err != nil {
		authError(w, err)
		return
	}
	h.writeState(w, 200, state)
}

func (h *authHTTP) writeState(w http.ResponseWriter, status int, state auth.State) {
	name := "UTC"
	if h.timeZone != nil {
		name = h.timeZone.Name()
	}
	writeJSON(w, status, struct {
		auth.State
		TimeZone string `json:"time_zone"`
	}{State: state, TimeZone: name})
}

func (h *authHTTP) stateForTenant(ctx context.Context, sessionToken string) (auth.State, error) {
	state, err := h.service.State(ctx, sessionToken)
	if err != nil || state.User == nil {
		return state, err
	}
	if h.tenants == nil {
		if h.tenantID > 1 {
			state.User = nil
			state.NeedsWorkspace = true
		}
		return state, nil
	}
	membership, active, err := h.tenants.Membership(ctx, h.tenantID, state.User.ID)
	if err != nil {
		return auth.State{}, err
	}
	if !active {
		state.User = nil
		state.NeedsWorkspace = true
		return state, nil
	}
	if membership.Role == tenants.RoleOwner || membership.Role == tenants.RoleAdmin {
		state.User.Role = auth.RoleAdmin
	} else {
		state.User.Role = auth.RoleMember
	}
	workspaces, err := h.tenants.List(ctx, state.User.ID)
	if err != nil {
		return auth.State{}, err
	}
	state.WorkspaceCount = len(workspaces)
	return state, nil
}

func (h *authHTTP) authenticate(w http.ResponseWriter, r *http.Request, setup bool) {
	var input credentials
	var workspaceName string
	if setup {
		var initial setupCredentials
		if !decodeJSON(w, r, &initial) {
			return
		}
		input = credentials{Username: initial.Username, Password: initial.Password}
		workspaceName = initial.WorkspaceName
	} else if !decodeJSON(w, r, &input) {
		return
	}
	var session auth.Session
	var err error
	status := 200
	if setup {
		session, err = h.service.Setup(r.Context(), input.Username, input.Password, workspaceName)
		status = 201
	} else {
		session, err = h.service.Login(r.Context(), input.Username, input.Password)
	}
	if err != nil {
		authError(w, err)
		return
	}
	scoped, err := h.stateForTenant(r.Context(), session.Token)
	if err != nil {
		authError(w, err)
		return
	}
	if scoped.User == nil && !scoped.NeedsWorkspace {
		_ = h.service.Revoke(r.Context(), session.Token)
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.secure(r), MaxAge: int(auth.SessionTTL.Seconds()), Expires: session.ExpiresAt})
	h.writeState(w, status, scoped)
}

func (h *authHTTP) logout(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decodeJSON(w, r, &input) {
		return
	}
	// Never tell the browser logout succeeded before the durable revocation has completed.
	if err := h.service.Revoke(r.Context(), token(r)); err != nil {
		authError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.secure(r), MaxAge: -1, Expires: time.Unix(1, 0)})
	h.writeState(w, 200, auth.State{Initialized: true})
}

func (h *authHTTP) registerInvitation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	session, err := h.service.RegisterInvitation(r.Context(), h.tenantID, input.Token, input.Username, input.Password)
	if err != nil {
		authError(w, err)
		return
	}
	state, err := h.stateForTenant(r.Context(), session.Token)
	if err != nil {
		authError(w, err)
		return
	}
	if state.User == nil {
		_ = h.service.Revoke(r.Context(), session.Token)
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.secure(r), MaxAge: int(auth.SessionTTL.Seconds()), Expires: session.ExpiresAt})
	h.writeState(w, 201, state)
}

type sessionUserKey struct{}

func sessionUser(r *http.Request) auth.User {
	user, _ := r.Context().Value(sessionUserKey{}).(auth.User)
	return user
}

func (h *authHTTP) requireAdmin(next http.Handler) http.Handler {
	return h.requireUser(requireAdminRole(next))
}

func requireAdminRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sessionUser(r).Role != auth.RoleAdmin {
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requirePlatformAdmin(tenantID int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tenantID != 1 || sessionUser(r).ID != 1 {
				writeJSON(w, 403, map[string]string{"error": "forbidden"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *authHTTP) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.available(w) {
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !h.sameOrigin(w, r) {
			return
		}
		state, err := h.stateForTenant(r.Context(), token(r))
		if err != nil {
			authError(w, err)
			return
		}
		if state.User == nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		ctx := context.WithValue(r.Context(), sessionUserKey{}, *state.User)
		ctx = audit.WithActor(ctx, audit.Actor{TenantID: h.tenantID, ID: state.User.ID, Username: state.User.Username, Role: string(state.User.Role), Source: "user"})
		h.auditRequest(next, w, r.WithContext(ctx))
	})
}

func authError(w http.ResponseWriter, err error) {
	status, code := 503, "unavailable"
	switch {
	case errors.Is(err, auth.ErrInitialized):
		status, code = 409, "already_initialized"
	case errors.Is(err, auth.ErrCredentials):
		status, code = 401, "invalid_credentials"
	case errors.Is(err, auth.ErrCurrentPassword):
		status, code = 400, "current_password_invalid"
	case errors.Is(err, auth.ErrPasswordChanged):
		status, code = 409, "password_changed"
	case errors.Is(err, auth.ErrInput):
		status, code = 400, "invalid_input"
	case errors.Is(err, auth.ErrBusy):
		status, code = 429, "auth_busy"
		w.Header().Set("Retry-After", "1")
	case errors.Is(err, auth.ErrUsernameTaken):
		status, code = 409, "username_taken"
	case errors.Is(err, auth.ErrMemberNotFound):
		status, code = 404, "member_not_found"
	case errors.Is(err, auth.ErrInvitationInvalid):
		status, code = 410, "invitation_invalid"
	}
	writeJSON(w, status, map[string]string{"error": code})
}
