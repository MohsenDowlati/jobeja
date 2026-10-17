package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/MohsenDowlati/jobeja/internal/storage/rabbitmq"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxPublisher struct {
	db     *pgxpool.Pool
	rabbit *rabbitmq.Client
}

type claimedItem struct {
	OutboxID string
	RunID    string
}

func NewOutboxPublisher(db *pgxpool.Pool, rabbit *rabbitmq.Client) *OutboxPublisher {
	return &OutboxPublisher{
		db:     db,
		rabbit: rabbit,
	}
}

func (p *OutboxPublisher) ProcessOutbox(ctx context.Context, batchSize int) (int, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin outbox claim tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		WITH picked AS (
			SELECT id
			FROM dispatch_outbox
			WHERE published_at IS NULL
			  AND (claim_until IS NULL OR claim_until < NOW())
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE dispatch_outbox AS o
		SET claim_until = NOW() + INTERVAL '1 minute',
		    attempts = attempts + 1
		FROM picked
		WHERE o.id = picked.id
		RETURNING o.id::text, o.run_id::text;
	`, batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim outbox items: %w", err)
	}

	var items []claimedItem
	for rows.Next() {
		var item claimedItem
		if err := rows.Scan(&item.OutboxID, &item.RunID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox claim item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate outbox claim rows: %w", err)
	}
	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit outbox claim tx: %w", err)
	}

	if len(items) == 0 {
		return 0, nil
	}

	publishedCount := 0
	for _, item := range items {
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := p.rabbit.PublishWithConfirm(publishCtx, rabbitmq.DispatchMessage{RunID: item.RunID})
		cancel()

		if err != nil {
			log.Printf("[Outbox] Failed to publish run_id %s: %v", item.RunID, err)
			continue
		}

		_, err = p.db.Exec(ctx, `
			UPDATE dispatch_outbox
			SET published_at = NOW(),
			    claim_until = NULL
			WHERE id = $1::uuid
			  AND published_at IS NULL
		`, item.OutboxID)
		if err != nil {
			log.Printf("[Outbox] Warning: published run_id %s but failed to update status: %v", item.RunID, err)
			continue
		}
		publishedCount++
	}

	return publishedCount, nil
}
