CREATE INDEX allocation_entries_retention ON allocation_entries(state, reset_at);
CREATE INDEX usage_hourly_retention ON usage_hourly(hour);
CREATE INDEX tenants_owner ON tenants(owner_user_id);
