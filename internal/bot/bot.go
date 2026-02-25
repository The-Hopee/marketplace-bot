package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	"marketplace-bot/internal/marketplace"
	"marketplace-bot/internal/payment"
	"marketplace-bot/internal/subscription"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api        *tgbotapi.BotAPI
	handler    *Handler
	cfg        *config.Config
	repo       *database.Repository
	subService *subscription.Service
}

func New(cfg *config.Config, db *database.DB) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	repo := database.NewRepository(db)
	tbankClient := payment.NewTBankClient(cfg.TBankTerminalKey, cfg.TBankSecretKey, cfg.TBankBaseURL)
	subService := subscription.NewService(repo, tbankClient, cfg)
	aggregator := marketplace.NewAggregator()

	handler := NewHandler(api, repo, aggregator, subService, cfg)

	return &Bot{
		api:        api,
		handler:    handler,
		cfg:        cfg,
		repo:       repo,
		subService: subService,
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	// Запускаем webhook сервер для платежей
	go b.startPaymentWebhook()

	// Запускаем polling для Telegram
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Println("Bot started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			go b.handler.HandleUpdate(update)
		}
	}
}

func (b *Bot) startPaymentWebhook() {
	http.HandleFunc("/webhook/payment", b.handlePaymentWebhook)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Starting webhook server on :%s", b.cfg.ServerPort)
	if err := http.ListenAndServe(":"+b.cfg.ServerPort, nil); err != nil {
		log.Printf("Webhook server error: %v", err)
	}
}

func (b *Bot) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var notification payment.NotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		log.Printf("Error decoding payment notification: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	log.Printf("Payment notification received: OrderID=%s, Status=%s", notification.OrderId, notification.Status)

	// Проверяем статус платежа
	if notification.Status == "CONFIRMED" {
		ctx := context.Background()
		telegramID, err := b.subService.ConfirmPayment(
			ctx,
			notification.OrderId,
			fmt.Sprintf("%d", notification.PaymentId),
		)
		if err != nil {
			log.Printf("Error confirming payment: %v", err)
		} else {
			// Отправляем уведомление пользователю
			msg := tgbotapi.NewMessage(telegramID, "🎉 Оплата прошла успешно!\n\n✅ Подписка активирована на 30 дней.\n\nТеперь вам доступен безлимитный поиск!")
			msg.ReplyMarkup = MainMenuKeyboard()
			b.api.Send(msg)
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
