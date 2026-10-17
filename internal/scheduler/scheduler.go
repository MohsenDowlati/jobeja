package scheduler

import (
	"context"
	"fmt"
	"github.com/robfig/cron/v3"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Scheduler struct {
	DB *pgxpool.Pool
}

type dueTask struct {
	ID          string
	ScheduledAt time.Time
	Type        string
	CronExpr    *string
	Timezone    string
}

func (s *Scheduler) MaterializeDue(ctx context.Context, limit int) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin scheduler transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT NOW()`).Scan(&now); err != nil {
		return 0, fmt.Errorf("read database time: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, scheduled_at, type::text, cron_expr, timezone
		FROM tasks
		WHERE status = 'pending'
		  AND scheduled_at <= $1
		ORDER BY scheduled_at, id
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, now, limit)
	if err != nil {
		return 0, fmt.Errorf("select due tasks: %w", err)
	}

	var tasks []dueTask
	for rows.Next() {
		var task dueTask
		if err := rows.Scan(
			&task.ID,
			&task.ScheduledAt,
			&task.Type,
			&task.CronExpr,
			&task.Timezone,
		); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan due task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate due tasks: %w", err)
	}
	rows.Close()

	for _, task := range tasks {
		var runID string
		err := tx.QueryRow(ctx, `
			INSERT INTO task_runs (task_id, scheduled_at)
			VALUES ($1::uuid, $2)
			ON CONFLICT (task_id, scheduled_at) DO NOTHING
			RETURNING id::text
		`, task.ID, task.ScheduledAt).Scan(&runID)
		if err != nil {
			return 0, fmt.Errorf("create run for task %s: %w", task.ID, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO dispatch_outbox (run_id)
			VALUES ($1::uuid)
		`, runID); err != nil {
			return 0, fmt.Errorf("create outbox entry for run %s: %w", runID, err)
		}

		switch task.Type {
		case "one_off":
			_, err = tx.Exec(ctx, `
				UPDATE tasks
				SET status = 'processing', updated_at = NOW()
				WHERE id = $1::uuid
			`, task.ID)

		case "recurring":
			if task.CronExpr == nil {
				return 0, fmt.Errorf("recurring task %s has no cron expression", task.ID)
			}

			location, locErr := time.LoadLocation(task.Timezone)
			if locErr != nil {
				return 0, fmt.Errorf("task %s timezone: %w", task.ID, locErr)
			}

			schedule, parseErr := cron.ParseStandard(*task.CronExpr)
			if parseErr != nil {
				return 0, fmt.Errorf("task %s cron: %w", task.ID, parseErr)
			}

			next := schedule.Next(now.In(location)).UTC()

			_, err = tx.Exec(ctx, `
				UPDATE tasks
				SET scheduled_at = $2, updated_at = NOW()
				WHERE id = $1::uuid
			`, task.ID, next)

		default:
			return 0, fmt.Errorf("unknown type %q for task %s", task.Type, task.ID)
		}

		if err != nil {
			return 0, fmt.Errorf("advance task %s: %w", task.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit scheduler transaction: %w", err)
	}
	return len(tasks), nil
}
