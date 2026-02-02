package models

import "time"

// Product представляет товар с маркетплейса
type Product struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Price       float64 `json:"price"`
	OldPrice    float64 `json:"old_price,omitempty"`
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"review_count"`
	URL         string  `json:"url"`
	ImageURL    string  `json:"image_url"`
	Marketplace string  `json:"marketplace"`
	InStock     bool    `json:"in_stock"`
}

// User представляет пользователя бота
type User struct {
	TelegramID     int64     `json:"telegram_id"`
	Username       string    `json:"username"`
	SubscribedAt   time.Time `json:"subscribed_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	IsActive       bool      `json:"is_active"`
	SearchCount    int       `json:"search_count"`
	LastSearchDate time.Time `json:"last_search_date"`
}

// SearchResult объединяет результаты со всех маркетплейсов
type SearchResult struct {
	Query    string    `json:"query"`
	Products []Product `json:"products"`
	Error    error     `json:"-"`
}
