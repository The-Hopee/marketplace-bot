package database

import (
	"time"
)

type User struct {
	ID               int64      `json:"id"`
	TelegramID       int64      `json:"telegram_id"`
	Username         string     `json:"username"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	SubscriptionEnd  *time.Time `json:"subscription_end"`
	IsActive         bool       `json:"is_active"`
	SearchCount      int        `json:"search_count"`
	FreeSearchesLeft int        `json:"free_searches_left"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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

func (u *User) HasActiveSubscription() bool {
	if u.SubscriptionEnd == nil {
		return false
	}
	return u.SubscriptionEnd.After(time.Now())
}

func (u *User) CanSearch() bool {
	return u.HasActiveSubscription() || u.FreeSearchesLeft > 0
}
