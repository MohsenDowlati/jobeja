ALTER TABLE tasks
    ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT 'UTC';

UPDATE tasks AS t
SET timezone = u.timezone
    FROM users AS u
WHERE t.user_id = u.telegram_id;

CREATE TABLE task_runs (
                           id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                           task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
                           scheduled_at TIMESTAMPTZ NOT NULL,
                           status task_status NOT NULL DEFAULT 'pending',
                           created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

                           UNIQUE (task_id, scheduled_at)
);

CREATE TABLE dispatch_outbox (
                                 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                 run_id UUID NOT NULL UNIQUE REFERENCES task_runs(id) ON DELETE CASCADE,
                                 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                 claim_until TIMESTAMPTZ,
                                 published_at TIMESTAMPTZ,
                                 attempts INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_dispatch_outbox_ready
    ON dispatch_outbox (created_at)
    WHERE published_at IS NULL;
