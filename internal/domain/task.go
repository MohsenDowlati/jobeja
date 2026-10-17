package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TaskType string

const (
	TaskTypeOneOff    TaskType = "one_off"
	TaskTypeRecurring TaskType = "recurring"
)

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type Task struct {
	ID             uuid.UUID
	UserID         int64
	Message        string
	ScheduledAt    time.Time
	CronExpr       *string
	Type           TaskType
	Status         TaskStatus
	IdempotencyKey *string
	RetryCount     int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type TaskPayload struct {
	TaskID    uuid.UUID `json:"task_id"`
	UserID    int64     `json:"user_id"`
	Message   string    `json:"message"`
	Type      TaskType  `json:"type"`
	CronExpr  *string   `json:"cron_expr,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

type TaskRepository interface {
	CreateTask(ctx context.Context, task *Task) error
	FetchAndLockReadyTasks(ctx context.Context, limit int32) ([]Task, error)
	UpdateTaskStatus(ctx context.Context, id uuid.UUID, status TaskStatus, incrementRetry bool) error
	RescheduleTask(ctx context.Context, id uuid.UUID, nextRun time.Time) error
}

type TaskQueue interface {
	Publish(ctx context.Context, payload TaskPayload) error
	Consume(ctx context.Context) (<-chan TaskPayload, error)
}
