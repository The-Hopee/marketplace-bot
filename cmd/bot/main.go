package bot

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"The-Hopee/marketplace-bot/internal/bot"
	"The-Hopee/marketplace-bot/internal/config"
	"The-Hopee/marketplace-bot/internal/database"
)

func main() {
	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	if cfg.TelegramToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен")
	}

	// Подключаемся к БД
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}
	defer db.Close()

	// Создаём бота
	b, err := bot.New(cfg, db)
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Получен сигнал завершения, останавливаем бота...")
		cancel()
	}()

	// Запускаем бота
	if err := b.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Ошибка работы бота: %v", err)
	}

	log.Println("Бот остановлен")
}
