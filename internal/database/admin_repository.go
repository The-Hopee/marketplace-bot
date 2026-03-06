package database

import (
	"context"
)

// ==================== Admin ====================

func (r *Repository) IsAdmin(ctx context.Context, telegramID int64) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM admins WHERE telegram_id = $1)`,
		telegramID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) AddAdmin(ctx context.Context, telegramID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`INSERT INTO admins (telegram_id) VALUES ($1) ON CONFLICT DO NOTHING`,
		telegramID,
	)
	return err
}

// ==================== Ads ====================

func (r *Repository) GetAllAds(ctx context.Context) ([]Ad, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, name, text, button_text, button_url, is_active,
            priority, views_count, clicks_count, created_at
     FROM ads ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ads []Ad
	for rows.Next() {
		var a Ad
		if err := rows.Scan(&a.ID, &a.Name, &a.Text, &a.ButtonText, &a.ButtonURL,
			&a.IsActive, &a.Priority, &a.ViewsCount, &a.ClicksCount, &a.CreatedAt); err != nil {
			return nil, err
		}
		ads = append(ads, a)
	}
	return ads, nil
}

func (r *Repository) GetActiveAds(ctx context.Context) ([]Ad, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, name, text, button_text, button_url, is_active,
            priority, views_count, clicks_count, created_at
     FROM ads WHERE is_active = true ORDER BY priority DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ads []Ad
	for rows.Next() {
		var a Ad
		if err := rows.Scan(&a.ID, &a.Name, &a.Text, &a.ButtonText, &a.ButtonURL,
			&a.IsActive, &a.Priority, &a.ViewsCount, &a.ClicksCount, &a.CreatedAt); err != nil {
			return nil, err
		}
		ads = append(ads, a)
	}
	return ads, nil
}

func (r *Repository) GetAdByID(ctx context.Context, id int64) (*Ad, error) {
	var a Ad
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, name, text, button_text, button_url, is_active,
            priority, views_count, clicks_count, created_at
     FROM ads WHERE id = $1`, id,
	).Scan(&a.ID, &a.Name, &a.Text, &a.ButtonText, &a.ButtonURL,
		&a.IsActive, &a.Priority, &a.ViewsCount, &a.ClicksCount, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) CreateAd(ctx context.Context, ad *Ad) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO ads (name, text, button_text, button_url, is_active, priority)
     VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		ad.Name, ad.Text, ad.ButtonText, ad.ButtonURL, ad.IsActive, ad.Priority,
	).Scan(&ad.ID, &ad.CreatedAt)
}

func (r *Repository) UpdateAd(ctx context.Context, ad *Ad) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE ads SET name=$2, text=$3, button_text=$4, button_url=$5,
                   is_active=$6, priority=$7
     WHERE id=$1`,
		ad.ID, ad.Name, ad.Text, ad.ButtonText, ad.ButtonURL, ad.IsActive, ad.Priority,
	)
	return err
}

func (r *Repository) DeleteAd(ctx context.Context, id int64) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM ads WHERE id = $1`, id)
	return err
}

func (r *Repository) IncrementAdViews(ctx context.Context, id int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE ads SET views_count = views_count + 1 WHERE id = $1`, id)
	return err
}

func (r *Repository) IncrementAdClicks(ctx context.Context, id int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE ads SET clicks_count = clicks_count + 1 WHERE id = $1`, id)
	return err
}

// ==================== Broadcasts ====================

func (r *Repository) GetAllBroadcasts(ctx context.Context) ([]Broadcast, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, name, text, button_text, button_url, status,
            total_users, sent_count, failed_count, last_user_id, created_at
     FROM broadcasts ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Broadcast
	for rows.Next() {
		var b Broadcast
		if err := rows.Scan(&b.ID, &b.Name, &b.Text, &b.ButtonText, &b.ButtonURL,
			&b.Status, &b.TotalUsers, &b.SentCount, &b.FailedCount,
			&b.LastUserID, &b.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, nil
}

func (r *Repository) GetBroadcastByID(ctx context.Context, id int64) (*Broadcast, error) {
	var b Broadcast
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, name, text, button_text, button_url, status,
            total_users, sent_count, failed_count, last_user_id, created_at
     FROM broadcasts WHERE id = $1`, id,
	).Scan(&b.ID, &b.Name, &b.Text, &b.ButtonText, &b.ButtonURL,
		&b.Status, &b.TotalUsers, &b.SentCount, &b.FailedCount,
		&b.LastUserID, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) CreateBroadcast(ctx context.Context, b *Broadcast) error {
	return r.db.Pool.QueryRow(ctx,
		`INSERT INTO broadcasts (name, text, button_text, button_url, status)
     VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		b.Name, b.Text, b.ButtonText, b.ButtonURL, b.Status,
	).Scan(&b.ID, &b.CreatedAt)
}

func (r *Repository) UpdateBroadcastStatus(ctx context.Context, id int64, status BroadcastStatus) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE broadcasts SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (r *Repository) UpdateBroadcastProgress(ctx context.Context, id int64, sent, failed int, lastUserID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE broadcasts SET sent_count=$2, failed_count=$3, last_user_id=$4 WHERE id=$1`,
		id, sent, failed, lastUserID)
	return err
}

func (r *Repository) SetBroadcastTotalUsers(ctx context.Context, id int64, total int) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE broadcasts SET total_users = $2 WHERE id = $1`, id, total)
	return err
}

