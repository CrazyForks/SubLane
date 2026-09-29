-- name: GetAlertConfig :one
SELECT alert_config FROM tenants WHERE id=sqlc.arg(tenant_id) AND status='active';

-- name: SaveAlertConfig :execrows
UPDATE tenants SET alert_config=sqlc.arg(config) WHERE id=sqlc.arg(tenant_id) AND status='active';

-- name: ListAlertConfigs :many
SELECT id,alert_config FROM tenants WHERE alert_config<>'{}' ORDER BY id;

-- name: CountWebhookSecrets :one
SELECT COUNT(*) FROM tenants WHERE json_extract(alert_config,'$.endpoint') IS NOT NULL;

-- name: ClearAlertStates :exec
DELETE FROM alert_states WHERE tenant_id=sqlc.arg(tenant_id);

-- name: ListAlertStates :many
SELECT * FROM alert_states WHERE tenant_id=sqlc.arg(tenant_id) ORDER BY retry_at,changed_at,kind,subject;

-- name: SaveAlertState :exec
INSERT INTO alert_states(tenant_id,kind,subject,active,delivered_active,event_id,changed_at,attempts,retry_at,delivered_at,delivery_failed)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(tenant_id,kind,subject) DO UPDATE SET active=excluded.active,delivered_active=excluded.delivered_active,
event_id=excluded.event_id,changed_at=excluded.changed_at,attempts=excluded.attempts,retry_at=excluded.retry_at,
delivered_at=excluded.delivered_at,delivery_failed=excluded.delivery_failed;

-- name: FinishAlertDelivery :exec
UPDATE alert_states SET delivered_active=active,delivered_at=sqlc.arg(now),attempts=0,retry_at=0,delivery_failed=0
WHERE tenant_id=sqlc.arg(tenant_id) AND event_id=sqlc.arg(event_id);

-- name: FailAlertDelivery :exec
UPDATE alert_states SET delivery_failed=1 WHERE tenant_id=sqlc.arg(tenant_id) AND event_id=sqlc.arg(event_id);

-- name: ListAlertSignals :many
SELECT 'account_reauthorization' AS kind,a.id AS subject FROM accounts a
WHERE a.tenant_id=sqlc.arg(tenant_id) AND a.enabled=1 AND a.provider='codex' AND a.status='reauth_required'
UNION ALL
SELECT 'pool_unavailable',CAST(g.id AS TEXT) FROM account_groups g
WHERE g.tenant_id=sqlc.arg(tenant_id) AND g.enabled=1 AND NOT EXISTS(
 SELECT 1 FROM group_accounts ga JOIN accounts a ON a.id=ga.account_id
 LEFT JOIN account_runtime r ON r.account_id=a.id
 WHERE ga.group_id=g.id AND a.enabled=1 AND a.provider='codex' AND a.status='ready'
 AND COALESCE(r.cooldown_until,0)=0
)
UNION ALL
SELECT 'request_failures','workspace' FROM request_records r JOIN account_groups g ON g.id=r.group_id
WHERE g.tenant_id=sqlc.arg(tenant_id)
AND r.started_at+r.duration_ms/1000>=sqlc.arg(since)
AND r.started_at+r.duration_ms/1000<=sqlc.arg(now)
AND r.outcome IN ('success','error','incomplete')
GROUP BY g.tenant_id
HAVING COUNT(*)>=5 AND SUM(CASE WHEN r.outcome IN ('error','incomplete') THEN 1 ELSE 0 END)*2>=COUNT(*);
