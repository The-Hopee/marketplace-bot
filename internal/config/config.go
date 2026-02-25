package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Telegram
	TelegramToken string

	// Database
	DatabaseURL string

	// T-Bank (Tinkoff)
	TBankTerminalKey string
	TBankSecretKey   string
	TBankBaseURL     string

	// Subscription
	SubscriptionPrice int64 // в копейках
	SubscriptionDays  int

	// Server
	WebhookURL string
	ServerPort string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	price, _ := strconv.ParseInt(getEnv("SUBSCRIPTION_PRICE", "10000"), 10, 64) // 100 рублей = 10000 копеек
	days, _ := strconv.Atoi(getEnv("SUBSCRIPTION_DAYS", "30"))

	return &Config{
		TelegramToken:     os.Getenv("TELEGRAM_TOKEN"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		TBankTerminalKey:  os.Getenv("TBANK_TERMINAL_KEY"),
		TBankSecretKey:    os.Getenv("TBANK_SECRET_KEY"),
		TBankBaseURL:      getEnv("TBANK_BASE_URL", "https://securepay.tinkoff.ru/v2"),
		SubscriptionPrice: price,
		SubscriptionDays:  days,
		WebhookURL:        os.Getenv("WEBHOOK_URL"),
		ServerPort:        getEnv("SERVER_PORT", "8080"),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
