package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"The-Hopee/marketplace-bot/internal/config"
	"The-Hopee/marketplace-bot/internal/database"
	"The-Hopee/marketplace-bot/internal/models"
	"The-Hopee/marketplace-bot/internal/parser"
	"The-Hopee/marketplace-bot/internal/ratelimiter"
)

type Bot struct {
	api         *tgbotapi.BotAPI
	cfg         *config.Config
	db          *database.Database
	parsers     []parser.Parser
	rateLimiter *ratelimiter.PerMarketplaceLimiter
}

func New(cfg *config.Config, db *database.Database) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("создание бота: %w", err)
	}

	api.Debug = cfg.Debug

	parsers := []parser.Parser{
		parser.NewWildberriesParser(cfg.ProxyURL),
		parser.NewOzonParser(cfg.ProxyURL),
		parser.NewYandexParser(cfg.ProxyURL),
	}

	return &Bot{
		api:         api,
		cfg:         cfg,
		db:          db,
		parsers:     parsers,
		rateLimiter: ratelimiter.New(),
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	log.Printf("Бот @%s запущен", b.api.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			go b.handleUpdate(update)
		}
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	user, err := b.db.GetOrCreateUser(msg.From.ID, msg.From.UserName)
	if err != nil {
		log.Printf("Ошибка получения пользователя: %v", err)
		return
	}

	switch msg.Command() {
	case "start":
		b.handleStart(msg, user)
	case "help":
		b.handleHelp(msg)
	case "search":
		b.handleSearch(msg, user)
	case "subscribe":
		b.handleSubscribe(msg, user)
	case "status":
		b.handleStatus(msg, user)
	case "admin":
		b.handleAdmin(msg)
	default:
		if msg.Text != "" && !msg.IsCommand() {
			b.performSearch(msg, user, msg.Text)
		}
	}
}

func (b *Bot) handleStart(msg *tgbotapi.Message, user *models.User) {
	text := fmt.Sprintf(`👋 Привет, %s!
  
  🔍 Я бот для поиска товаров на маркетплейсах:
  • Wildberries
  • Ozon  
  • Яндекс Маркет
  
  📝 Просто отправь мне название товара, и я найду лучшие предложения!
  
  🆓 Бесплатно: %d поисков в день
  💎 Подписка: безлимитный поиск за %d₽/месяц
  
  Команды:
  /search [запрос] - поиск товаров
  /subscribe - оформить подписку
  /status - статус подписки
  /help - помощь`,
		msg.From.FirstName,
		b.cfg.FreeSearchLimit,
		b.cfg.SubscriptionPrice,
	)

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	b.api.Send(reply)
}

func (b *Bot) handleHelp(msg *tgbotapi.Message) {
	text := `📖 Как пользоваться ботом:
  
  1️⃣ Просто напишите название товара
	 Пример: iPhone 15 Pro Max
  
  2️⃣ Или используйте команду /search
	 Пример: /search наушники Sony
  
  📊 Результаты включают:
  • Название и цену
  • Рейтинг и отзывы
  • Ссылку на товар
  
  💡 Советы:
  • Пишите конкретные запросы для лучших результатов
  • Указывайте бренд, если ищете конкретный товар
  • Проверяйте актуальность цен на сайте`

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	b.api.Send(reply)
}

func (b *Bot) handleSearch(msg *tgbotapi.Message, user *models.User) {
	query := msg.CommandArguments()
	if query == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Укажите запрос для поиска\n\nПример: /search iPhone 15")
		b.api.Send(reply)
		return
	}

	b.performSearch(msg, user, query)
}

