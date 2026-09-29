ALTER TABLE tenants ADD COLUMN alert_config TEXT NOT NULL DEFAULT '{}' CHECK(length(alert_config)<=8192 AND json_valid(alert_config));

CREATE TABLE alert_states (
 tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('account_reauthorization','pool_unavailable','request_failures')),
 subject TEXT NOT NULL CHECK(length(subject)<=64),
 active INTEGER NOT NULL CHECK(active IN (0,1)),
 delivered_active INTEGER NOT NULL CHECK(delivered_active IN (0,1)),
 event_id TEXT NOT NULL CHECK(length(event_id)=32),
 changed_at INTEGER NOT NULL,
 attempts INTEGER NOT NULL DEFAULT 0,
 retry_at INTEGER NOT NULL DEFAULT 0,
 delivered_at INTEGER NOT NULL DEFAULT 0,
 delivery_failed INTEGER NOT NULL DEFAULT 0 CHECK(delivery_failed IN (0,1)),
 PRIMARY KEY(tenant_id,kind,subject)
);
