# Jobeja

<div align="center">

  <img src="https://img.shields.io/badge/Go-1.27%2B-00ADD8?style=for-the-badge&logo=go" alt="Go 1.27+" />
  <img src="https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql" alt="PostgreSQL 16" />
  <img src="https://img.shields.io/badge/Redis-7-FF4438?style=for-the-badge&logo=redis" alt="Redis 7" />
  <img src="https://img.shields.io/badge/RabbitMQ-3-FF6600?style=for-the-badge&logo=rabbitmq" alt="RabbitMQ 3" />
  <img src="https://img.shields.io/badge/Telegram-Bot-26A5E4?style=for-the-badge&logo=telegram" alt="Telegram Bot" />
  <img src="https://img.shields.io/badge/OpenAI-Whisper%20%2B%20Structured%20Outputs-412991?style=for-the-badge&logo=openai" alt="OpenAI" />

  <p>
    <strong>Production-grade AI task scheduler and Telegram reminder bot</strong>
  </p>

</div>

Jobeja is a fault-tolerant task scheduling system that turns natural-language messages and voice notes into reliable reminders. It uses PostgreSQL for durable task state, Redis for distributed coordination, RabbitMQ for reliable dispatch, and OpenAI for parsing prompts and transcribing voice messages.

## Why Jobeja?

Most reminder bots are brittle: they rely on single-process scheduling, lose tasks under failure, or struggle with time parsing. Jobeja is designed for production-like systems where correctness and resilience matter.

It gives you:

- AI-powered task extraction from text and voice
- Reliable scheduling with PostgreSQL-backed task state
- Distributed lock-safe due-task selection via `SKIP LOCKED`
- Outbox-driven event publishing for fault tolerance
- Redis-backed rate limiting and operational safety
- Recurring tasks with cron expressions
- Telegram-first user experience for reminders and notifications

## Architecture

```text
User message / voice note
        │
        ▼
Telegram Bot (cmd/bot)
        │
        ├─ Parse text with OpenAI
        ├─ Transcribe voice with Whisper
        └─ Save task in PostgreSQL
        │
        ▼
PostgreSQL tasks table
        │
        ├─ scheduled_at + status + cron_expr
        ├─ task_runs
        └─ dispatch_outbox
        │
        ▼
Scheduler materializer
        │  (FOR UPDATE SKIP LOCKED)
        ▼
Outbox publisher
        │
        ▼
RabbitMQ
        │
        ▼
Worker consumer
        │
        ▼
Telegram delivery
```

## Core Features

- Natural-language schedule parsing with OpenAI structured output
- Voice reminder support using OpenAI Whisper
- One-off and recurring reminders
- Timezone-aware scheduling
- PostgreSQL `task_runs` and `dispatch_outbox` pattern for resilient delivery
- RabbitMQ publisher confirms for reliable message delivery
- Redis rate limiting per user
- Long-polling Telegram bot
- Dockerized local infrastructure for Postgres, Redis, and RabbitMQ

## Tech Stack

- Go 1.27+
- PostgreSQL 16
- Redis 7
- RabbitMQ 3
- Telegram Bot API
- OpenAI API (chat + Whisper)
- Docker / Docker Compose

## Repository Layout

```text
.
├── cmd/
│   ├── bot/
│   │   └── main.go          # Telegram bot entrypoint
│   └── worker/
│       └── main.go          # Scheduler + worker entrypoint
├── internal/
│   ├── ai/
│   │   └── client.go        # OpenAI parsing + transcription
│   ├── config/
│   │   └── config.go        # Env loading and config assembly
│   ├── domain/
│   │   ├── task.go
│   │   └── user.go
│   ├── scheduler/
│   │   ├── consumer.go      # RabbitMQ consumer / task runtime
│   │   ├── publisher.go     # Outbox publisher
│   │   └── scheduler.go     # Due-task materialization
│   ├── storage/
│   │   ├── migrations/
│   │   ├── postgres/
│   │   ├── queries/
│   │   ├── rabbitmq/
│   │   └── redis/
│   └── telegram/
│       ├── bot.go           # Telegram send helpers
│       └── telegram.go      # Telegram API integration
├── .env.example
├── docker-compose.yml
├── go.mod
├── go.sum
├── sqlc.yaml
├── Dockerfile
├── LICENSE
├── README.md
└── .gitignore
```

## Getting Started

### 1) Clone the repo

```bash
git clone https://github.com/MohsenDowlati/jobeja.git
cd jobeja
```

### 2) Configure environment

Copy the example env file and fill it in:

```bash
cp .env.example .env
```

Required values include:

```env
BOT_TOKEN=your_telegram_bot_token
OPENAI_API_KEY=your_openai_key

POSTGRES_USER=jobeja_user
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=jobeja_db
POSTGRES_PORT=5433

REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password

RABBITMQ_USER=jobeja_rabbit
RABBITMQ_PASSWORD=your_rabbitmq_password
RABBITMQ_PORT=5672
```

### 3) Start dependencies

```bash
docker compose up -d postgres redis rabbitmq
```

This starts:

- PostgreSQL on `localhost:5433`
- Redis on `localhost:6379`
- RabbitMQ on `localhost:5672`
- RabbitMQ management UI on `http://localhost:15672`

### 4) Run the bot

```bash
go run ./cmd/bot
```

### 5) Run the worker

In a second terminal:

```bash
go run ./cmd/worker
```

The worker handles:

- materializing due tasks
- creating task runs
- publishing dispatch events to RabbitMQ
- consuming queued work and sending Telegram reminders

## Example Usage

Send a message to the bot like:

```text
Remind me to submit the client report tomorrow at 9:30 AM.
```

Or:

```text
Every Monday at 8:00 AM, remind me to review the product backlog.
```

Or send a voice note:

```text
Please remind me to call the dentist on Friday at 5 PM.
```

The bot will parse the message using OpenAI and schedule the reminder automatically.

## Production Notes

Jobeja is designed to follow production-oriented patterns rather than toybot assumptions:

- durable scheduling with relational state
- outbox pattern for message reliability
- lock-aware scheduling to avoid duplicate execution
- rate limiting to avoid abuse from Telegram users
- explicit config separation for environment-driven deployments

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome. If you have ideas for improving reliability, AI parsing accuracy, or operator tooling, feel free to open an issue or a pull request.

## Status

Jobeja is actively built as a production-minded reminder and automation platform for Telegram-driven workflows.

---

Made with Go, Redis, PostgreSQL, RabbitMQ, and OpenAI.
