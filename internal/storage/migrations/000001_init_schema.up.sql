CREATE TABLE users (
                       telegram_id BIGINT PRIMARY KEY,
                       username VARCHAR(255),
                       timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
                       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE task_type AS ENUM ('one_off', 'recurring');

CREATE TYPE task_status AS ENUM ('pending', 'processing', 'completed', 'failed', 'cancelled');

CREATE TABLE tasks (
                       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       user_id BIGINT NOT NULL REFERENCES users(telegram_id) ON DELETE CASCADE,

                       message TEXT NOT NULL,

                       scheduled_at TIMESTAMPTZ NOT NULL,

                       cron_expr VARCHAR(64),

                       type task_type NOT NULL DEFAULT 'one_off',
                       status task_status NOT NULL DEFAULT 'pending',

                       idempotency_key VARCHAR(128) UNIQUE,

                       retry_count INT NOT NULL DEFAULT 0,
                       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                       updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_scheduler
    ON tasks (scheduled_at, status)
    WHERE status = 'pending';