func (b *Bot) performSearch(msg *tgbotapi.Message, user *models.User, query string) {
	// Проверяем лимиты
	hasSubscription, _ := b.db.HasActiveSubscription(user.TelegramID)

	if !hasSubscription {
		searchCount, _ := b.db.GetTodaySearchCount(user.TelegramID)
		if searchCount >= b.cfg.FreeSearchLimit {
			text := fmt.Sprintf(`⚠️ Вы исчерпали лимит бесплатных поисков на сегодня (%d/%d)
		💎 Оформите подписку за %d₽/месяц для безлимитного поиска!

/subscribe - оформить подписку`,
				searchCount, b.cfg.FreeSearchLimit, b.cfg.SubscriptionPrice)
			reply := tgbotapi.NewMessage(msg.Chat.ID, text)
			b.api.Send(reply)
			return
		}
	}

	// Отправляем сообщение о начале поиска
	waitMsg := tgbotapi.NewMessage(msg.Chat.ID, "🔍 Ищу товары на маркетплейсах...")
	sentMsg, _ := b.api.Send(waitMsg)

	// Выполняем поиск параллельно
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := b.searchAllMarketplaces(ctx, query, 5)

	// Удаляем сообщение о поиске
	deleteMsg := tgbotapi.NewDeleteMessage(msg.Chat.ID, sentMsg.MessageID)
	b.api.Request(deleteMsg)

	// Формируем ответ
	b.sendSearchResults(msg.Chat.ID, query, results)

	// Сохраняем статистику
	totalResults := 0
	for _, r := range results {
		totalResults += len(r.Products)
	}
	b.db.IncrementSearchCount(user.TelegramID)
	b.db.SaveSearchHistory(user.TelegramID, query, totalResults)
}

func (b *Bot) searchAllMarketplaces(ctx context.Context, query string, limit int) []models.SearchResult {
	results := make([]models.SearchResult, len(b.parsers))
	var wg sync.WaitGroup

	for i, p := range b.parsers {
		wg.Add(1)
		go func(idx int, parser parser.Parser) {
			defer wg.Done()

			// Rate limiting
			marketplaceName := parser.Name()
			b.rateLimiter.Wait(ctx, marketplaceName)

			products, err := parser.Search(ctx, query, limit)
			results[idx] = models.SearchResult{
				Query:    query,
				Products: products,
				Error:    err,
			}
		}(i, p)
	}

	wg.Wait()
	return results
}

func (b *Bot) sendSearchResults(chatID int64, query string, results []models.SearchResult) {
	var allProducts []models.Product
	var errors []string

	for _, r := range results {
		if r.Error != nil {
			errors = append(errors, r.Error.Error())
			continue
		}
		allProducts = append(allProducts, r.Products...)
	}

	if len(allProducts) == 0 {
		text := fmt.Sprintf("😔 По запросу «%s» ничего не найдено", query)
		if len(errors) > 0 {
			text += "\n\n⚠️ Некоторые маркетплейсы недоступны"
		}
		reply := tgbotapi.NewMessage(chatID, text)
		b.api.Send(reply)
		return
	}

	text := fmt.Sprintf("🔍 Результаты поиска: *%s*\n\n", escapeMarkdown(query))

	// Группируем по маркетплейсам
	byMarketplace := make(map[string][]models.Product)
	for _, p := range allProducts {
		byMarketplace[p.Marketplace] = append(byMarketplace[p.Marketplace], p)
	}

	for marketplace, products := range byMarketplace {
		emoji := getMarketplaceEmoji(marketplace)
		text += fmt.Sprintf("%s *%s*\n", emoji, marketplace)

		for i, p := range products {
			if i >= 3 { // Показываем только топ-3 с каждого маркетплейса
				break
			}

			priceStr := formatPrice(p.Price)
			if p.OldPrice > p.Price && p.OldPrice > 0 {
				discount := int((1 - p.Price/p.OldPrice) * 100)
				priceStr += fmt.Sprintf(" (−%d%%)", discount)
			}

			ratingStr := ""
			if p.Rating > 0 {
				ratingStr = fmt.Sprintf(" ⭐️%.1f", p.Rating)
				if p.ReviewCount > 0 {
					ratingStr += fmt.Sprintf(" (%d)", p.ReviewCount)
				}
			}

			name := truncate(p.Name, 50)
			text += fmt.Sprintf("• [%s](%s)\n  💰 %s%s\n",
				escapeMarkdown(name), p.URL, priceStr, ratingStr)
		}
		text += "\n"
	}

	if len(errors) > 0 {
		text += "⚠️ _Некоторые маркетплейсы временно недоступны_"
	}

	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = "Markdown"
	reply.DisableWebPagePreview = true
	b.api.Send(reply)
}

