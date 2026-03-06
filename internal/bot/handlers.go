package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"unicode/utf8"

	"marketplace-bot/internal/analysis"
	"marketplace-bot/internal/cache"
	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	imagesearch "marketplace-bot/internal/imageSearch"
	"marketplace-bot/internal/marketplace"
	"marketplace-bot/internal/service"
	"marketplace-bot/internal/subscription"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Handler struct {
	bot           *tgbotapi.BotAPI
	repo          *database.Repository
	aggregator    *marketplace.Aggregator
	subService    *subscription.Service
	cache         *cache.RedisCache
	analyzer      *analysis.Analyzer
	imageSearcher *imagesearch.ImageSearcher
	cfg           *config.Config

	// Новые зависимости
	adminHandlers *AdminHandlers
	referralSvc   *service.ReferralService

	userStates   map[int64]string
	lastSearch   map[int64]string
	lastAnalysis map[int64]*analysis.AnalysisResult
}

func NewHandler(
	bot *tgbotapi.BotAPI,
	repo *database.Repository,
	aggregator *marketplace.Aggregator,
	subService *subscription.Service,
	cache *cache.RedisCache,
	cfg *config.Config,
	adminHandlers *AdminHandlers,
	referralSvc *service.ReferralService,
) *Handler {
	return &Handler{
		bot:           bot,
		repo:          repo,
		aggregator:    aggregator,
		subService:    subService,
		cache:         cache,
		analyzer:      analysis.NewAnalyzer(),
		imageSearcher: imagesearch.NewImageSearcher(),
		cfg:           cfg,
		adminHandlers: adminHandlers,
		referralSvc:   referralSvc,
		userStates:    make(map[int64]string),
		lastSearch:    make(map[int64]string),
		lastAnalysis:  make(map[int64]*analysis.AnalysisResult),
	}
}

func (h *Handler) HandleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		h.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		h.handleCallback(update.CallbackQuery)
	}
}

func (h *Handler) handleMessage(message *tgbotapi.Message) {
	ctx := context.Background()
	userID := message.From.ID

	_, err := h.repo.CreateUser(ctx, userID, message.From.UserName, message.From.FirstName, message.From.LastName)
	if err != nil {
		log.Printf("Error creating user: %v", err)
	}

	// ═══════ 1) Админ-команды (проверяем первыми) ═══════
	if h.adminHandlers.HandleAdminCommand(ctx, message) {
		return
	}

	// ═══════ 2) Фото ═══════
	if message.Photo != nil && len(message.Photo) > 0 {
		if state, ok := h.userStates[userID]; ok && state == "waiting_image" {
			h.handleImageSearch(ctx, message)
			return
		}
	}

	// ═══════ 3) Стейты ═══════
	if state, ok := h.userStates[userID]; ok {
		switch state {
		case "waiting_search":
			h.handleSearchQuery(ctx, message)
			return
		case "waiting_image":
			msg := tgbotapi.NewMessage(message.Chat.ID, "📷 Пожалуйста, отправьте фото товара")
			h.bot.Send(msg)
			return
		case "waiting_promo":
			h.applyPromo(ctx, message)
			return
		}
	}

	// ═══════ 4) Команды ═══════
	if message.IsCommand() {
		switch message.Command() {
		case "start":
			h.handleStart(ctx, message)
		case "help":
			h.handleHelp(message)
		case "promo":
			if args := message.CommandArguments(); args != "" {
				message.Text = args
				h.applyPromo(ctx, message)
			} else {
				h.handlePromoButton(message)
			}
		case "referral":
			h.handleReferral(ctx, message)
		default:
			h.handleHelp(message)
		}
		return
	}
	// ═══════ 5) Кнопки меню ═══════
	switch message.Text {
	case "🔍 Поиск товаров":
		h.handleSearchStart(message)
	case "📷 Поиск по фото":
		h.handleImageSearchStart(message)
	case "🔥 Популярные запросы":
		h.handlePopularSearches(ctx, message)
	case "💎 Подписка":
		h.handleSubscription(ctx, message)
	case "🎁 Промокод":
		h.handlePromoButton(message)
	case "👥 Рефералы":
		h.handleReferral(ctx, message)
	case "👤 Профиль":
		h.handleProfile(ctx, message)
	case "❓ Помощь":
		h.handleHelp(message)
	case "❌ Отмена":
		delete(h.userStates, userID)
		m := tgbotapi.NewMessage(message.Chat.ID, "Отменено")
		m.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(m)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "👆 Используйте кнопки меню")
		msg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(msg)
	}
}

