package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"marketplace-bot/internal/bot"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
)

func main() {
	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Подключаемся к базе данных
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Выполняем миграции
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// Создаем и запускаем бота
	telegramBot, err := bot.New(cfg, db)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Graceful shutdown
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
