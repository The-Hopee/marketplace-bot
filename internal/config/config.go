package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken     string
	DatabasePath      string
	SubscriptionPrice int
	FreeSearchLimit   int
	AdminID           int64
	ProxyURL          string
	Debug             bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	adminID, _ := strconv.ParseInt(os.Getenv("ADMIN_ID"), 10, 64)
	freeLimit, _ := strconv.Atoi(os.Getenv("FREE_SEARCH_LIMIT"))
	if freeLimit == 0 {
		freeLimit = 5
	}

	return &Config{
		TelegramToken:     os.Getenv("TELEGRAM_BOT_TOKEN"),
		DatabasePath:      getEnvOrDefault("DATABASE_PATH", "./bot.db"),
		SubscriptionPrice: 399,
		FreeSearchLimit:   freeLimit,
		AdminID:           adminID,
		ProxyURL:          os.Getenv("PROXY_URL"),
		Debug:             os.Getenv("DEBUG") == "true",
	}, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
