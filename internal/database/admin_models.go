package database

import "time"

// ==================== Ad ====================

type Ad struct {
	ID          int64
	Name        string
	Text        string
	ButtonText  *string
	ButtonURL   *string
	IsActive    bool
	Priority    int
	ViewsCount  int
	ClicksCount int
	CreatedAt   time.Time
}

// ==================== Broadcast ====================

type BroadcastStatus string

const (
	BroadcastDraft     BroadcastStatus = "draft"
	BroadcastRunning   BroadcastStatus = "running"
	BroadcastPaused    BroadcastStatus = "paused"
	BroadcastCompleted BroadcastStatus = "completed"
)

type Broadcast struct {
	ID          int64
	Name        string
	Text        string
	ButtonText  *string
	ButtonURL   *string
	Status      BroadcastStatus
	TotalUsers  int
	SentCount   int
	FailedCount int
	LastUserID  int64
	CreatedAt   time.Time
}

// ==================== Promocode ====================

type Promocode struct {
	ID        int64
	Code      string
	FreeDays  int
	MaxUses   *int
	UsedCount int
	IsActive  bool
	CreatedAt time.Time
}

type PromoUsage struct {
	ID         int64
	TelegramID int64
	PromoCode  string
	CreatedAt  time.Time
}

// ==================== Referral ====================

type Referral struct {
	ID                       int64
	ReferrerTelegramID       int64
	ReferredTelegramID       int64
	ReferrerRegBonusGiven    bool
	ReferredRegBonusGiven    bool
	ReferrerSearchBonusGiven bool
	ReferredSearchBonusGiven bool
	CreatedAt                time.Time
}
