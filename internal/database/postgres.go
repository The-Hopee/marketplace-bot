package database

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

// NewDB подключается к БД и запускает миграции
func NewDB(databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	db := &DB{Pool: pool}

	// Автоматически запускаем миграции при создании пула
	if err := db.Migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return db, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

// Migrate создаёт все таблицы и добавляет необходимые колонки/индексы
func (db *DB) Migrate(ctx context.Context) error {
	log.Println("[DB] Running migrations...")

	// ======================================================================
	// 1. Пользователи (users) + ДОПОЛНИТЕЛЬНЫЕ КОЛОНКИ
	// ======================================================================
	_, err := db.Pool.Exec(ctx, `
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
      referred_by BIGINT,                     -- НОВОЕ: кто пригласил
      cross_promo_shown BOOLEAN DEFAULT false, -- НОВОЕ: показано ли промо другого бота
      last_weekly_promo TIMESTAMP,             -- НОВОЕ: когда последний раз отправляли еженедельное промо
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	// ======================================================================
	// 2. Платежи (payments)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS payments (
      id SERIAL PRIMARY KEY,
      user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
      telegram_id BIGINT NOT NULL,
      order_id VARCHAR(255) UNIQUE NOT NULL,
      payment_id VARCHAR(255),
      amount BIGINT NOT NULL,
      status VARCHAR(50) DEFAULT 'pending',
      payment_url TEXT,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      confirmed_at TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create payments table: %w", err)
	}

	// ======================================================================
	// 3. История поиска (search_history)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS search_history (
      id SERIAL PRIMARY KEY,
      user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
      query VARCHAR(500) NOT NULL,
      result_count INTEGER DEFAULT 0,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create search_history table: %w", err)
	}

	// ======================================================================
	// 4. АДМИНЫ (admins)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS admins (
      id SERIAL PRIMARY KEY,
      telegram_id BIGINT UNIQUE NOT NULL,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create admins table: %w", err)
	}
	// ======================================================================
	// 5. РЕКЛАМА (ads)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS ads (
      id SERIAL PRIMARY KEY,
      name VARCHAR(255) NOT NULL,
      text TEXT NOT NULL,
      button_text VARCHAR(255),
      button_url VARCHAR(500),
      is_active BOOLEAN DEFAULT true,
      priority INTEGER DEFAULT 1,
      views_count INTEGER DEFAULT 0,
      clicks_count INTEGER DEFAULT 0,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create ads table: %w", err)
	}

	// ======================================================================
	// 6. РАССЫЛКИ (broadcasts)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS broadcasts (
      id SERIAL PRIMARY KEY,
      name VARCHAR(255) NOT NULL,
      text TEXT NOT NULL,
      button_text VARCHAR(255),
      button_url VARCHAR(500),
      status VARCHAR(20) DEFAULT 'draft',        -- draft / running / paused / completed
      total_users INTEGER DEFAULT 0,
      sent_count INTEGER DEFAULT 0,
      failed_count INTEGER DEFAULT 0,
      last_user_id BIGINT DEFAULT 0,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create broadcasts table: %w", err)
	}

	// ======================================================================
	// 7. ПРОМОКОДЫ (promocodes)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS promocodes (
      id SERIAL PRIMARY KEY,
      code VARCHAR(50) UNIQUE NOT NULL,
      free_days INTEGER DEFAULT 0,
      max_uses INTEGER,
      used_count INTEGER DEFAULT 0,
      is_active BOOLEAN DEFAULT true,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS promo_usages (
      id SERIAL PRIMARY KEY,
      telegram_id BIGINT NOT NULL,
      promo_code VARCHAR(50) NOT NULL,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      UNIQUE(telegram_id, promo_code)
    );
  `)
	if err != nil {
		return fmt.Errorf("create promocodes tables: %w", err)
	}

	// Сид: промокод FIRST (30 дней, лимит 100 использований)
	_, err = db.Pool.Exec(ctx, `
    INSERT INTO promocodes (code, free_days, max_uses, is_active)
    VALUES ('FIRST', 30, 100, true)
    ON CONFLICT (code) DO NOTHING;
  `)
	if err != nil {
		return fmt.Errorf("insert seed promocode FIRST: %w", err)
	}

	// ======================================================================
	// 8. РЕФЕРАЛЫ (referrals)
	// ======================================================================
	_, err = db.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS referrals (
      id SERIAL PRIMARY KEY,
      referrer_telegram_id BIGINT NOT NULL,
      referred_telegram_id BIGINT UNIQUE NOT NULL,
      referrer_reg_bonus_given BOOLEAN DEFAULT false,
      referred_reg_bonus_given BOOLEAN DEFAULT false,
      referrer_search_bonus_given BOOLEAN DEFAULT false,
      referred_search_bonus_given BOOLEAN DEFAULT false,
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  `)
	if err != nil {
		return fmt.Errorf("create referrals table: %w", err)
	}
	// ======================================================================
	// ИНДЕКСЫ (для скорости)
	// ======================================================================
	indices := []string{
		`CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);`,
		`CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);`,
		`CREATE INDEX IF NOT EXISTS idx_payments_telegram_id ON payments(telegram_id);`,
		`CREATE INDEX IF NOT EXISTS idx_search_history_user_id ON search_history(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_referrals_referred ON referrals(referred_telegram_id);`,
		`CREATE INDEX IF NOT EXISTS idx_promo_usages ON promo_usages(telegram_id, promo_code);`,
	}

	for _, idx := range indices {
		if _, err := db.Pool.Exec(ctx, idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	log.Println("[DB] Migrations completed successfully!")
	return nil
}