func (b *Bot) handleSubscribe(msg *tgbotapi.Message, user *models.User) {
	hasSubscription, _ := b.db.HasActiveSubscription(user.TelegramID)

	if hasSubscription {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "✅ У вас уже есть активная подписка!")
		b.api.Send(reply)
		return
	}

	text := fmt.Sprintf(`💎 *Подписка на бота*

Стоимость: %d₽/месяц
Что включено:
✅ Безлимитный поиск
✅ Приоритетная обработка
✅ Поддержка 24/7

Для оплаты переведите %d₽ по реквизитам:
💳 Карта: [НОМЕР_КАРТЫ]

После оплаты отправьте скриншот администратору или напишите /paid`,
		b.cfg.SubscriptionPrice, b.cfg.SubscriptionPrice)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonCallback("✅ Я оплатил", "paid"),
		),
	)

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = keyboard
	b.api.Send(reply)
}

func (b *Bot) handleStatus(msg *tgbotapi.Message, user *models.User) {
	hasSubscription, _ := b.db.HasActiveSubscription(user.TelegramID)
	searchCount, _ := b.db.GetTodaySearchCount(user.TelegramID)

	var text string
	if hasSubscription {
		text = fmt.Sprintf(`📊 *Ваш статус*

💎 Подписка: Активна
📅 Действует до: %s
🔍 Поисков сегодня: %d (безлимит)`,
			user.ExpiresAt.Format("02.01.2006"),
			searchCount)
	} else {
		text = fmt.Sprintf(`📊 *Ваш статус*

🆓 Тариф: Бесплатный
🔍 Поисков сегодня: %d/%d

💎 Хотите безлимит? /subscribe`,
			searchCount, b.cfg.FreeSearchLimit)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

func (b *Bot) handleAdmin(msg *tgbotapi.Message) {
	if msg.From.ID != b.cfg.AdminID {
		return
	}

	totalUsers, activeSubscriptions, totalSearches, err := b.db.GetStats()
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ Ошибка получения статистики")
		b.api.Send(reply)
		return
	}

	text := fmt.Sprintf(`📊 *Статистика бота*

👥 Всего пользователей: %d
💎 Активных подписок: %d
🔍 Всего поисков: %d

💰 Потенциальный доход: %d₽`,
		totalUsers, activeSubscriptions, totalSearches,
		activeSubscriptions*b.cfg.SubscriptionPrice)

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "Markdown"
	b.api.Send(reply)
}

func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	switch callback.Data {
	case "paid":
		// Уведомляем админа
		if b.cfg.AdminID != 0 {
			text := fmt.Sprintf("💰 Новая оплата от @%s (ID: %d)\n\nПроверьте и активируйте подписку командой:\n/activate %d",
				callback.From.UserName, callback.From.ID, callback.From.ID)
			adminMsg := tgbotapi.NewMessage(b.cfg.AdminID, text)
			b.api.Send(adminMsg)
		}

		answerText := "✅ Заявка отправлена! Подписка будет активирована после проверки оплаты."
		answer := tgbotapi.NewCallback(callback.ID, answerText)
		b.api.Request(answer)

		reply := tgbotapi.NewMessage(callback.Message.Chat.ID, answerText)
		b.api.Send(reply)
	}
}

// Вспомогательные функции
func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"", "\\",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

func formatPrice(price float64) string {
	if price >= 1000 {
		return fmt.Sprintf("%.0f ₽", price)
	}
	return fmt.Sprintf("%.2f ₽", price)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getMarketplaceEmoji(marketplace string) string {
	switch marketplace {
	case "Wildberries":
		return "🟣"
	case "Ozon":
		return "🔵"
	case "Yandex Market":
		return "🟡"
	default:
		return "📦"
	}
}
