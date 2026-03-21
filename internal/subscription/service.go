package subscription

import (
	"context"
	"fmt"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	"marketplace-bot/internal/payment"
	"strings"
	"time"
)

const (
	SearchTypeWBText   = "wb_text"
	SearchTypeOzonText = "ozon_text"
	SearchTypeImage    = "image"
	DailyFreeLimit     = 5
)

type Service struct {
	repo  *database.Repository
	tbank *payment.TBankClient
	cfg   *config.Config
}

func NewService(repo *database.Repository, tbank *payment.TBankClient, cfg *config.Config) *Service {
	return &Service{
		repo:  repo,
		tbank: tbank,
		cfg:   cfg,
	}
}

type PaymentInfo struct {
	OrderID    string
	PaymentURL string
	Amount     int64
}

func (s *Service) CreateSubscriptionPayment(ctx context.Context, telegramID int64, username string, tier string) (*PaymentInfo, error) {
	// Определяем цену в зависимости от выбранного тарифа
	amount := s.cfg.SubscriptionPrice
	if tier == "pro" {
		amount = s.cfg.ProSubscriptionPrice
	}

	// Генерируем OrderID и вшиваем туда уровень подписки (чтобы потом прочитать его при подтверждении)
	orderID := fmt.Sprintf("sub_%d_%d_%s", telegramID, time.Now().Unix(), tier)

	// Создаем платеж в T-Bank
	userData := map[string]string{
		"TelegramID": fmt.Sprintf("%d", telegramID),
		"Username":   username,
		"Tier":       tier,
	}

	initResp, err := s.tbank.InitPayment(
		ctx,
		orderID, // Передаем сгенерированный OrderID
		amount,  // Передаем правильную цену (premium или pro)
		fmt.Sprintf("Подписка на MarketBot (%s)", strings.ToUpper(tier)),
		userData,
	)
	if err != nil {
		return nil, err
	}

	// Сохраняем платеж в БД
	paymentRecord := &database.Payment{
		TelegramID: telegramID,
		OrderID:    initResp.OrderId,
		Amount:     initResp.Amount,
		Status:     "pending",
		PaymentURL: initResp.PaymentURL,
	}

	if err := s.repo.CreatePayment(ctx, paymentRecord); err != nil {
		return nil, err
	}

	return &PaymentInfo{
		OrderID:    initResp.OrderId,
		PaymentURL: initResp.PaymentURL,
		Amount:     initResp.Amount,
	}, nil
}

func (s *Service) ConfirmPayment(ctx context.Context, orderID, paymentID string) (int64, error) {
	// Получаем платеж из БД
	paymentRecord, err := s.repo.GetPaymentByOrderID(ctx, orderID)
	if err != nil {
		return 0, err
	}

	// Обновляем статус платежа
	if err := s.repo.UpdatePaymentStatus(ctx, orderID, "confirmed", paymentID); err != nil {
		return 0, err
	}

	// Читаем тариф из конца номера заказа (например: sub_12345_173000000_pro)
	tier := "premium" // по умолчанию
	if strings.HasSuffix(orderID, "_pro") {
		tier = "pro"
	}

	// Продлеваем подписку, выдавая правильный уровень
	if err := s.repo.ExtendSubscription(ctx, paymentRecord.TelegramID, s.cfg.SubscriptionDays, tier); err != nil {
		return 0, err
	}

	return paymentRecord.TelegramID, nil
}

func (s *Service) CheckSubscription(ctx context.Context, telegramID int64) (bool, error) {
	user, err := s.repo.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false, err
	}
	return user.HasActiveSubscription(), nil
}

func (s *Service) CanUserSearch(ctx context.Context, userID int64, searchType string) (bool, int, error) {
	user, err := s.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		return false, 0, err
	}

	// Если наступил новый день (или дата пустая) — сбрасываем счетчики
	if user.LastSearchDate == nil || user.LastSearchDate.Before(time.Now().Truncate(24*time.Hour)) {
		_ = s.repo.ResetDailySearches(ctx, userID)
		user.DailyWbText = 0
		user.DailyOzonText = 0
		user.DailyImage = 0
		now := time.Now()
		user.LastSearchDate = &now
	}

	// Премиум и ПРО имеют безлимит
	if user.HasActiveSubscription() {
		return true, 999, nil
	}

	// Проверяем конкретный лимит в зависимости от типа поиска
	used := 0
	switch searchType {
	case SearchTypeWBText:
		used = user.DailyWbText
	case SearchTypeOzonText:
		used = user.DailyOzonText
	case SearchTypeImage:
		used = user.DailyImage
	}

	left := DailyFreeLimit - used
	if left > 0 {
		return true, left, nil
	}

	return false, 0, nil
}

func (s *Service) UseSearch(ctx context.Context, userID int64, searchType string) error {
	// Инкрементируем нужный счетчик
	return s.repo.IncrementSearchLimit(ctx, userID, searchType)
}
