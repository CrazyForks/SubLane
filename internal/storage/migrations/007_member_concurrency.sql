-- Alter only the limit column so membership references, rate windows and indexes survive.
ALTER TABLE memberships RENAME COLUMN max_concurrency TO previous_max_concurrency;
ALTER TABLE memberships ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 10
CHECK(max_concurrency BETWEEN 0 AND 10);

-- Preserve saved policies, including an explicit unlimited value of zero.
UPDATE memberships SET max_concurrency = previous_max_concurrency;
ALTER TABLE memberships DROP COLUMN previous_max_concurrency;
