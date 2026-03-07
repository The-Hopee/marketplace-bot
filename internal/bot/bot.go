package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"marketplace-bot/internal/cache"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	"marketplace-bot/internal/marketplace"
	"marketplace-bot/internal/payment"
	"marketplace-bot/internal/service"
	"marketplace-bot/internal/subscription"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api         *tgbotapi.BotAPI
	handler     *Handler
	cfg         *config.Config
	subService  *subscription.Service
	tbankClient *payment.TBankClient
}

func New(cfg *config.Config, db *database.DB, redisCache *cache.RedisCache) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}
	log.Printf("Authorized on account %s", api.Self.UserName)

	repo := database.NewRepository(db)

	// T-Bank
	notificationURL := ""
	if cfg.WebhookURL != "" {
		notificationURL = cfg.WebhookURL + "/webhook/payment"
	}
	tbankClient := payment.NewTBankClient(
		cfg.TBankTerminalKey,
		cfg.TBankSecretKey,
		cfg.TBankBaseURL,
		notificationURL,
	)

	// Сервисы
	subService := subscription.NewService(repo, tbankClient, cfg)
	adSvc := service.NewAdService(repo)
	broadcastSvc := service.NewBroadcastService(api, repo)
	referralSvc := service.NewReferralService(repo, api.Self.UserName)

	// Админ-хендлеры
	adminHandlers := NewAdminHandlers(api, repo, broadcastSvc, adSvc, cfg.AdminTelegramID)

	// Агрегатор
	aggregator := marketplace.NewAggregator()

	// Основной хендлер — передаём новые зависимости
	handler := NewHandler(api, repo, aggregator, subService, redisCache, cfg,
		adminHandlers, referralSvc)

	return &Bot{
		api:         api,
		handler:     handler,
		cfg:         cfg,
		subService:  subService,
		tbankClient: tbankClient,
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	go b.startPaymentWebhook()

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
		log.Fatalf("webhook server error: %v", err)
	}
}

func (b *Bot) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var notification payment.NotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		log.Printf("webhook: bad json: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	log.Printf("Payment notification: OrderID=%s Status=%s PaymentId=%d",
		notification.OrderId, notification.Status, notification.PaymentId)

	if !b.tbankClient.VerifyNotification(&notification) {
		log.Printf("webhook: invalid token for order %s", notification.OrderId)
		http.Error(w, "Invalid token", http.StatusForbidden)
		return
	}

	if notification.Status == "CONFIRMED" {
		ctx := context.Background()
		telegramID, err := b.subService.ConfirmPayment(
			ctx,
			notification.OrderId,
			notification.PaymentId,
		)
		if err != nil {
			log.Printf("Error confirming payment: %v", err)
		} else {
			msg := tgbotapi.NewMessage(telegramID,
				"🎉 Оплата прошла! Подписка активирована на 30 дней.")
			msg.ReplyMarkup = MainMenuKeyboard()
			b.api.Send(msg)
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
