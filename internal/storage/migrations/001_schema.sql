-- Initial schema for new installations. Existing pre-release databases are not upgraded.
CREATE TABLE "account_affinity" (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL DEFAULT 1 REFERENCES account_groups(id) ON DELETE CASCADE,
    session_hash BLOB NOT NULL CHECK(length(session_hash)=32),
    provider TEXT NOT NULL DEFAULT 'codex',
    account_id TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY(user_id,group_id,session_hash,provider)
);

CREATE TABLE "account_groups" (
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL COLLATE NOCASE,
 enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 restricted_models INTEGER NOT NULL DEFAULT 0 CHECK(restricted_models IN (0,1)),
 tenant_id INTEGER NOT NULL DEFAULT 1 REFERENCES tenants(id),
 UNIQUE(tenant_id,name));

CREATE TABLE account_runtime (
 account_id TEXT PRIMARY KEY NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 cooldown_until INTEGER NOT NULL DEFAULT 0,
 reason TEXT NOT NULL DEFAULT '',
 failures INTEGER NOT NULL DEFAULT 0,
 last_failure_at INTEGER NOT NULL DEFAULT 0,
 revision INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE "account_usage" (
    account_id TEXT PRIMARY KEY NOT NULL REFERENCES "accounts"(id) ON DELETE CASCADE,
    snapshot BLOB NOT NULL CHECK (length(snapshot) <= 131072),
    updated_at INTEGER NOT NULL CHECK (updated_at > 0)
, revision INTEGER NOT NULL DEFAULT 0);

CREATE TABLE "accounts" (
    id TEXT PRIMARY KEY NOT NULL,
    provider TEXT NOT NULL DEFAULT 'codex' CHECK (provider IN ('codex', 'claude', 'antigravity')),
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    plan TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    status TEXT NOT NULL CHECK (status IN ('ready', 'unverified', 'reauth_required')),
    credential BLOB NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL, max_concurrency INTEGER NOT NULL DEFAULT 2 CHECK(max_concurrency BETWEEN 1 AND 8), models_snapshot BLOB
    CHECK(models_snapshot IS NULL OR length(models_snapshot) <= 131072), models_revision INTEGER NOT NULL DEFAULT 0,
    tenant_id INTEGER NOT NULL DEFAULT 1 REFERENCES tenants(id),
    UNIQUE(tenant_id, provider, account_id)
);

CREATE TABLE allocation_entries (
 request_id TEXT PRIMARY KEY,
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id),
 revision_id INTEGER NOT NULL REFERENCES allocation_revisions(id),
 user_id INTEGER NOT NULL REFERENCES users(id),
 account_id TEXT NOT NULL,
 model TEXT NOT NULL,
 mode TEXT NOT NULL CHECK(mode IN ('tokens','amount')),
 window_start INTEGER NOT NULL,
 reset_at INTEGER NOT NULL,
 started_at INTEGER NOT NULL,
 finished_at INTEGER NOT NULL DEFAULT 0,
 state TEXT NOT NULL CHECK(state IN ('active','pending','settled')),
 input_tokens INTEGER NOT NULL DEFAULT 0,
 output_tokens INTEGER NOT NULL DEFAULT 0,
 cached_tokens INTEGER NOT NULL DEFAULT 0,
 cost INTEGER NOT NULL DEFAULT 0 CHECK(cost>=0),
 manual INTEGER NOT NULL DEFAULT 0 CHECK(manual IN (0,1))
);

CREATE TABLE allocation_keys (
 key_id INTEGER PRIMARY KEY REFERENCES api_keys(id),
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id)
);

CREATE TABLE allocation_revisions (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 scheme_id INTEGER NOT NULL REFERENCES allocation_schemes(id),
 effective_at INTEGER NOT NULL,
 config TEXT NOT NULL CHECK(json_valid(config)),
 UNIQUE(scheme_id,effective_at)
);

CREATE TABLE "allocation_schemes" (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
 group_id INTEGER NOT NULL UNIQUE REFERENCES account_groups(id),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 created_at INTEGER NOT NULL
);

CREATE TABLE "api_keys" (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER NOT NULL DEFAULT 1 REFERENCES account_groups(id),
    name TEXT NOT NULL,
    prefix TEXT NOT NULL,
    token_hash BLOB NOT NULL UNIQUE CHECK(length(token_hash)=32),
    created_at INTEGER NOT NULL,
    last_used_at INTEGER,
    revoked_at INTEGER
, enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)), expires_at INTEGER, encrypted_secret BLOB
    CHECK(encrypted_secret IS NULL OR length(encrypted_secret) BETWEEN 1 AND 512));

CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL DEFAULT 1 REFERENCES tenants(id),
    actor_id INTEGER NOT NULL,
    actor_name TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    source TEXT NOT NULL CHECK(source IN ('user','local')),
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK(outcome IN ('success','failure')),
    http_status INTEGER,
    created_at INTEGER NOT NULL
);

