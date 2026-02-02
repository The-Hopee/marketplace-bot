package database

import (
	"The-Hopee/marketplace-bot.git/internal/models"
	"database/sql"
	"time"
)

type Database struct {
	db *sql.DB
}

func New(path string) (*Database, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	d := &Database{db: db}
	if err := d.migrate(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Database) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
	  telegram_id INTEGER PRIMARY KEY,
	  username TEXT,
	  subscribed_at DATETIME,
	  expires_at DATETIME,
	  is_active BOOLEAN DEFAULT 0,
	  search_count INTEGER DEFAULT 0,
	  last_search_date DATE,
	  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE TABLE IF NOT EXISTS payments (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  telegram_id INTEGER,
	  amount INTEGER,
	  status TEXT,
	  payment_id TEXT,
	  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	  FOREIGN KEY (telegram_id) REFERENCES users(telegram_id)
	);
  
	CREATE TABLE IF NOT EXISTS search_history (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  telegram_id INTEGER,
	  query TEXT,
	  results_count INTEGER,
	  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	  FOREIGN KEY (telegram_id) REFERENCES users(telegram_id)
	);
  
	CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
	CREATE INDEX IF NOT EXISTS idx_payments_telegram_id ON payments(telegram_id);
	`
	_, err := d.db.Exec(query)
	return err
}

func (d *Database) GetOrCreateUser(telegramID int64, username string) (*models.User, error) {
	user := &models.User{}

	err := d.db.QueryRow(`
	  SELECT telegram_id, username, subscribed_at, expires_at, is_active, search_count, last_search_date
	  FROM users WHERE telegram_id = ?
	`, telegramID).Scan(
		&user.TelegramID, &user.Username, &user.SubscribedAt,
		&user.ExpiresAt, &user.IsActive, &user.SearchCount, &user.LastSearchDate,
	)

	if err == sql.ErrNoRows {
		_, err = d.db.Exec(`
		INSERT INTO users (telegram_id, username, search_count, last_search_date)
		VALUES (?, ?, 0, ?)
	  `, telegramID, username, time.Now().Format("2006-01-02"))

		if err != nil {
			return nil, err
		}

		return &models.User{
			TelegramID:     telegramID,
			Username:       username,
			SearchCount:    0,
			LastSearchDate: time.Now(),
		}, nil
	}

	return user, err
}

func (d *Database) HasActiveSubscription(telegramID int64) (bool, error) {
	var expiresAt time.Time
	var isActive bool

	err := d.db.QueryRow(`
	  SELECT expires_at, is_active FROM users WHERE telegram_id = ?
	`, telegramID).Scan(&expiresAt, &isActive)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return isActive && time.Now().Before(expiresAt), nil
}

func (d *Database) GetTodaySearchCount(telegramID int64) (int, error) {
	var count int
	var lastDate string
	today := time.Now().Format("2006-01-02")

	err := d.db.QueryRow(`
	  SELECT search_count, last_search_date FROM users WHERE telegram_id = ?
	`, telegramID).Scan(&count, &lastDate)

	if err != nil {
		return 0, err
	}

	// Если последний поиск был не сегодня, сбрасываем счётчик
	if lastDate != today {
		_, _ = d.db.Exec(`
		UPDATE users SET search_count = 0, last_search_date = ? WHERE telegram_id = ?
	  `, today, telegramID)
		return 0, nil
	}

	return count, nil
}

func (d *Database) IncrementSearchCount(telegramID int64) error {
	today := time.Now().Format("2006-01-02")
	_, err := d.db.Exec(`
	  UPDATE users SET search_count = search_count + 1, last_search_date = ?
	  WHERE telegram_id = ?
	`, today, telegramID)
	return err
}

func (d *Database) ActivateSubscription(telegramID int64, days int) error {
	expiresAt := time.Now().AddDate(0, 0, days)
	_, err := d.db.Exec(`
	  UPDATE users SET is_active = 1, subscribed_at = ?, expires_at = ?
	  WHERE telegram_id = ?
	`, time.Now(), expiresAt, telegramID)
	return err
}

func (d *Database) SaveSearchHistory(telegramID int64, query string, resultsCount int) error {
	_, err := d.db.Exec(`
	  INSERT INTO search_history (telegram_id, query, results_count)
	  VALUES (?, ?, ?)
	`, telegramID, query, resultsCount)
	return err
}

func (d *Database) GetStats() (totalUsers int, activeSubscriptions int, totalSearches int, err error) {
	err = d.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&totalUsers)
	if err != nil {
		return
	}

	err = d.db.QueryRow(`
	  SELECT COUNT(*) FROM users WHERE is_active = 1 AND expires_at > datetime('now')
	`).Scan(&activeSubscriptions)
	if err != nil {
		return
	}

	err = d.db.QueryRow(`SELECT COUNT(*) FROM search_history`).Scan(&totalSearches)
	return
}

func (d *Database) Close() error {
	return d.db.Close()
}
