package subscription

import (
	"context"
	"fmt"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	"marketplace-bot/internal/payment"
	"time"
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

	// Продлеваем подписку
	if err := s.repo.ExtendSubscription(ctx, paymentRecord.TelegramID, s.cfg.SubscriptionDays); err != nil {
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

func (s *Service) CanUserSearch(ctx context.Context, telegramID int64) (bool, int, error) {
	user, err := s.repo.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false, 0, err
	}

	// Подписка — безлимит
	if user.HasActiveSubscription() {
		return true, -1, nil
	}

	// Basic: 5 поисков в день
	const dailyLimit = 5
	today := time.Now().UTC().Truncate(24 * time.Hour)

	if user.LastSearchDate == nil || user.LastSearchDate.Before(today) {
		// Новый день — счётчик с нуля
		user.DailySearches = 0
		s.repo.ResetDailySearches(ctx, telegramID)
	}

	if user.DailySearches >= dailyLimit {
		return false, 0, nil
	}

	// Осталось:
	left := dailyLimit - user.DailySearches
	return true, left, nil
}

func (s *Service) UseSearch(ctx context.Context, telegramID int64) error {
	user, err := s.repo.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return err
	}

	if user.HasActiveSubscription() {
		return s.repo.IncrementSearchCount(ctx, telegramID)
	}

	// Basic: увеличиваем дневной счётчик
	return s.repo.IncrementDailySearch(ctx, telegramID)
}
