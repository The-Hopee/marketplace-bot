# MarketBot - Agent Guide

A Telegram bot for intelligent marketplace aggregation and product analysis across **Wildberries**, **Ozon**, and **Avito**.

## Quick Start

### Setup & Build
```bash
# Environment setup
cp .env.example .env  # Configure: TELEGRAM_TOKEN, DB credentials, API keys
docker-compose up  # Starts bot, PostgreSQL, Redis

# Local development
go build -o bot ./cmd/bot
./bot  # Requires .env file
```

**Key environment variables:**
- `TELEGRAM_TOKEN`: Telegram Bot API token
- `DATABASE_URL`: PostgreSQL connection (format: `postgres://user:pass@host/db?sslmode=disable`)
- `REDIS_URL`: Redis connection (optional, for caching)
- `PROXYAPI_KEY`: ProxyAPI key (for Gemini routing)
- `PROXYAPI_URL`: ProxyAPI endpoint (default: `https://api.proxyapi.ru/openai/v1`)
- `TBANK_*`: T-Bank payment integration
- `ADMIN_TELEGRAM_ID`: Admin user ID for admin commands
- `SUBSCRIPTION_PRICE`, `PRO_SUBSCRIPTION_PRICE`: In kopecks (cents)

### Database
```bash
# Migrations run automatically on startup
# Or manually: psql -U postgres -d marketplace_bot -f migrations/001_init.sql
```

## Architecture

### Core Components

**Bot Entry**: [cmd/bot/main.go](cmd/bot/main.go)
- Initializes config, database, cache, and bot
- Sets up graceful shutdown on SIGINT/SIGTERM

**Handlers**: [internal/bot/](internal/bot/)
- `handlers.go` (900+ lines): Main message/callback routing
  - Search flows (text/image)
  - Subscription & payment
  - Admin commands (broadcasts, promotions)
  - Referral program
- `bot.go`: Bot instance, payment webhook server
- `admin_handlers.go`: Admin-specific functionality
- `keyboard.go`: Telegram inline/reply keyboards

**Marketplace Aggregation**: [internal/marketplace/](internal/marketplace/)
- `aggregator.go`: Orchestrates parallel searches, respects subscription tiers
  - Free/Premium: Wildberries + Ozon
  - Pro: Adds Avito
- `interface.go`: Common marketplace interface
- `wildberries.go`, `ozon.go`, `avito.go`: Individual parsers (XML-based or browser automation)

**Services**: [internal/service/](internal/service/)
- `subscription.go`: Subscription state & search limits
- `refferal.go`: Referral bonuses & tracking
- `ad.go`, `broadcast.go`: Admin broadcast features

**AI Analysis**: [internal/analysis/](internal/analysis/)
- `ai-agent.go`: Calls Google Gemini 2.0 Flash (via ProxyAPI) for Pro tier analysis
  - Compares products across marketplaces with emotional and analytical insights
  - Highlights best deals with engaging narrative
  - Handles condition detection (New/Used) for Avito
  - Optimized for token efficiency and quick responses
- `anzlyzer.go` (inferred): Basic analysis for Free/Premium tiers

**Database**: [internal/database/](internal/database/)
- PostgreSQL repository pattern
- Models for users, payments, search history
- Admin models for promotions, referrals

**Cache**: [internal/cache/](internal/cache/)
- Redis caching of search results
- Popular searches tracking
- Configurable TTL

## User Flows

### Text Search
1. User clicks "🔍 Поиск товаров"
2. Bot collects query, checks subscription limits
3. Aggregator searches active marketplaces (based on tier)
4. Results cached, sent to user
5. Pro tier: AI analysis, Free/Premium: Basic analysis

### Image Search
1. User sends photo
2. Image recognized (likely Yandex API)
3. Similar products fetched from marketplaces
4. Free tier: Excludes Avito results
5. Pro tier: AI analysis of results

### Subscriptions
- **Free**: 5 searches/day, limited to WB+Ozon
- **Premium**: Unlimited searches, WB+Ozon+Avito, basic analysis
- **Pro**: All Premium + AI-powered analysis (GPT-4o-mini)
- Payment via T-Bank webhook

