-- ==================== Админы ====================
CREATE TABLE IF NOT EXISTS admins (
    id SERIAL PRIMARY KEY,
    telegram_id BIGINT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==================== Реклама ====================
CREATE TABLE IF NOT EXISTS ads (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    text TEXT NOT NULL,
    button_text VARCHAR(255),
    button_url VARCHAR(500),
    is_active BOOLEAN DEFAULT true,
    priority INT DEFAULT 1,
    views_count INT DEFAULT 0,
    clicks_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==================== Рассылки ====================
CREATE TABLE IF NOT EXISTS broadcasts (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    text TEXT NOT NULL,
    button_text VARCHAR(255),
    button_url VARCHAR(500),
    status VARCHAR(20) DEFAULT 'draft',
    total_users INT DEFAULT 0,
    sent_count INT DEFAULT 0,
    failed_count INT DEFAULT 0,
    last_user_id BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==================== Промокоды ====================
CREATE TABLE IF NOT EXISTS promocodes (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    free_days INT DEFAULT 0,
    max_uses INT,
    used_count INT DEFAULT 0,
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

-- Промокод FIRST: 30 дней бесплатно, лимит 100 использований
INSERT INTO promocodes (code, free_days, max_uses, is_active)
VALUES ('FIRST', 30, 100, true)
ON CONFLICT (code) DO NOTHING;

-- ==================== Рефералы ====================
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

-- Добавляем поле referred_by к users (если ещё нет)
ALTER TABLE users ADD COLUMN IF NOT EXISTS referred_by BIGINT;