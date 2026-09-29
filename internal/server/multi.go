package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/tenants"
)

const workspaceHeader = "X-SubLane-Workspace"

type globalUserKey struct{}

type multiHTTP struct {
	connection *sql.DB
	identity   *auth.Service
	tenancy    *tenants.Service
	publicURL  string
	forTenant  func(int64) http.Handler
}

// Browser requests select a workspace; gateway requests use the key's stored binding.
func NewMulti(connection *sql.DB, identity *auth.Service, tenancy *tenants.Service, publicURL string, tenantHandler func(int64) http.Handler) http.Handler {
	h := &multiHTTP{connection: connection, identity: identity, tenancy: tenancy, publicURL: publicURL, forTenant: tenantHandler}
	router := chi.NewRouter()
	router.Use(requestLogging)
	routeErrors(router)
	router.Route("/api/workspaces", func(global chi.Router) {
		routeErrors(global)
		global.Use(h.requireGlobalUser)
		global.Get("/", h.list)
		global.With((&authHTTP{publicURL: publicURL}).requireOrigin).Post("/", h.create)
	})
	gateway := http.HandlerFunc(h.gateway)
	router.Handle("/v1", gateway)
	router.Handle("/v1/*", gateway)
	router.Handle("/v1beta", gateway)
	router.Handle("/v1beta/*", gateway)
	router.Handle("/*", http.HandlerFunc(h.selected))
	return router
}

func (h *multiHTTP) requireGlobalUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.identity == nil || h.tenancy == nil {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		state, err := h.identity.State(r.Context(), token(r))
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		if state.User == nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), globalUserKey{}, state.User.ID)))
	})
}

func (h *multiHTTP) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(globalUserKey{}).(int64)
	values, err := h.tenancy.List(r.Context(), userID)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	writeJSON(w, 200, map[string]any{"tenants": values})
}

func (h *multiHTTP) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	userID, _ := r.Context().Value(globalUserKey{}).(int64)
	created, err := h.tenancy.Create(r.Context(), userID, input.Name)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrInput):
			writeJSON(w, 400, map[string]string{"error": "invalid_input"})
		case errors.Is(err, tenants.ErrForbidden):
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
		case errors.Is(err, tenants.ErrWorkspaceLimit):
			writeJSON(w, 409, map[string]string{"error": "workspace_limit_reached"})
		default:
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
		}
		return
	}
	writeJSON(w, 201, created)
}

func (h *multiHTTP) gateway(w http.ResponseWriter, r *http.Request) {
	tenantID := int64(1)
	// Match the protocol-specific credential parser used by the tenant handler.
	// Otherwise native clients with header/query keys would be sent to workspace one.
	kind := gateway.Responses
	if r.URL.Path == "/v1/messages" || strings.HasPrefix(r.URL.Path, "/v1/messages/") {
		kind = gateway.Messages
	} else if r.URL.Path == "/v1beta" || strings.HasPrefix(r.URL.Path, "/v1beta/") {
		kind = gateway.Gemini
	}
	if secret, ok := gatewaySecret(r, kind); ok && len(secret) == 46 && strings.HasPrefix(secret, "sl_") {
		digest := sha256.Sum256([]byte(secret))
		err := h.connection.QueryRowContext(r.Context(), `SELECT g.tenant_id FROM api_keys k
			JOIN account_groups g ON g.id=k.group_id WHERE k.token_hash=?`, digest[:]).Scan(&tenantID)
		if err != nil && err != sql.ErrNoRows {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		if err == sql.ErrNoRows {
			tenantID = 1
		}
	}
	h.serveTenant(w, r, tenantID)
}

func (h *multiHTTP) selected(w http.ResponseWriter, r *http.Request) {
	tenantID := int64(1)
	if raw := r.Header.Get(workspaceHeader); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			writeJSON(w, 400, map[string]string{"error": "invalid_workspace"})
			return
		}
		tenantID = parsed
	}
	h.serveTenant(w, r, tenantID)
}

func (h *multiHTTP) serveTenant(w http.ResponseWriter, r *http.Request, tenantID int64) {
	// Workspace one is created by first-run setup, after the SPA and health routes must already work.
	if tenantID != 1 {
		var exists bool
		if err := h.connection.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM tenants WHERE id=?)", tenantID).Scan(&exists); err != nil {
			writeJSON(w, 503, map[string]string{"error": "unavailable"})
			return
		}
		if !exists {
			writeJSON(w, 404, map[string]string{"error": "workspace_not_found"})
			return
		}
	}
	handler := h.forTenant(tenantID)
	if handler == nil {
		writeJSON(w, 503, map[string]string{"error": "unavailable"})
		return
	}
	handler.ServeHTTP(w, r)
}
