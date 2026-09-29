CREATE TABLE account_model_runtime (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model TEXT NOT NULL CHECK(length(model) BETWEEN 1 AND 128),
    cooldown_until INTEGER NOT NULL,
    failures INTEGER NOT NULL CHECK(failures BETWEEN 1 AND 16),
    lifecycle INTEGER NOT NULL,
    runtime_revision INTEGER NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY(account_id,model)
);
