-- name: UpsertUser :one
INSERT INTO users (telegram_id, username, timezone)
VALUES ($1, $2, $3)
    ON CONFLICT (telegram_id)
DO UPDATE SET
    username = EXCLUDED.username,
           timezone = COALESCE(NULLIF(EXCLUDED.timezone, ''), users.timezone)
           RETURNING telegram_id, username, timezone, created_at;

-- name: GetUser :one
SELECT telegram_id, username, timezone, created_at
FROM users
WHERE telegram_id = $1;