CREATE TABLE group_accounts (
    group_id INTEGER NOT NULL REFERENCES account_groups(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    PRIMARY KEY(group_id,account_id)
);

CREATE TABLE group_members (
    group_id INTEGER NOT NULL REFERENCES account_groups(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY(group_id,user_id)
);

CREATE TABLE group_models (
    group_id INTEGER NOT NULL REFERENCES account_groups(id) ON DELETE CASCADE,
    model TEXT NOT NULL,
    PRIMARY KEY(group_id,model)
);

CREATE TABLE member_rate (
 tenant_id INTEGER NOT NULL,
 user_id INTEGER NOT NULL,
 window_start INTEGER NOT NULL,
 requests INTEGER NOT NULL CHECK(requests BETWEEN 1 AND 6000),
 PRIMARY KEY(tenant_id,user_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES memberships(tenant_id,user_id) ON DELETE CASCADE
);

CREATE TABLE memberships (
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK(role IN ('owner', 'admin', 'member')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    requests_per_minute INTEGER NOT NULL DEFAULT 0 CHECK(requests_per_minute BETWEEN 0 AND 6000),
    max_concurrency INTEGER NOT NULL DEFAULT 0 CHECK(max_concurrency BETWEEN 0 AND 8),
    created_at INTEGER NOT NULL,
    PRIMARY KEY(tenant_id, user_id)
);

CREATE TABLE "request_records" (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 user_id INTEGER NOT NULL,
 key_id INTEGER NOT NULL,
 group_id INTEGER NOT NULL,
 account_id TEXT NOT NULL DEFAULT '',
 provider TEXT NOT NULL DEFAULT '',
 model TEXT NOT NULL DEFAULT '' CHECK(length(model)<=160),
 transport TEXT NOT NULL CHECK(transport IN ('http','websocket')),
 operation TEXT NOT NULL CHECK(operation IN ('responses','chat','compact','messages','gemini')),
 started_at INTEGER NOT NULL,
 duration_ms INTEGER NOT NULL CHECK(duration_ms>=0),
 outcome TEXT NOT NULL CHECK(outcome IN ('success','incomplete','error','canceled','rejected')),
 error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code)<=64),
 upstream_status INTEGER,
 input_tokens INTEGER,
 output_tokens INTEGER,
 cached_tokens INTEGER,
 request_id TEXT NOT NULL DEFAULT '' CHECK(length(request_id)<=64),
 first_token_ms INTEGER CHECK(first_token_ms>=0)
);

CREATE TABLE sessions (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK (length(token_hash) = 32),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tenants (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64),
    owner_user_id INTEGER NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'suspended')),
    created_at INTEGER NOT NULL
);

CREATE TABLE usage_daily (
 day INTEGER NOT NULL,
 user_id INTEGER NOT NULL,
 group_id INTEGER NOT NULL,
 provider TEXT NOT NULL,
 model TEXT NOT NULL CHECK(length(model)<=180),
 requests INTEGER NOT NULL DEFAULT 0 CHECK(requests>=0),
 completed INTEGER NOT NULL DEFAULT 0 CHECK(completed>=0),
 incomplete INTEGER NOT NULL DEFAULT 0 CHECK(incomplete>=0),
 errors INTEGER NOT NULL DEFAULT 0 CHECK(errors>=0),
 canceled INTEGER NOT NULL DEFAULT 0 CHECK(canceled>=0),
 rejected INTEGER NOT NULL DEFAULT 0 CHECK(rejected>=0),
 duration_ms INTEGER NOT NULL DEFAULT 0 CHECK(duration_ms>=0),
 input_tokens INTEGER NOT NULL DEFAULT 0 CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL DEFAULT 0 CHECK(output_tokens>=0),
 cached_tokens INTEGER NOT NULL DEFAULT 0 CHECK(cached_tokens>=0),
 input_reported INTEGER NOT NULL DEFAULT 0 CHECK(input_reported>=0),
 output_reported INTEGER NOT NULL DEFAULT 0 CHECK(output_reported>=0),
 cached_reported INTEGER NOT NULL DEFAULT 0 CHECK(cached_reported>=0),
 PRIMARY KEY(day,user_id,group_id,provider,model)
);

CREATE TABLE usage_hourly (
 tenant_id INTEGER NOT NULL REFERENCES tenants(id),
 hour INTEGER NOT NULL,
 user_id INTEGER NOT NULL,
 requests INTEGER NOT NULL CHECK(requests>=0),
 input_tokens INTEGER NOT NULL CHECK(input_tokens>=0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens>=0),
 input_reported INTEGER NOT NULL CHECK(input_reported BETWEEN 0 AND requests),
 output_reported INTEGER NOT NULL CHECK(output_reported BETWEEN 0 AND requests),
 PRIMARY KEY(tenant_id,hour,user_id)
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member')),
    password_hash TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at INTEGER NOT NULL,
    CHECK ((id = 1 AND role = 'admin' AND enabled = 1) OR (id > 1 AND role = 'member'))
);