// ==================== /start (с реферальной ссылкой) ====================

func (h *Handler) handleStart(ctx context.Context, message *tgbotapi.Message) {
	// Проверяем реферальную ссылку: /start ref_12345
	args := message.CommandArguments()
	if strings.HasPrefix(args, "ref_") {
		referrerIDStr := strings.TrimPrefix(args, "ref_")
		if referrerID, err := strconv.ParseInt(referrerIDStr, 10, 64); err == nil {
			if err := h.referralSvc.ProcessNewReferral(ctx, referrerID, message.From.ID); err != nil {
				log.Printf("Referral error: %v", err)
			} else {
				// Уведомляем реферера
				refMsg := tgbotapi.NewMessage(referrerID,
					fmt.Sprintf("🎉 По вашей ссылке зарегистрировался новый пользователь! +%d дней подписки!",
						service.ReferralBonusDays))
				h.bot.Send(refMsg)

				// Уведомляем приглашённого
				invMsg := tgbotapi.NewMessage(message.Chat.ID,
					fmt.Sprintf("🎁 Вы зарегистрировались по реферальной ссылке! +%d дней подписки!",
						service.ReferralBonusDays))
				h.bot.Send(invMsg)
			}
		}
	}

	text := fmt.Sprintf(`👋 Привет, %s!

🛒 Я бот для поиска товаров на Wildberries

📦 Что я умею:
• 🔍 Искать товары по названию
• 📷 Искать по фотографии
• 📊 Анализировать цены и скидки
• 🏆 Рекомендовать лучшие товары

🎁 У вас есть 5 бесплатных поисков!

Используйте кнопки меню 👇`, message.From.FirstName)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = MainMenuKeyboard()
	h.bot.Send(msg)
}

// ==================== Поиск ====================

func (h *Handler) handleSearchStart(message *tgbotapi.Message) {
	h.userStates[message.From.ID] = "waiting_search"
	msg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Введите название товара:")
	h.bot.Send(msg)
}

func (h *Handler) handleSearchQuery(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	query := strings.TrimSpace(message.Text)

	delete(h.userStates, userID)

	log.Printf("[Handler] Search query from user %d: %s", userID, query)

	if len(query) < 2 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Слишком короткий запрос")
		h.bot.Send(msg)
		return
	}

	canSearch, freeLeft, err := h.subService.CanUserSearch(ctx, userID)
	if err != nil {
		log.Printf("[Handler] Error: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка. Попробуйте позже.")
		h.bot.Send(msg)
		return
	}

	if !canSearch {
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"❌ Бесплатные поиски закончились.\n\n💎 Нажмите \"💎 Подписка\" или введите промокод 🎁")
		h.bot.Send(msg)
		return
	}

	searchMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Ищу товары...")
	sentMsg, _ := h.bot.Send(searchMsg)

	h.subService.UseSearch(ctx, userID)

	h.performSearch(ctx, message.Chat.ID, userID, query, sentMsg.MessageID)

	// Показываем оставшиеся поиски
	if freeLeft > 0 && freeLeft <= 2 {
		infoMsg := tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("⚠️ Осталось бесплатных поисков: %d", freeLeft-1))
		h.bot.Send(infoMsg)
	}
	// ═══════ Проверяем реферальный бонус за 20 поисков ═══════
	bonusGiven, referrerID, _ := h.referralSvc.CheckSearchBonus(ctx, userID)
	if bonusGiven {
		bonus := tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("🎉 Вы сделали %d поисков! +%d дней подписки по реферальной программе!",
				service.ReferralSearchTarget, service.ReferralBonusDays))
		h.bot.Send(bonus)

		if referrerID > 0 {
			refMsg := tgbotapi.NewMessage(referrerID,
				fmt.Sprintf("🎯 Ваш приглашённый сделал %d поисков! +%d дней подписки!",
					service.ReferralSearchTarget, service.ReferralBonusDays))
			h.bot.Send(refMsg)
		}
	}
}

