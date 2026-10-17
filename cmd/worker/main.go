package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MohsenDowlati/jobeja/internal/config"
	"github.com/MohsenDowlati/jobeja/internal/scheduler"
	"github.com/MohsenDowlati/jobeja/internal/storage/rabbitmq"
	"github.com/MohsenDowlati/jobeja/internal/telegram"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	if cfg.TelegramBotToken == "" {
		log.Fatal("BOT_TOKEN (or TELEGRAM_BOT_TOKEN) is required")
	}

	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()
	dbCtx, cancelDB := context.WithTimeout(ctx, 5*time.Second)
	err = dbPool.Ping(dbCtx)
	cancelDB()
	if err != nil {
		log.Fatalf("Database startup check failed: %v", err)
	}

	rabbitClient, err := rabbitmq.NewClient(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("RabbitMQ init error: %v\n", err)
	}
	defer rabbitClient.Close()

	tgClient := telegram.NewBotClient(cfg.TelegramBotToken)

	sched := &scheduler.Scheduler{DB: dbPool}
	publisher := scheduler.NewOutboxPublisher(dbPool, rabbitClient)
	worker := scheduler.NewWorker(dbPool, rabbitClient, tgClient)

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, err := sched.MaterializeDue(ctx, 50)
				if err != nil {
					log.Printf("[Scheduler] Error materializing due tasks: %v", err)
				} else if count > 0 {
					log.Printf("[Scheduler] Materialized %d due tasks", count)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, err := publisher.ProcessOutbox(ctx, 50)
				if err != nil {
					log.Printf("[Publisher] Error dispatching outbox: %v", err)
				} else if count > 0 {
					log.Printf("[Publisher] Published %d tasks to RabbitMQ", count)
				}
			}
		}
	}()

	log.Println("Worker is running...")
	if err := worker.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("Worker stopped with error: %v\n", err)
	}
}
