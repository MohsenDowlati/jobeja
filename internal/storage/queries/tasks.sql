-- name: CreateTask :one
INSERT INTO tasks (
    user_id,
    type,
    scheduled_at,
    cron_expr,
    timezone,
    message,
    idempotency_key
) VALUES (
             $1, $2, $3, $4, $5, $6, $7
         )
    ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;
-- name: FetchAndLockReadyTasks :many
UPDATE tasks
SET status = 'processing', updated_at = NOW()
WHERE id IN (
    SELECT id
    FROM tasks
    WHERE status = 'pending'
      AND scheduled_at <= NOW()
    ORDER BY scheduled_at ASC
    LIMIT $1
    FOR UPDATE SKIP LOCKED
            )
            RETURNING id, user_id, message, scheduled_at, cron_expr, type, status, idempotency_key, retry_count;

-- name: UpdateTaskStatus :exec
UPDATE tasks
SET status = $2,
    retry_count = CASE WHEN $3::boolean THEN retry_count + 1 ELSE retry_count END,
    updated_at = NOW()
WHERE id = $1;

-- name: RescheduleTask :exec
UPDATE tasks
SET scheduled_at = $2,
    status = 'pending',
    updated_at = NOW()
WHERE id = $1;


