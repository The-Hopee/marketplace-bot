// cmd/bot/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"marketplace-bot/internal/bot"
	"marketplace-bot/internal/cache"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Redis (опционально)
	var redisCache *cache.RedisCache
	if cfg.RedisURL != "" {
		redisCache, err = cache.NewRedisCache(cfg.RedisURL, cfg.CacheTTL)
		if err != nil {
			log.Printf("Redis not available: %v (continuing without cache)", err)
		} else {
			defer redisCache.Close()
		}
	}

	telegramBot, err := bot.New(cfg, db, redisCache)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	if err := telegramBot.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Bot error: %v", err)
	}

	log.Println("Bot stopped")
}
