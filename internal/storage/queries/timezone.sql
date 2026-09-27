-- name: GetTimeZone :one
SELECT value FROM settings WHERE key = 'instance.time_zone';

-- name: SaveTimeZone :exec
INSERT INTO settings(key,value) VALUES('instance.time_zone',sqlc.arg(value))
ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP;
