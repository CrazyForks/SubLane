DROP INDEX IF EXISTS allocation_entries_member;
CREATE INDEX allocation_entries_member ON allocation_entries(scheme_id,user_id,started_at);
