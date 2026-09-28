-- Keep saved account limits and references while widening the allowed range.
ALTER TABLE accounts RENAME COLUMN max_concurrency TO previous_max_concurrency;
ALTER TABLE accounts ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 30
CHECK(max_concurrency BETWEEN 1 AND 30);
UPDATE accounts SET max_concurrency = previous_max_concurrency;
ALTER TABLE accounts DROP COLUMN previous_max_concurrency;
