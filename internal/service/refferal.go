package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"marketplace-bot/internal/database"
)

const (
	ReferralBonusDays    = 7  // +7 дней за каждое событие
	ReferralMaxInvites   = 10 // макс 10 приглашений
	ReferralUnlockDays   = 3  // доступно с 3го дня
	ReferralSearchTarget = 20 // 20 поисков для бонуса
)

type ReferralService struct {
	repo    *database.Repository
	botName string
}

func NewReferralService(repo *database.Repository, botName string) *ReferralService {
	return &ReferralService{repo: repo, botName: botName}
}

// GetReferralLink — реферальная ссылка пользователя
func (s *ReferralService) GetReferralLink(telegramID int64) string {
	return fmt.Sprintf("https://t.me/%s?start=ref_%d", s.botName, telegramID)
}

// CanUseReferral — можно ли пользоваться реферальной программой
// daysLeft > 0 означает «ещё не разблокировано»
func (s *ReferralService) CanUseReferral(ctx context.Context, telegramID int64) (canUse bool, daysLeft int, err error) {
	user, err := s.repo.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false, 0, err
	}

	daysSinceReg := int(time.Since(user.CreatedAt).Hours() / 24)
	if daysSinceReg < ReferralUnlockDays {
		return false, ReferralUnlockDays - daysSinceReg, nil
	}

	count, err := s.repo.GetReferralCount(ctx, telegramID)
	if err != nil {
		return false, 0, err
	}
	if count >= ReferralMaxInvites {
		return false, 0, nil // лимит, daysLeft=0
	}

	return true, 0, nil
}

// ProcessNewReferral — вызывается при /start ref_XXX
//
// Бонус за РЕГИСТРАЦИЮ:
//   - реферер:      +7 дней
//   - приглашённый:  +7 дней
func (s *ReferralService) ProcessNewReferral(ctx context.Context, referrerID, referredID int64) error {
	if referrerID == referredID {
		return fmt.Errorf("self-referral")
	}

	// Реферер должен существовать
	if _, err := s.repo.GetUserByTelegramID(ctx, referrerID); err != nil {
		return fmt.Errorf("referrer not found")
	}

	// Проверяем лимит
	count, _ := s.repo.GetReferralCount(ctx, referrerID)
	if count >= ReferralMaxInvites {
		return fmt.Errorf("referral limit reached")
	}

	// Создаём связь
	if err := s.repo.CreateReferral(ctx, referrerID, referredID); err != nil {
		return err
	}
	_ = s.repo.SetUserReferredBy(ctx, referredID, referrerID)

	// +7 дней обоим (бонус за регистрацию)
	if err := s.repo.ExtendSubscription(ctx, referrerID, ReferralBonusDays); err != nil {
		log.Printf("referral: extend referrer %d: %v", referrerID, err)
	}
	if err := s.repo.ExtendSubscription(ctx, referredID, ReferralBonusDays); err != nil {
		log.Printf("referral: extend referred %d: %v", referredID, err)
	}
	_ = s.repo.MarkRegBonusGiven(ctx, referredID)

	log.Printf("Referral REG bonus: %d invited %d, +%d days each", referrerID, referredID, ReferralBonusDays)
	return nil
}

// CheckSearchBonus — вызывается после каждого поиска.
//
// Когда приглашённый достигает 20 поисков:
//   - реферер:      +7 дней
//   - приглашённый:  +7 дней
//
// Возвращает (true, referrerID) если бонус только что выдан.
func (s *ReferralService) CheckSearchBonus(ctx context.Context, telegramID int64) (bonusGiven bool, referrerID int64, err error) {
	user, err := s.repo.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return false, 0, err
	}

	// Ещё не набрал 20 поисков
	if user.SearchCount < ReferralSearchTarget {
		return false, 0, nil
	}

	// Проверяем, был ли приглашён
	ref, err := s.repo.GetReferralByReferred(ctx, telegramID)
	if err != nil {
		return false, 0, nil // не был приглашён — ок
	}

	// Бонус уже выдан
	if ref.ReferredSearchBonusGiven {
		return false, 0, nil
	}

	// +7 дней обоим (бонус за 20 поисков)
	_ = s.repo.ExtendSubscription(ctx, ref.ReferrerTelegramID, ReferralBonusDays)
	_ = s.repo.ExtendSubscription(ctx, telegramID, ReferralBonusDays)
	_ = s.repo.MarkSearchBonusGiven(ctx, telegramID)

	log.Printf("Referral SEARCH bonus: referred=%d referrer=%d, +%d days each",
		telegramID, ref.ReferrerTelegramID, ReferralBonusDays)

	return true, ref.ReferrerTelegramID, nil
}

// GetStats — статистика рефералов для отображения в профиле
func (s *ReferralService) GetStats(ctx context.Context, telegramID int64) (total, searchBonuses int, err error) {
	refs, err := s.repo.GetReferralsByReferrer(ctx, telegramID)
	if err != nil {
		return 0, 0, err
	}
	total = len(refs)
	for _, r := range refs {
		if r.ReferrerSearchBonusGiven {
			searchBonuses++
		}
	}
	return total, searchBonuses, nil
}