### Referral Program
- Users share `/start ref_{USER_ID}` links
- New users: +N days free Pro subscription
- Referrer bonus: +N days Pro for every 20 searches by referred user

## Key Patterns & Conventions

### Subscription Tier Check
```go
tier := user.GetTier()  // "free", "premium", "pro"
// Aggregator respects tier: only enables Avito for premium/pro
```

### User State Management
```go
h.userStates[userID] = "waiting_search"  // Track user interaction state
delete(h.userStates, userID)            // Clear when done
```

### Search Results Caching
```go
// Results cached by query string
h.cache.GetSearchResults(ctx, query, &results)
h.cache.SetSearchResults(ctx, query, results)
```

### Error Handling
- Non-blocking marketplace failures (one fails, others continue)
- Graceful fallbacks (AI error → basic analysis)
- User-friendly error messages

### Payment Integration
- T-Bank creates payment URL
- Webhook at `/webhook/payment` confirms payment
- Subscription automatically activated

## Common Tasks

### Add New Marketplace
1. Create [internal/marketplace/newmarket.go](internal/marketplace/)
2. Implement `Marketplace` interface
3. Register in `aggregator.NewAggregator()`
4. Update aggregator tier logic if restricted access needed

### Add Admin Feature
1. Add handler in [internal/bot/admin_handlers.go](internal/bot/admin_handlers.go)
2. Register command in [internal/bot/handlers.go](internal/bot/handlers.go#L91) admin check
3. Require `cfg.AdminTelegramID` validation

### Adjust Search Limits
- Free searches: [internal/subscription/service.go](internal/subscription/service.go)
- Cache TTL: Environment variable `CACHE_TTL_MINUTES`

### Test Marketplace Integration
- HTML debug samples in [debug_html/](debug_html/) (Avito example)
- Browser automation: go-rod configuration in marketplace files

## Development Tips

- **Logging**: Use `log.Printf()` with consistent prefixes (e.g., `[Handler]`, `[AIAgent]`)
- **Context propagation**: Pass `ctx` through all calls for cancellation support
- **Markdown in Telegram**: Set `msg.ParseMode = "Markdown"` for formatting
- **Rate limiting**: Consider per-user request throttling if needed
- **Testing**: Populate `.env` with test credentials before running
- **Database**: Migrations auto-run; add new migrations to [migrations/](migrations/) with sequential naming

## File Structure Reference
```
cmd/bot/main.go              ← Entry point
internal/
  ├─ bot/                    ← Telegram handlers (900+ lines handlers.go)
  ├─ marketplace/            ← Aggregator + marketplace parsers
  ├─ analysis/               ← AI agent + analyzer
  ├─ database/               ← PostgreSQL models & repos
  ├─ service/                ← Business logic (subscriptions, referrals)
  ├─ cache/                  ← Redis caching
  ├─ config/                 ← Environment config loader
  ├─ browser/                ← Browser automation setup
  ├─ payment/                ← T-Bank client
  └─ subscription/           ← Subscription limits & state
migrations/                  ← SQL schema files
docker-compose.yml          ← Services: bot, postgres, redis
Dockerfile                  ← Multi-stage build
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Bot not receiving updates | Check `TELEGRAM_TOKEN` validity & admin ID |
| Database connection fails | Verify `DATABASE_URL` format; ensure Postgres running |
| Redis optional error | Redis is optional; bot continues without cache |
| Searches return 0 results | Check if marketplace is up; examine logs for parser errors |
| Payment webhook not triggering | Ensure `WEBHOOK_URL` is publicly accessible & port 8080 exposed |
| AI analysis times out | `ai-agent.go` has error handling; falls back to basic analysis |

## Related Documentation
- [migrations/](migrations/) - Database schema
- [docker-compose.yml](docker-compose.yml) - Service configuration
- [go.mod](go.mod) - Dependencies (telegram-bot-api, pgx, go-rod, go-openai, redis)
