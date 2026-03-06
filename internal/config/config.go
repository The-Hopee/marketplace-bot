// internal/config/config.go
package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken     string
	DatabaseURL       string
	RedisURL          string
	CacheTTL          time.Duration
	TBankTerminalKey  string
	TBankSecretKey    string
	TBankBaseURL      string
	SubscriptionPrice int64
	SubscriptionDays  int
	WebhookURL        string
	ServerPort        string
	AdminTelegramID   int64
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	price, _ := strconv.ParseInt(getEnv("SUBSCRIPTION_PRICE", "2500"), 10, 64)
	days, _ := strconv.Atoi(getEnv("SUBSCRIPTION_DAYS", "30"))
	cacheTTL, _ := strconv.Atoi(getEnv("CACHE_TTL_MINUTES", "30"))
	ID, _ := strconv.ParseInt(os.Getenv("ADMIN_TELEGRAM_ID"), 10, 64)

	return &Config{
		TelegramToken:     os.Getenv("TELEGRAM_TOKEN"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		CacheTTL:          time.Duration(cacheTTL) * time.Minute,
		TBankTerminalKey:  os.Getenv("TBANK_TERMINAL_KEY"),
		TBankSecretKey:    os.Getenv("TBANK_SECRET_KEY"),
		TBankBaseURL:      getEnv("TBANK_BASE_URL", "https://securepay.tinkoff.ru/v2"),
		SubscriptionPrice: price,
		SubscriptionDays:  days,
		WebhookURL:        os.Getenv("WEBHOOK_URL"),
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		AdminTelegramID:   ID,
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
