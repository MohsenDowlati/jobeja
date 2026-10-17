package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/MohsenDowlati/jobeja/internal/ai"
	"github.com/MohsenDowlati/jobeja/internal/config"
	"github.com/MohsenDowlati/jobeja/internal/telegram"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	if cfg.TelegramBotToken == "" || cfg.OpenAIAPIKey == "" {
		log.Fatal("BOT_TOKEN (or TELEGRAM_BOT_TOKEN) and OPENAI_API_KEY are required")
	}

	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	defer dbPool.Close()
	dbCtx, cancelDB := context.WithTimeout(ctx, 5*time.Second)
	err = dbPool.Ping(dbCtx)
	cancelDB()
	if err != nil {
		log.Fatalf("Database startup check failed: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer rdb.Close()

	bot := telegram.NewBotClient(cfg.TelegramBotToken)
	aiClient := ai.NewClient(cfg.OpenAIAPIKey)

	log.Println("Telegram Bot is running (Long Polling)...")

	offset := 0
	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down bot...")
			return
		default:
			updates, err := bot.GetUpdates(ctx, offset, 20)
			if err != nil {
				log.Printf("getUpdates error: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			for _, update := range updates {
				offset = update.UpdateID + 1
				if update.Message == nil || update.Message.From == nil {
					continue
				}

				go handleIncomingMessage(ctx, dbPool, rdb, bot, aiClient, update.Message)
			}
		}
	}
}

func handleIncomingMessage(
	ctx context.Context,
	db *pgxpool.Pool,
	rdb *redis.Client,
	bot *telegram.BotClient,
	aiClient *ai.Client,
	msg *telegram.Message,
) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	rateLimitKey := fmt.Sprintf("ratelimit:user:%d", userID)
	count, err := rdb.Incr(ctx, rateLimitKey).Result()
	if err == nil {
		if count == 1 {
			rdb.Expire(ctx, rateLimitKey, time.Minute)
		}
		if count > 5 {
			_ = bot.SendMessage(ctx, chatID, "Too many request, please wait ...")
			return
		}
	}

	userTZ := "Asia/Tehran"
	_, err = db.Exec(ctx, `
		INSERT INTO users (telegram_id, username, timezone)
		VALUES ($1, $2, $3)
		ON CONFLICT (telegram_id) DO UPDATE
		SET username = EXCLUDED.username
	`, userID, msg.From.Username, userTZ)
	if err != nil {
		log.Printf("Failed to upsert user: %v", err)
		_ = bot.SendMessage(ctx, chatID, "I couldn't save your reminder. Please try again.")
		return
	}

	var promptText string

	if msg.Voice != nil {
		_ = bot.SendMessage(ctx, chatID, "🎙️ Processing the voice ...")

		tempVoicePath := filepath.Join(os.TempDir(), fmt.Sprintf("voice_%d_%s.oga", userID, msg.Voice.FileID))
		if err := bot.DownloadVoiceFile(ctx, msg.Voice.FileID, tempVoicePath); err != nil {
			log.Printf("Voice download failed: %v", err)
			_ = bot.SendMessage(ctx, chatID, "Error accord during processing the voice")
			return
		}
		defer os.Remove(tempVoicePath)

		transcription, err := aiClient.TranscribeAudio(ctx, tempVoicePath)
		if err != nil {
			log.Printf("Whisper failed: %v", err)
			_ = bot.SendMessage(ctx, chatID, "Error accord during converting the voice to the text")
			return
		}
		promptText = transcription
	} else if msg.Text != "" {
		if msg.Text == "/start" {
			_ = bot.SendMessage(ctx, chatID,
				"Hey shawty! I'm your task-scheduling bot. Drop me a text or voice note, and I'll set up a reminder for you. \n Example: 'Remind me about the work meeting tomorrow at 10 AM.'")
			return
		}
		promptText = msg.Text
	} else {
		return
	}

	parsed, err := aiClient.ParseTaskPrompt(ctx, promptText, userTZ, time.Now().UTC())
	if err != nil {
		log.Printf("OpenAI parse error: %v", err)
		_ = bot.SendMessage(ctx, chatID, "I couldn't analyze your prompt, try again.")
		return
	}

	if !parsed.IsTask {
		if parsed.ErrorReason != "" {
			_ = bot.SendMessage(ctx, chatID, fmt.Sprintf("task didn't find: %s", parsed.ErrorReason))
		} else {
			_ = bot.SendMessage(ctx, chatID, "My hands are tied to just tasks and reminders. Hit me with a specific time, please!")
		}
		return
	}

	scheduledTime, err := time.Parse(time.RFC3339, parsed.ScheduledAt)
	if err != nil {
		log.Printf("Invalid time format from AI: %s", parsed.ScheduledAt)
		_ = bot.SendMessage(ctx, chatID, "Choose a valid time")
		return
	}

	taskType := "one_off"
	var cronExpr *string
	if parsed.IsRecurring && parsed.CronExpr != "" {
		taskType = "recurring"
		cronExpr = &parsed.CronExpr
	}

	idempotencyKey := fmt.Sprintf("tg:%d:%d", chatID, msg.MessageID)

	var taskID string
	err = db.QueryRow(ctx, `
		INSERT INTO tasks (
			user_id,
			type,
			scheduled_at,
			cron_expr,
			timezone,
			message,
			idempotency_key
		) VALUES ($1, $2::task_type, $3, $4, $5, $6, $7)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id::text
	`, userID, taskType, scheduledTime, cronExpr, userTZ, parsed.Message, idempotencyKey).Scan(&taskID)

	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("Insert task failed: %v", err)
		_ = bot.SendMessage(ctx, chatID, "I couldn't save your reminder. Please try again.")
		return
	}

	loc, _ := time.LoadLocation(userTZ)
	userFormattedTime := scheduledTime.In(loc).Format("2006-01-02 time 15:04")

	reply := fmt.Sprintf("✅ Reminder locked in!\n\n📌 Title: %s\n⏰ Time: %s (%s)",
		parsed.Message, userFormattedTime, userTZ)
	if parsed.IsRecurring {
		reply += fmt.Sprintf("\n🔄 Recurrenge: %s", parsed.CronExpr)
	}

	_ = bot.SendMessage(ctx, chatID, reply)
}
