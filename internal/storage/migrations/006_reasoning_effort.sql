ALTER TABLE request_records ADD COLUMN reasoning_effort TEXT NOT NULL DEFAULT ''
CHECK(reasoning_effort IN ('','none','minimal','low','medium','high','xhigh','max','ultra'));