CREATE INDEX account_affinity_expiry ON account_affinity(expires_at);

CREATE INDEX account_groups_tenant ON account_groups(tenant_id, id);

CREATE INDEX accounts_enabled ON accounts(enabled, created_at);

CREATE INDEX accounts_tenant ON accounts(tenant_id, id);

CREATE INDEX allocation_entries_member ON allocation_entries(scheme_id,user_id,window_start);

CREATE INDEX allocation_entries_pending ON allocation_entries(account_id,state);

CREATE INDEX api_keys_active_owner ON api_keys(user_id) WHERE revoked_at IS NULL;

CREATE INDEX api_keys_group ON api_keys(group_id);

CREATE INDEX api_keys_owner ON api_keys(user_id,id);

CREATE INDEX audit_events_created ON audit_events(created_at);

CREATE INDEX audit_events_tenant ON audit_events(tenant_id,id);

CREATE INDEX group_accounts_account ON group_accounts(account_id);

CREATE INDEX group_members_user ON group_members(user_id);

CREATE UNIQUE INDEX memberships_one_owner ON memberships(tenant_id) WHERE role = 'owner';

CREATE INDEX memberships_user ON memberships(user_id, tenant_id);

CREATE INDEX request_records_account ON request_records(account_id,id);

CREATE INDEX request_records_request_id ON request_records(request_id) WHERE request_id!='';

CREATE INDEX request_records_time ON request_records(started_at);

CREATE INDEX sessions_expiry ON sessions(expires_at);

CREATE INDEX sessions_user ON sessions(user_id, created_at);

CREATE INDEX usage_daily_user ON usage_daily(user_id,day);

CREATE INDEX usage_hourly_user ON usage_hourly(tenant_id,user_id,hour);

CREATE VIEW effective_group_access AS
SELECT m.user_id,g.id AS group_id
FROM memberships m
JOIN tenants t ON t.id=m.tenant_id
JOIN users u ON u.id=m.user_id
JOIN account_groups g ON g.tenant_id=m.tenant_id
WHERE t.status='active' AND m.enabled=1 AND u.enabled=1 AND g.enabled=1 AND (
 m.role IN ('owner','admin') OR EXISTS(
  SELECT 1 FROM group_members grant_row
  WHERE grant_row.group_id=g.id AND grant_row.user_id=m.user_id
 )
);

CREATE TRIGGER account_groups_tenant_update BEFORE UPDATE OF tenant_id ON account_groups
WHEN NEW.tenant_id != OLD.tenant_id
BEGIN SELECT RAISE(ABORT, 'tenant_immutable'); END;

CREATE TRIGGER accounts_tenant_update BEFORE UPDATE OF tenant_id ON accounts
WHEN NEW.tenant_id != OLD.tenant_id
BEGIN SELECT RAISE(ABORT, 'tenant_immutable'); END;

-- API keys retain their original pool and therefore their original workspace.
CREATE TRIGGER api_keys_group_update BEFORE UPDATE OF group_id ON api_keys
WHEN NEW.group_id != OLD.group_id
BEGIN SELECT RAISE(ABORT, 'key_pool_immutable'); END;

CREATE TRIGGER allocation_exclusive_account BEFORE INSERT ON group_accounts
WHEN EXISTS(SELECT 1 FROM allocation_schemes WHERE group_id=NEW.group_id) OR EXISTS (
 SELECT 1 FROM allocation_schemes s JOIN group_accounts ga ON ga.group_id=s.group_id
 WHERE ga.account_id=NEW.account_id AND ga.group_id<>NEW.group_id
)
BEGIN SELECT RAISE(ABORT,'allocation_pool_conflict'); END;

CREATE TRIGGER allocation_fixed_pool BEFORE DELETE ON group_accounts
WHEN EXISTS(SELECT 1 FROM allocation_schemes WHERE group_id=OLD.group_id)
BEGIN SELECT RAISE(ABORT,'allocation_pool_locked'); END;

CREATE TRIGGER group_accounts_tenant_insert BEFORE INSERT ON group_accounts
WHEN (SELECT tenant_id FROM account_groups WHERE id=NEW.group_id)
     IS NOT (SELECT tenant_id FROM accounts WHERE id=NEW.account_id)
BEGIN SELECT RAISE(ABORT, 'cross_tenant_account'); END;

CREATE TRIGGER group_accounts_tenant_update BEFORE UPDATE ON group_accounts
WHEN (SELECT tenant_id FROM account_groups WHERE id=NEW.group_id)
     IS NOT (SELECT tenant_id FROM accounts WHERE id=NEW.account_id)
BEGIN SELECT RAISE(ABORT, 'cross_tenant_account'); END;

INSERT INTO settings(key,value) VALUES('usage.daily.started_at',CAST(unixepoch() AS TEXT));
INSERT INTO settings(key,value) VALUES('usage.hourly.started_at',CAST(unixepoch() AS TEXT));
