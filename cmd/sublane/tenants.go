package main

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/oauth"
	"github.com/murongg/SubLane/internal/pricing"
	"github.com/murongg/SubLane/internal/server"
	"github.com/murongg/SubLane/internal/tenants"
	"github.com/murongg/SubLane/internal/timezone"
	"github.com/murongg/SubLane/internal/upstream"
	"github.com/murongg/SubLane/internal/vault"
	"github.com/murongg/SubLane/internal/versions"
)

type tenantRuntime struct {
	handler http.Handler
	gateway *gateway.Service
}

type tenantRegistry struct {
	mu        sync.Mutex
	runtimes  map[int64]tenantRuntime
	ctx       context.Context
	db        *sql.DB
	vault     *vault.Vault
	auth      *auth.Service
	tenants   *tenants.Service
	provider  *upstream.Client
	pricing   *pricing.Service
	versions  *versions.Service
	timeZone  *timezone.Service
	assets    fs.FS
	dataDir   string
	publicURL string
	version   string
	started   time.Time
}

func (r *tenantRegistry) Handler(id int64) http.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	if runtime, ok := r.runtimes[id]; ok {
		return runtime.handler
	}
	accountService := accounts.NewForTenant(r.db, r.vault, id)
	accountService.RestrictToCodex()
	forwarding := gateway.NewForTenant(r.ctx, r.db, accountService, r.provider, id, r.pricing)
	forwarding.SetTimeZone(r.timeZone)
	dataDir := ""
	var codexVersions *versions.Service
	if id == 1 {
		// The instance archive and version policy remain platform operations.
		dataDir, codexVersions = r.dataDir, r.versions
	}
	handler := server.New(server.Options{
		DataDir: dataDir, Assets: r.assets, Version: r.version, StartedAt: r.started,
		Ping: r.db.PingContext, Audit: audit.NewForTenant(r.db, id), Auth: r.auth,
		Keys: apikey.NewForTenant(r.db, r.vault, id), Accounts: accountService,
		OAuth: oauth.New(accountService, r.provider), Gateway: forwarding,
		Groups: groups.NewForTenant(r.db, id), Tenants: r.tenants, TenantID: id,
		PublicURL: r.publicURL, CodexVersions: codexVersions, TimeZone: r.timeZone,
	})
	if r.runtimes == nil {
		r.runtimes = make(map[int64]tenantRuntime)
	}
	r.runtimes[id] = tenantRuntime{handler: handler, gateway: forwarding}
	return handler
}

func (r *tenantRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, runtime := range r.runtimes {
		runtime.gateway.Close()
	}
}