func (r *Repository) GetUsersForBroadcast(ctx context.Context, afterID int64, limit int) ([]User, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, telegram_id, username, first_name, last_name, subscription_end,
            is_active, search_count, free_searches_left, created_at, updated_at
     FROM users WHERE id > $1 AND is_active = true
     ORDER BY id ASC LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName,
			&u.SubscriptionEnd, &u.IsActive, &u.SearchCount, &u.FreeSearchesLeft,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *Repository) GetTotalUsersCount(ctx context.Context) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// ==================== Промокоды ====================

func (r *Repository) GetAllPromocodes(ctx context.Context) ([]Promocode, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, code, free_days, max_uses, used_count, is_active, created_at
     FROM promocodes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Promocode
	for rows.Next() {
		var p Promocode
		if err := rows.Scan(&p.ID, &p.Code, &p.FreeDays, &p.MaxUses,
			&p.UsedCount, &p.IsActive, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (r *Repository) GetPromocodeByCode(ctx context.Context, code string) (*Promocode, error) {
	var p Promocode
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, code, free_days, max_uses, used_count, is_active, created_at
     FROM promocodes WHERE code = $1`, code,
	).Scan(&p.ID, &p.Code, &p.FreeDays, &p.MaxUses, &p.UsedCount, &p.IsActive, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func (r *Repository) CreatePromocode(ctx context.Context, code string, freeDays, maxUses int) error {
	var maxUsesPtr *int
	if maxUses > 0 {
		maxUsesPtr = &maxUses
	}
	_, err := r.db.Pool.Exec(ctx,
		`INSERT INTO promocodes (code, free_days, max_uses) VALUES ($1, $2, $3)`,
		code, freeDays, maxUsesPtr)
	return err
}

func (r *Repository) DeletePromocode(ctx context.Context, code string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM promocodes WHERE code = $1`, code)
	return err
}

func (r *Repository) TogglePromocode(ctx context.Context, code string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE promocodes SET is_active = NOT is_active WHERE code = $1`, code)
	return err
}

func (r *Repository) IncrementPromoUsage(ctx context.Context, code string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE promocodes SET used_count = used_count + 1 WHERE code = $1`, code)
	return err
}

func (r *Repository) HasUsedPromo(ctx context.Context, telegramID int64, code string) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM promo_usages WHERE telegram_id=$1 AND promo_code=$2)`,
		telegramID, code,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) RecordPromoUsage(ctx context.Context, telegramID int64, code string) error {
	_, err := r.db.Pool.Exec(ctx,
		`INSERT INTO promo_usages (telegram_id, promo_code) VALUES ($1, $2)
	   ON CONFLICT DO NOTHING`, telegramID, code)
	return err
}

// ==================== Рефералы ====================

func (r *Repository) CreateReferral(ctx context.Context, referrerID, referredID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`INSERT INTO referrals (referrer_telegram_id, referred_telegram_id)
	   VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		referrerID, referredID)
	return err
}

func (r *Repository) GetReferralByReferred(ctx context.Context, referredID int64) (*Referral, error) {
	var ref Referral
	err := r.db.Pool.QueryRow(ctx,
		`SELECT id, referrer_telegram_id, referred_telegram_id,
			  referrer_reg_bonus_given, referred_reg_bonus_given,
			  referrer_search_bonus_given, referred_search_bonus_given, created_at
	   FROM referrals WHERE referred_telegram_id = $1`, referredID,
	).Scan(&ref.ID, &ref.ReferrerTelegramID, &ref.ReferredTelegramID,
		&ref.ReferrerRegBonusGiven, &ref.ReferredRegBonusGiven,
		&ref.ReferrerSearchBonusGiven, &ref.ReferredSearchBonusGiven, &ref.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (r *Repository) GetReferralsByReferrer(ctx context.Context, referrerID int64) ([]Referral, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, referrer_telegram_id, referred_telegram_id,
			  referrer_reg_bonus_given, referred_reg_bonus_given,
			  referrer_search_bonus_given, referred_search_bonus_given, created_at
	   FROM referrals WHERE referrer_telegram_id = $1 ORDER BY created_at DESC`,
		referrerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var ref Referral
		if err := rows.Scan(&ref.ID, &ref.ReferrerTelegramID, &ref.ReferredTelegramID,
			&ref.ReferrerRegBonusGiven, &ref.ReferredRegBonusGiven,
			&ref.ReferrerSearchBonusGiven, &ref.ReferredSearchBonusGiven, &ref.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, ref)
	}
	return list, nil
}

func (r *Repository) GetReferralCount(ctx context.Context, referrerID int64) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM referrals WHERE referrer_telegram_id = $1`, referrerID,
	).Scan(&count)
	return count, err
}

func (r *Repository) MarkRegBonusGiven(ctx context.Context, referredID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE referrals SET referrer_reg_bonus_given=true, referred_reg_bonus_given=true
	   WHERE referred_telegram_id = $1`, referredID)
	return err
}
func (r *Repository) MarkSearchBonusGiven(ctx context.Context, referredID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE referrals SET referrer_search_bonus_given=true, referred_search_bonus_given=true
	   WHERE referred_telegram_id = $1`, referredID)
	return err
}

func (r *Repository) SetUserReferredBy(ctx context.Context, telegramID, referrerID int64) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE users SET referred_by = $2 WHERE telegram_id = $1 AND referred_by IS NULL`,
		telegramID, referrerID)
	return err
}
