package database

import (
	"os"
	"strconv"
	"time"
)

const (
	TierFree    = "free"
	TierPremium = "premium"
	TierPro     = "pro"
)

type User struct {
	ID               int64      `json:"id"`
	TelegramID       int64      `json:"telegram_id"`
	Username         string     `json:"username"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	SubscriptionEnd  *time.Time `json:"subscription_end"`
	SubscriptionTier string     `json:"subscription_tier"`
	IsActive         bool       `json:"is_active"`
	SearchCount      int        `json:"search_count"`
	DailySearches    int        `json:"daily_searches"`
	FreeSearchesLeft int        `json:"free_searches_left"`

	// Новые раздельные лимиты (использовано за день)
	DailyWbText    int        `json:"daily_wb_text"`
	DailyOzonText  int        `json:"daily_ozon_text"`
	DailyImage     int        `json:"daily_image"`
	LastSearchDate *time.Time `json:"last_search_date"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	City string `json:"city"`
}

type Payment struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	TelegramID  int64      `json:"telegram_id"`
	OrderID     string     `json:"order_id"`
	PaymentID   string     `json:"payment_id"`
	Amount      int64      `json:"amount"` // в копейках
	Status      string     `json:"status"` // pending, confirmed, rejected, refunded
	PaymentURL  string     `json:"payment_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
}

type SearchHistory struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Query       string    `json:"query"`
	ResultCount int       `json:"result_count"`
	CreatedAt   time.Time `json:"created_at"`
}

func getOwnerID() int64 {
	id, _ := strconv.ParseInt(os.Getenv("ADMIN_TELEGRAM_ID"), 10, 64)
	return id
}

func (u *User) HasActiveSubscription() bool {
	if u.TelegramID == getOwnerID() {
		return true
	}
	if u.SubscriptionEnd == nil {
		return false
	}
	return u.SubscriptionEnd.After(time.Now()) && (u.SubscriptionTier == TierPremium || u.SubscriptionTier == TierPro)
}

func (u *User) IsPro() bool {
	if u.TelegramID == getOwnerID() {
		return true
	}
	return u.HasActiveSubscription() && u.SubscriptionTier == TierPro
}

// НОВАЯ ФУНКЦИЯ: возвращает реальный уровень подписки
func (u *User) GetTier() string {
	// Админ всегда PRO
	if u.TelegramID == getOwnerID() {
		return TierPro
	}

	// Проверяем активную подписку
	if u.SubscriptionEnd != nil && u.SubscriptionEnd.After(time.Now()) {
		if u.SubscriptionTier != "" {
			return u.SubscriptionTier
		}
		return TierPremium // Для старых записей без tier
	}

	return TierFree
}
