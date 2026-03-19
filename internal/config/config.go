// internal/config/config.go
package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken        string
	DatabaseURL          string
	RedisURL             string
	CacheTTL             time.Duration
	TBankTerminalKey     string
	TBankSecretKey       string
	TBankBaseURL         string
	SubscriptionPrice    int64
	ProSubscriptionPrice int64
	SubscriptionDays     int
	WebhookURL           string
	ServerPort           string
	AdminTelegramID      int64

	ScraperAPIKey string
	OpenAIKey     string
	OpenAIBaseURL string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	price, _ := strconv.ParseInt(getEnv("SUBSCRIPTION_PRICE", "7900"), 10, 64)
	proPrice, _ := strconv.ParseInt(getEnv("PRO_SUBSCRIPTION_PRICE", "14900"), 10, 64) // 149 руб
	days, _ := strconv.Atoi(getEnv("SUBSCRIPTION_DAYS", "30"))
	cacheTTL, _ := strconv.Atoi(getEnv("CACHE_TTL_MINUTES", "30"))
	ID, _ := strconv.ParseInt(os.Getenv("ADMIN_TELEGRAM_ID"), 10, 64)

	// Дефолтный URL OpenAI, но если используешь прокси - берем из .env
	aiBaseURL := getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1")

	return &Config{
		TelegramToken:        os.Getenv("TELEGRAM_TOKEN"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		RedisURL:             getEnv("REDIS_URL", "redis://localhost:6379"),
		CacheTTL:             time.Duration(cacheTTL) * time.Minute,
		TBankTerminalKey:     os.Getenv("TBANK_TERMINAL_KEY"),
		TBankSecretKey:       os.Getenv("TBANK_SECRET_KEY"),
		TBankBaseURL:         getEnv("TBANK_BASE_URL", "https://securepay.tinkoff.ru/v2"),
		SubscriptionPrice:    price,
		ProSubscriptionPrice: proPrice,
		SubscriptionDays:     days,
		WebhookURL:           os.Getenv("WEBHOOK_URL"),
		ServerPort:           getEnv("SERVER_PORT", "8080"),
		AdminTelegramID:      ID,
		ScraperAPIKey:        os.Getenv("SCRAPER_API_KEY"),
		OpenAIKey:            os.Getenv("OPENAI_KEY"),
		OpenAIBaseURL:        aiBaseURL,
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
