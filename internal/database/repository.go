package database

import (
	"context"
)

type Repository struct {
	db *DB
}

func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// User methods
func (r *Repository) CreateUser(ctx context.Context, telegramID int64, username, firstName, lastName string) (*User, error) {
	query := `
		INSERT INTO users (telegram_id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (telegram_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, telegram_id, username, first_name, last_name, subscription_end, 
				  is_active, search_count, free_searches_left, created_at, updated_at
	`

	var user User
	err := r.db.Pool.QueryRow(ctx, query, telegramID, username, firstName, lastName).Scan(
		&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.LastName,
		&user.SubscriptionEnd, &user.IsActive, &user.SearchCount, &user.FreeSearchesLeft,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByTelegramID(ctx context.Context, telegramID int64) (*User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, last_name, subscription_end,
			   is_active, search_count, free_searches_left, created_at, updated_at
		FROM users WHERE telegram_id = $1
	`

	var user User
	err := r.db.Pool.QueryRow(ctx, query, telegramID).Scan(
		&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.LastName,
		&user.SubscriptionEnd, &user.IsActive, &user.SearchCount, &user.FreeSearchesLeft,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) ExtendSubscription(ctx context.Context, telegramID int64, days int) error {
	query := `
		UPDATE users SET 
			subscription_end = CASE 
				WHEN subscription_end IS NULL OR subscription_end < CURRENT_TIMESTAMP 
				THEN CURRENT_TIMESTAMP + INTERVAL '1 day' * $2
				ELSE subscription_end + INTERVAL '1 day' * $2
			END,
			updated_at = CURRENT_TIMESTAMP
		WHERE telegram_id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, telegramID, days)
	return err
}

func (r *Repository) DecrementFreeSearches(ctx context.Context, telegramID int64) error {
	query := `
		UPDATE users SET 
			free_searches_left = free_searches_left - 1,
			search_count = search_count + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE telegram_id = $1 AND free_searches_left > 0
	`
	_, err := r.db.Pool.Exec(ctx, query, telegramID)
	return err
}

func (r *Repository) IncrementSearchCount(ctx context.Context, telegramID int64) error {
	query := `
		UPDATE users SET 
			search_count = search_count + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE telegram_id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, telegramID)
	return err
}

// Payment methods
func (r *Repository) CreatePayment(ctx context.Context, payment *Payment) error {
	query := `
		INSERT INTO payments (user_id, telegram_id, order_id, amount, status, payment_url)
		VALUES ((SELECT id FROM users WHERE telegram_id = $1), $1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	return r.db.Pool.QueryRow(ctx, query,
		payment.TelegramID, payment.OrderID, payment.Amount, payment.Status, payment.PaymentURL,
	).Scan(&payment.ID, &payment.CreatedAt)
}

func (r *Repository) UpdatePaymentStatus(ctx context.Context, orderID, status, paymentID string) error {
	query := `
		UPDATE payments SET 
			status = $2,
			payment_id = $3,
			confirmed_at = CASE WHEN $2 = 'confirmed' THEN CURRENT_TIMESTAMP ELSE confirmed_at END
		WHERE order_id = $1
	`
	_, err := r.db.Pool.Exec(ctx, query, orderID, status, paymentID)
	return err
}

func (r *Repository) GetPaymentByOrderID(ctx context.Context, orderID string) (*Payment, error) {
	query := `
		SELECT id, user_id, telegram_id, order_id, payment_id, amount, status, payment_url, created_at, confirmed_at
		FROM payments WHERE order_id = $1
	`
	var payment Payment
	err := r.db.Pool.QueryRow(ctx, query, orderID).Scan(
		&payment.ID, &payment.UserID, &payment.TelegramID, &payment.OrderID,
		&payment.PaymentID, &payment.Amount, &payment.Status, &payment.PaymentURL,
		&payment.CreatedAt, &payment.ConfirmedAt,
	)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// Search history
func (r *Repository) SaveSearchHistory(ctx context.Context, telegramID int64, query string, resultCount int) error {
	q := `
		INSERT INTO search_history (user_id, query, result_count)
		VALUES ((SELECT id FROM users WHERE telegram_id = $1), $2, $3)
	`
	_, err := r.db.Pool.Exec(ctx, q, telegramID, query, resultCount)
	return err
}

func (r *Repository) GetUserStats(ctx context.Context, telegramID int64) (totalSearches int, err error) {
	query := `SELECT search_count FROM users WHERE telegram_id = $1`
	err = r.db.Pool.QueryRow(ctx, query, telegramID).Scan(&totalSearches)
	return
}

// Admin methods
func (r *Repository) GetAllUsers(ctx context.Context) ([]User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, last_name, subscription_end,
			   is_active, search_count, free_searches_left, created_at, updated_at
		FROM users ORDER BY created_at DESC
	`
	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(
			&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.LastName,
			&user.SubscriptionEnd, &user.IsActive, &user.SearchCount, &user.FreeSearchesLeft,
			&user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (r *Repository) GetActiveSubscribersCount(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE subscription_end > CURRENT_TIMESTAMP`
	var count int
	err := r.db.Pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}

func (r *Repository) GetTotalRevenue(ctx context.Context) (int64, error) {
	query := `SELECT COALESCE(SUM(amount), 0) FROM payments WHERE status = 'confirmed'`
	var total int64
	err := r.db.Pool.QueryRow(ctx, query).Scan(&total)
	return total, err
}