func (h *Handler) performSearch(ctx context.Context, chatID int64, userID int64, query string, msgIDToDelete int) {
	var results *marketplace.AggregatedResult
	var fromCache bool

	if h.cache != nil {
		var cached marketplace.AggregatedResult
		found, err := h.cache.GetSearchResults(ctx, query, &cached)
		if err == nil && found {
			results = &cached
			fromCache = true
			log.Printf("[Handler] Results from cache for: %s", query)
		}
	}

	if results == nil {
		results = h.aggregator.Search(ctx, query, 10)

		if h.cache != nil && results.TotalCount > 0 {
			h.cache.SetSearchResults(ctx, query, results)
			h.cache.IncrementSearchCount(ctx, query)
		}
	}

	log.Printf("[Handler] Search completed. Total: %d, fromCache: %v", results.TotalCount, fromCache)

	h.repo.SaveSearchHistory(ctx, userID, query, results.TotalCount)
	h.lastSearch[userID] = query

	h.bot.Request(tgbotapi.NewDeleteMessage(chatID, msgIDToDelete))

	if results.TotalCount == 0 {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("😔 По запросу \"%s\" ничего не найдено", query))
		msg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(msg)
		return
	}

	analysisResult := h.analyzer.Analyze(results)
	h.lastAnalysis[userID] = analysisResult

	h.sendSearchResultsWithAnalysis(chatID, query, results, analysisResult, fromCache)
}

func (h *Handler) sendSearchResultsWithAnalysis(chatID int64, query string, results *marketplace.AggregatedResult, analysis *analysis.AnalysisResult, fromCache bool) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🔍 %s\n", sanitizeString(query)))

	if fromCache {
		sb.WriteString("⚡️ Быстрый результат\n")
	}
	sb.WriteString("\n")

	if analysis.BestOverall != nil {
		sb.WriteString("🏆 ЛУЧШИЙ ВЫБОР:\n")
		best := analysis.BestOverall
		name := truncateUTF8(sanitizeString(best.Name), 45)

		mpEmoji := "📦"
		if best.Marketplace == "OZON" {
			mpEmoji = "🔵"
		} else if best.Marketplace == "Wildberries" {
			mpEmoji = "🟣"
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", mpEmoji, name))
		sb.WriteString(fmt.Sprintf("💰 %.0f руб.", best.Price))
		if best.Discount > 0 {
			sb.WriteString(fmt.Sprintf(" (-%d%%)", best.Discount))
		}
		sb.WriteString(fmt.Sprintf(" — %s\n", best.Reason))
		sb.WriteString(fmt.Sprintf("%s\n\n", best.URL))
	}

	sb.WriteString("📊 НАЙДЕНО:\n")
	for mpName, products := range results.Results {
		mpEmoji := "📦"
		if mpName == "OZON" {
			mpEmoji = "🔵"
		} else if mpName == "Wildberries" {
			mpEmoji = "🟣"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %d товаров\n", mpEmoji, mpName, len(products)))
	}
	sb.WriteString("\n")

	sb.WriteString("💰 ЦЕНЫ:\n")
	sb.WriteString(fmt.Sprintf("• Мин: %.0f руб.\n", analysis.PriceStats.MinPrice))
	sb.WriteString(fmt.Sprintf("• Макс: %.0f руб.\n", analysis.PriceStats.MaxPrice))
	sb.WriteString(fmt.Sprintf("• Средняя: %.0f руб.\n", analysis.PriceStats.AvgPrice))
	if analysis.PriceStats.AvgDiscount > 0 {
		sb.WriteString(fmt.Sprintf("• Ср. скидка: %.0f%%\n", analysis.PriceStats.AvgDiscount))
	}
	sb.WriteString("\n")

	showCount := len(analysis.TopProducts)
	if showCount > 6 {
		showCount = 6
	}

	sb.WriteString(fmt.Sprintf("📦 ТОП-%d:\n\n", showCount))

	for i := 0; i < showCount; i++ {
		p := analysis.TopProducts[i]
		mpEmoji := "📦"
		if p.Marketplace == "OZON" {
			mpEmoji = "🔵"
		} else if p.Marketplace == "Wildberries" {
			mpEmoji = "🟣"
		}

		name := truncateUTF8(sanitizeString(p.Name), 38)
		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, mpEmoji, name))
		sb.WriteString(fmt.Sprintf("   💰 %.0f руб.", p.Price))
		if p.Discount > 0 {
			sb.WriteString(fmt.Sprintf(" -%d%%", p.Discount))
		}
		sb.WriteString(fmt.Sprintf(" (скор: %.0f)\n", p.Score))
		sb.WriteString(fmt.Sprintf("   %s\n\n", p.URL))
	}

	text := sb.String()
	if !utf8.ValidString(text) {
		text = sanitizeString(text)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = MainMenuKeyboard()

	_, err := h.bot.Send(msg)
	if err != nil {
		log.Printf("[Handler] Error sending: %v", err)
		simpleMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("🔍 Найдено %d товаров", results.TotalCount))
		simpleMsg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(simpleMsg)
	}
}

