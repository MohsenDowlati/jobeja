package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config contains the settings shared by the bot and worker.
type Config struct {
	TelegramBotToken string
	OpenAIAPIKey     string
	DatabaseURL      string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	RabbitMQURL      string
}

// Load reads .env from the working directory. Process environment variables
// take precedence; the file is optional for deployed applications.
func Load() (Config, error) {
	values, err := godotenv.Read(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read .env: %w", err)
	}
	get := func(key, fallback string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		if value, ok := values[key]; ok {
			return value
		}
		return fallback
	}

	database := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(get("POSTGRES_USER", "postgres"), get("POSTGRES_PASSWORD", "postgres")),
		Host:   net.JoinHostPort(get("POSTGRES_HOST", "localhost"), get("POSTGRES_PORT", "5432")),
		Path:   "/" + get("POSTGRES_DB", "jobeja"),
	}
	query := url.Values{"sslmode": {get("POSTGRES_SSLMODE", "disable")}}
	database.RawQuery = query.Encode()
	rabbit := url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(get("RABBITMQ_USER", "guest"), get("RABBITMQ_PASSWORD", "guest")),
		Host:   net.JoinHostPort(get("RABBITMQ_HOST", "localhost"), get("RABBITMQ_PORT", "5672")),
		Path:   "/",
	}
	redisDB, err := strconv.Atoi(get("REDIS_DB", "0"))
	if err != nil || redisDB < 0 {
		return Config{}, fmt.Errorf("REDIS_DB must be a non-negative integer")
	}
	return Config{
		TelegramBotToken: get("TELEGRAM_BOT_TOKEN", get("BOT_TOKEN", "")),
		OpenAIAPIKey:     get("OPENAI_API_KEY", ""),
		DatabaseURL:      get("DATABASE_URL", database.String()),
		RedisAddr:        get("REDIS_ADDR", net.JoinHostPort(get("REDIS_HOST", "localhost"), get("REDIS_PORT", "6379"))),
		RedisPassword:    get("REDIS_PASSWORD", ""),
		RedisDB:          redisDB,
		RabbitMQURL:      get("RABBITMQ_URL", rabbit.String()),
	}, nil
}
