package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/MohsenDowlati/jobeja/internal/storage/rabbitmq"
	"github.com/MohsenDowlati/jobeja/internal/telegram"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Worker struct {
	db       *pgxpool.Pool
	rabbit   *rabbitmq.Client
	telegram *telegram.BotClient
}

func NewWorker(db *pgxpool.Pool, rabbit *rabbitmq.Client, tg *telegram.BotClient) *Worker {
	return &Worker{
		db:       db,
		rabbit:   rabbit,
		telegram: tg,
	}
}

func (w *Worker) Start(ctx context.Context) error {
	msgs, err := w.rabbit.StartConsumer(ctx)
	if err != nil {
		return fmt.Errorf("start consumer: %w", err)
	}

	log.Println("[Worker] Started consuming tasks from RabbitMQ...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			w.handleDelivery(ctx, d)
		}
	}
}

func (w *Worker) handleDelivery(ctx context.Context, d amqp.Delivery) {
	var payload rabbitmq.DispatchMessage
	if err := json.Unmarshal(d.Body, &payload); err != nil {
		log.Printf("[Worker] Invalid message body: %v", err)
		_ = d.Nack(false, false)
		return
	}

	processCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := w.processTaskRun(processCtx, payload.RunID); err != nil {
		log.Printf("[Worker] Failed to process run_id %s: %v", payload.RunID, err)
		_ = d.Nack(false, true)
		return
	}

	_ = d.Ack(false)
}

func (w *Worker) processTaskRun(ctx context.Context, runID string) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var taskID string
	var telegramID int64
	var messageText string

	err = tx.QueryRow(ctx, `
		SELECT tr.status, tr.task_id::text, t.user_id, t.message
		FROM task_runs tr
		JOIN tasks t ON t.id = tr.task_id
		WHERE tr.id = $1::uuid
		FOR UPDATE
	`, runID).Scan(&currentStatus, &taskID, &telegramID, &messageText)
	if err != nil {
		return fmt.Errorf("fetch task run info: %w", err)
	}

	if currentStatus == "completed" {
		return nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE task_runs
		SET status = 'processing'
		WHERE id = $1::uuid
	`, runID)
	if err != nil {
		return fmt.Errorf("update run to processing: %w", err)
	}

	reminderText := fmt.Sprintf("task reminder :\n\n%s", messageText)
	if err := w.telegram.SendMessage(ctx, telegramID, reminderText); err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE task_runs
		SET status = 'completed'
		WHERE id = $1::uuid
	`, runID)
	if err != nil {
		return fmt.Errorf("update run to completed: %w", err)
	}

	return tx.Commit(ctx)
}
