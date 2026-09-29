-- name: GetSetupMembership :one
SELECT m.first_request_at FROM memberships m JOIN users u ON u.id=m.user_id
JOIN tenants t ON t.id=m.tenant_id
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.user_id=sqlc.arg(user_id)
AND m.enabled=1 AND u.enabled=1 AND t.status='active';

-- name: GetSetupAccounts :one
SELECT CAST(COUNT(*) AS INTEGER) AS enabled,
CAST(COALESCE(SUM(CASE WHEN status='ready' THEN 1 ELSE 0 END),0) AS INTEGER) AS verified
FROM accounts WHERE tenant_id=sqlc.arg(tenant_id) AND enabled=1 AND provider IN ('codex','claude','antigravity','xai');

-- name: RecordFirstRequest :exec
UPDATE memberships SET first_request_at=sqlc.arg(started_at)
WHERE user_id=sqlc.arg(user_id) AND first_request_at=0
AND tenant_id=(SELECT tenant_id FROM account_groups WHERE id=sqlc.arg(group_id));
