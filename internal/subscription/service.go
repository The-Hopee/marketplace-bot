package subscription

import (
	"context"
	"fmt"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	"marketplace-bot/internal/payment"
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

func (s *Service) CreateSubscriptionPayment(ctx context.Context, telegramID int64, username string) (*PaymentInfo, error) {
	// Создаем платеж в T-Bank
	userData := map[string]string{
		"TelegramID": fmt.Sprintf("%d", telegramID),
		"Username":   username,
	}

	initResp, err := s.tbank.InitPayment(
		ctx,
		s.cfg.SubscriptionPrice,
		"Подписка на MarketBot (1 месяц)",
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

	// Продлеваем подписку (добавили "premium")
	if err := s.repo.ExtendSubscription(ctx, paymentRecord.TelegramID, s.cfg.SubscriptionDays, "premium"); err != nil {
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
