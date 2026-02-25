package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func NewDB(databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) Migrate(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		telegram_id BIGINT UNIQUE NOT NULL,
		username VARCHAR(255),
		first_name VARCHAR(255),
		last_name VARCHAR(255),
		subscription_end TIMESTAMP,
		is_active BOOLEAN DEFAULT true,
		search_count INTEGER DEFAULT 0,
		free_searches_left INTEGER DEFAULT 5,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS payments (
		id SERIAL PRIMARY KEY,
		user_id INTEGER REFERENCES users(id),
		telegram_id BIGINT NOT NULL,
		order_id VARCHAR(255) UNIQUE NOT NULL,
		payment_id VARCHAR(255),
		amount BIGINT NOT NULL,
		status VARCHAR(50) DEFAULT 'pending',
		payment_url TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		confirmed_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS search_history (
		id SERIAL PRIMARY KEY,
		user_id INTEGER REFERENCES users(id),
		query VARCHAR(500) NOT NULL,
		result_count INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
	CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);
	CREATE INDEX IF NOT EXISTS idx_payments_telegram_id ON payments(telegram_id);
	CREATE INDEX IF NOT EXISTS idx_search_history_user_id ON search_history(user_id);
	`

	_, err := db.Pool.Exec(ctx, query)
	return err
}