// ==================== Поиск по фото ====================

func (h *Handler) handleImageSearchStart(message *tgbotapi.Message) {
	h.userStates[message.From.ID] = "waiting_image"
	msg := tgbotapi.NewMessage(message.Chat.ID, `📷 Отправьте фото товара

Я распознаю товар и найду его на Wildberries.

💡 Советы:
• Фото должно быть чётким
• Товар должен быть хорошо виден
• Лучше фотографировать на светлом фоне`)
	h.bot.Send(msg)
}

func (h *Handler) handleImageSearch(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	delete(h.userStates, userID)

	canSearch, _, err := h.subService.CanUserSearch(ctx, userID)
	if err != nil || !canSearch {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ У вас закончились бесплатные поиски. Оформите подписку.")
		h.bot.Send(msg)
		return
	}

	searchMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Ищу товар по изображению...")
	sentMsg, _ := h.bot.Send(searchMsg)

	photo := message.Photo[len(message.Photo)-1]
	file, err := h.bot.GetFile(tgbotapi.FileConfig{FileID: photo.FileID})
	if err != nil {
		log.Printf("[Handler] Error getting file: %v", err)
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось загрузить фото")
		h.bot.Send(msg)
		return
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)

	imageResult, err := h.imageSearcher.SearchByImageURL(ctx, fileURL)
	if err != nil {
		log.Printf("[Handler] Image search error: %v", err)
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))
		h.offerManualSearch(message.Chat.ID, userID)
		return
	}

	if !imageResult.Success || len(imageResult.Products) == 0 {
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))
		h.offerManualSearch(message.Chat.ID, userID)
		return
	}

	h.subService.UseSearch(ctx, userID)

	h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))

	aggregatedResult := &marketplace.AggregatedResult{
		Query: imageResult.Query,
		Results: map[string][]marketplace.Product{
			"Wildberries": imageResult.Products,
		},
		TotalCount: len(imageResult.Products),
	}

	analysisResult := h.analyzer.Analyze(aggregatedResult)

	h.sendImageSearchResultsWithAnalysis(message.Chat.ID, imageResult, analysisResult)

	// ═══════ Проверяем реферальный бонус ═══════
	bonusGiven, referrerID, _ := h.referralSvc.CheckSearchBonus(ctx, userID)
	if bonusGiven {
		bonus := tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("🎉 Вы сделали %d поисков! +%d дней подписки по реферальной программе!",
				service.ReferralSearchTarget, service.ReferralBonusDays))
		h.bot.Send(bonus)

		if referrerID > 0 {
			refMsg := tgbotapi.NewMessage(referrerID,
				fmt.Sprintf("🎯 Ваш приглашённый сделал %d поисков! +%d дней подписки!",
					service.ReferralSearchTarget, service.ReferralBonusDays))
			h.bot.Send(refMsg)
		}
	}
}
func (h *Handler) sendImageSearchResultsWithAnalysis(chatID int64, result *imagesearch.ImageSearchResult, analysis *analysis.AnalysisResult) {
	var sb strings.Builder

	sb.WriteString("📷 Найдено по изображению\n\n")

	wbCount := 0
	ozonCount := 0
	for _, p := range result.Products {
		if p.Marketplace == "OZON" {
			ozonCount++
		} else {
			wbCount++
		}
	}

	sb.WriteString("📊 НАЙДЕНО:\n")
	if wbCount > 0 {
		sb.WriteString(fmt.Sprintf("🟣 Wildberries: %d\n", wbCount))
	}
	if ozonCount > 0 {
		sb.WriteString(fmt.Sprintf("🔵 OZON: %d\n", ozonCount))
	}
	sb.WriteString("\n")

	if analysis.BestOverall != nil {
		sb.WriteString("🏆 ЛУЧШИЙ ВЫБОР:\n")
		best := analysis.BestOverall

		mpEmoji := "📦"
		if best.Marketplace == "OZON" {
			mpEmoji = "🔵"
		} else if best.Marketplace == "Wildberries" {
			mpEmoji = "🟣"
		}

		name := truncateUTF8(sanitizeString(best.Name), 45)
		sb.WriteString(fmt.Sprintf("%s %s\n", mpEmoji, name))
		sb.WriteString(fmt.Sprintf("💰 %.0f руб.", best.Price))
		if best.Discount > 0 {
			sb.WriteString(fmt.Sprintf(" (-%d%%)", best.Discount))
		}
		sb.WriteString(fmt.Sprintf(" — %s\n", best.Reason))
		sb.WriteString(fmt.Sprintf("%s\n\n", best.URL))
	}

	sb.WriteString("💰 ЦЕНЫ:\n")
	sb.WriteString(fmt.Sprintf("• Мин: %.0f руб.\n", analysis.PriceStats.MinPrice))
	sb.WriteString(fmt.Sprintf("• Средняя: %.0f руб.\n", analysis.PriceStats.AvgPrice))
	sb.WriteString("\n")

	showCount := len(analysis.TopProducts)
	if showCount > 5 {
		showCount = 5
	}

	sb.WriteString(fmt.Sprintf("📦 ТОП-%d:\n\n", showCount))

	for i := 0; i < showCount; i++ {
		p := analysis.TopProducts[i]

		mpEmoji := "📦"
		if p.Marketplace == "OZON" {
			mpEmoji = "🔵"
		} else if p.Marketplace == "Wildberries" {
			mpEmoji = "🟣"
		}

		name := truncateUTF8(sanitizeString(p.Name), 38)
		sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, mpEmoji, name))
		sb.WriteString(fmt.Sprintf("   💰 %.0f руб.", p.Price))
		if p.Discount > 0 {
			sb.WriteString(fmt.Sprintf(" -%d%%", p.Discount))
		}
		sb.WriteString(fmt.Sprintf(" (скор: %.0f)\n", p.Score))
		sb.WriteString(fmt.Sprintf("   %s\n\n", p.URL))
	}

	remaining := len(result.Products) - showCount
	if remaining > 0 {
		sb.WriteString(fmt.Sprintf("...и ещё %d товаров\n", remaining))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = MainMenuKeyboard()

	_, err := h.bot.Send(msg)
	if err != nil {
		log.Printf("[Handler] Error sending: %v", err)
	}
}

