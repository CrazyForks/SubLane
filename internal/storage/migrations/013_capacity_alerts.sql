CREATE TABLE alert_states_next (
 tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK(kind IN ('account_reauthorization','pool_unavailable','request_failures','model_unavailable','quota_low')),
 subject TEXT NOT NULL CHECK(length(subject)<=160),
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
INSERT INTO alert_states_next SELECT * FROM alert_states;
DROP TABLE alert_states;
ALTER TABLE alert_states_next RENAME TO alert_states;
