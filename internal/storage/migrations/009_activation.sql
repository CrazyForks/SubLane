ALTER TABLE memberships ADD COLUMN first_request_at INTEGER NOT NULL DEFAULT 0 CHECK(first_request_at >= 0);

UPDATE memberships SET first_request_at=COALESCE((
 SELECT MIN(s.day) FROM usage_daily s JOIN account_groups g ON g.id=s.group_id
 WHERE g.tenant_id=memberships.tenant_id AND s.user_id=memberships.user_id AND s.completed>0
),0);