func (h *Handler) offerManualSearch(chatID int64, userID int64) {
	h.userStates[userID] = "waiting_search"
	msg := tgbotapi.NewMessage(chatID, `😔 Не удалось найти товар по фото.
  
  📝 Попробуйте ввести название товара вручную:`)
	h.bot.Send(msg)
}

// ==================== Популярные запросы ====================

func (h *Handler) handlePopularSearches(ctx context.Context, message *tgbotapi.Message) {
	if h.cache == nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "📊 Пока нет популярных запросов")
		h.bot.Send(msg)
		return
	}

	popular, err := h.cache.GetPopularSearches(ctx, 10)
	if err != nil || len(popular) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "📊 Пока нет популярных запросов")
		h.bot.Send(msg)
		return
	}

	var sb strings.Builder
	sb.WriteString("🔥 Популярные запросы:\n\n")
	for i, query := range popular {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, query))
	}
	sb.WriteString("\nНажмите \"🔍 Поиск товаров\" чтобы найти")

	msg := tgbotapi.NewMessage(message.Chat.ID, sb.String())
	msg.ReplyMarkup = MainMenuKeyboard()
	h.bot.Send(msg)
}

// ==================== 💎 Подписка ====================

func (h *Handler) handleSubscription(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		log.Printf("ERROR [subscription] GetUser telegram_id=%d: %v", userID, err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка")
		h.bot.Send(msg)
		return
	}

	if user.HasActiveSubscription() {
		var subInfo string
		if user.SubscriptionEnd != nil {
			subInfo = fmt.Sprintf("📅 Активна до: %s", user.SubscriptionEnd.Format("02.01.2006"))
		} else {
			subInfo = "♾️ Навсегда"
		}

		text := fmt.Sprintf(`💎 *Ваша подписка*
  
  ✅ Статус: *Активна*
  %s
  
  📊 *Статистика:*
  🔍 Поисков выполнено: %d
  🆓 Лимит: безлимитный`, subInfo, user.SearchCount)

		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(msg)
		return
	}

	// Нет подписки — предлагаем оплату
	paymentInfo, err := h.subService.CreateSubscriptionPayment(ctx, userID, message.From.UserName)
	if err != nil {
		log.Printf("ERROR [subscription] CreatePayment telegram_id=%d: %v", userID, err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка создания платежа")
		h.bot.Send(msg)
		return
	}

	text := fmt.Sprintf(`💎 *Подписка MarketBot*
  
  ❌ Статус: *Не активна*
  🆓 Бесплатных поисков: %d
  
  💰 %.0f руб/месяц
  
  ✅ Безлимитный поиск
  ✅ Поиск по фото
  ✅ Анализ цен
  ✅ Рекомендации
  
  Нажмите кнопку для оплаты 👇`, user.FreeSearchesLeft, float64(paymentInfo.Amount)/100)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = SubscriptionKeyboard(paymentInfo.PaymentURL)
	h.bot.Send(msg)
}

// ==================== 🎁 Промокод ====================

func (h *Handler) handlePromoButton(message *tgbotapi.Message) {
	h.userStates[message.From.ID] = "waiting_promo"
	m := tgbotapi.NewMessage(message.Chat.ID, "🎁 Введите промокод:")
	m.ReplyMarkup = CancelKeyboard()
	h.bot.Send(m)
}

func (h *Handler) applyPromo(ctx context.Context, message *tgbotapi.Message) {
	delete(h.userStates, message.From.ID)

	if message.Text == "❌ Отмена" {
		m := tgbotapi.NewMessage(message.Chat.ID, "Отменено")
		m.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(m)
		return
	}

	code := strings.ToUpper(strings.TrimSpace(message.Text))

	promo, err := h.repo.GetPromocodeByCode(ctx, code)
	if err != nil {
		m := tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод не найден")
		m.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(m)
		return
	}

	if !promo.IsActive {
		m := tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод неактивен")
		m.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(m)
		return
	}

	if promo.MaxUses != nil && promo.UsedCount >= *promo.MaxUses {
		m := tgbotapi.NewMessage(message.Chat.ID, "❌ Промокод исчерпан")
		m.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(m)
		return
	}

	used, _ := h.repo.HasUsedPromo(ctx, message.From.ID, code)
	if used {
		m := tgbotapi.NewMessage(message.Chat.ID, "❌ Вы уже использовали этот промокод")
		m.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(m)
		return
	}

	// Активируем
	if err := h.repo.ExtendSubscription(ctx, message.From.ID, promo.FreeDays); err != nil {
		log.Printf("ERROR extend sub promo %d: %v", message.From.ID, err)
		h.bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка"))
		return
	}
	_ = h.repo.RecordPromoUsage(ctx, message.From.ID, code)
	_ = h.repo.IncrementPromoUsage(ctx, code)

	text := fmt.Sprintf("🎉 Промокод %s активирован!\n\n+%d дней подписки!", code, promo.FreeDays)
	m := tgbotapi.NewMessage(message.Chat.ID, text)
	m.ParseMode = "Markdown"
	m.ReplyMarkup = MainMenuKeyboard()
	h.bot.Send(m)
}

// ==================== 👥 Рефералы ====================

func (h *Handler) handleReferral(ctx context.Context, message *tgbotapi.Message) {
	canUse, daysLeft, err := h.referralSvc.CanUseReferral(ctx, message.From.ID)
	if err != nil {
		h.bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка"))
		return
	}

	if daysLeft > 0 {
		text := fmt.Sprintf("⏳ Реферальная программа откроется через %d дн.\n\n"+
			"Пользуйтесь ботом, и скоро вы сможете приглашать друзей!", daysLeft)
		h.bot.Send(tgbotapi.NewMessage(message.Chat.ID, text))
		return
	}

	link := h.referralSvc.GetReferralLink(message.From.ID)
	total, searchBonuses, _ := h.referralSvc.GetStats(ctx, message.From.ID)

	slotsLeft := service.ReferralMaxInvites - total
	if slotsLeft < 0 {
		slotsLeft = 0
	}

	limitText := ""
	if !canUse && daysLeft == 0 {
		limitText = "\n\n⚠️ Лимит приглашений исчерпан"
	}
	text := fmt.Sprintf(`👥 *Реферальная программа*

🔗 Ваша ссылка:
%s

📊 *Статистика:*
👤 Приглашено: %d/%d
🎯 Активных (20+ поисков): %d
📭 Осталось слотов: %d

💎 *Бонусы:*
• +%d дней вам и другу за регистрацию
• +%d дней вам и другу за 20 поисков%s`,
		link, total, service.ReferralMaxInvites, searchBonuses, slotsLeft,
		service.ReferralBonusDays, service.ReferralBonusDays, limitText)

	m := tgbotapi.NewMessage(message.Chat.ID, text)
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

// ==================== 👤 Профиль ====================

func (h *Handler) handleProfile(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка")
		h.bot.Send(msg)
		return
	}

	var subStatus string
	if user.HasActiveSubscription() {
		subStatus = fmt.Sprintf("✅ До %s", user.SubscriptionEnd.Format("02.01.2006"))
	} else {
		subStatus = fmt.Sprintf("❌ Нет (осталось %d поисков)", user.FreeSearchesLeft)
	}

	refTotal, refSearch, _ := h.referralSvc.GetStats(ctx, userID)

	text := fmt.Sprintf(`👤 Профиль

Имя: %s %s
Поисков: %d
Подписка: %s
Приглашено друзей: %d/%d
Активных (20+ поисков): %d`,
		user.FirstName, user.LastName,
		user.SearchCount,
		subStatus,
		refTotal, service.ReferralMaxInvites,
		refSearch,
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	h.bot.Send(msg)
}

// ==================== ❓ Помощь ====================

func (h *Handler) handleHelp(message *tgbotapi.Message) {
	text := `❓ Помощь

🔍 Поиск по тексту — введите название
📷 Поиск по фото — отправьте фото товара
📊 Анализ — автоматически для каждого поиска
🏆 Рекомендации — лучший товар по скору

🎁 Промокод — введите для бонусов
👥 Рефералы — приглашайте друзей (от 3 дня)
   • +7 дней за регистрацию друга
   • +7 дней когда друг сделает 20 поисков

💎 Подписка — безлимитный поиск`

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	h.bot.Send(msg)
}

// ==================== Callback ====================

func (h *Handler) handleCallback(callback *tgbotapi.CallbackQuery) {
	ctx := context.Background()

	switch callback.Data {
	case "check_payment":
		h.handleCheckPayment(ctx, callback)
	case "new_search":
		h.userStates[callback.From.ID] = "waiting_search"
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "🔍 Введите название товара:")
		h.bot.Send(msg)
	case "back_to_menu":
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "📱 Меню")
		msg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(msg)
	}

	h.bot.Request(tgbotapi.NewCallback(callback.ID, ""))
}

func (h *Handler) handleCheckPayment(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	user, _ := h.repo.GetUserByTelegramID(ctx, callback.From.ID)
	if user != nil && user.HasActiveSubscription() {
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "✅ Подписка активирована!")
		msg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(msg)
	} else {
		h.bot.Request(tgbotapi.NewCallback(callback.ID, "⏳ Оплата не получена"))
	}
}

// ==================== Утилиты ====================

func truncateUTF8(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func sanitizeString(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	v := make([]rune, 0, len(s))
	for _, r := range s {
		if r != utf8.RuneError {
			v = append(v, r)
		}
	}
	return string(v)
}
