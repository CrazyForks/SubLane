-- name: GetMemberLimits :one
SELECT u.id,m.role,u.enabled,m.requests_per_minute,m.max_concurrency
FROM memberships m JOIN users u ON u.id=m.user_id JOIN tenants t ON t.id=m.tenant_id
WHERE m.tenant_id=sqlc.arg(tenant_id) AND m.user_id=sqlc.arg(user_id)
AND m.enabled=1 AND t.status='active';

-- name: SetMemberLimits :execrows
-- Limits apply to every workspace role, including the owner's own membership.
UPDATE memberships SET requests_per_minute=sqlc.arg(requests_per_minute),max_concurrency=sqlc.arg(max_concurrency)
WHERE tenant_id=sqlc.arg(tenant_id) AND user_id=sqlc.arg(user_id);

-- name: GetMemberWindow :one
SELECT window_start,requests FROM member_rate
WHERE tenant_id=sqlc.arg(tenant_id) AND user_id=sqlc.arg(user_id);

-- name: TakeMemberRate :execrows
INSERT INTO member_rate(tenant_id,user_id,window_start,requests)
SELECT sqlc.arg(tenant_id),sqlc.arg(user_id),sqlc.arg(window_start),1
WHERE NOT EXISTS(SELECT 1 FROM member_rate r WHERE r.tenant_id=sqlc.arg(tenant_id)
AND r.user_id=sqlc.arg(user_id) AND r.window_start=sqlc.arg(window_start) AND r.requests>=sqlc.arg(rate_limit))
ON CONFLICT(tenant_id,user_id) DO UPDATE SET window_start=excluded.window_start,requests=CASE WHEN member_rate.window_start=excluded.window_start THEN member_rate.requests+1 ELSE 1 END;
