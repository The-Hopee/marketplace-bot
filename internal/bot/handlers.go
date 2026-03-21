// internal/bot/handlers.go
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
	aiAgent       *analysis.AIAgent

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
	aiAgent *analysis.AIAgent,
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
		aiAgent:       aiAgent,
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

	// ═══════ 2) Фото или Документ-картинка ═══════
	isImage := false
	if message.Photo != nil && len(message.Photo) > 0 {
		isImage = true
	} else if message.Document != nil && strings.HasPrefix(message.Document.MimeType, "image/") {
		isImage = true
	}

	if isImage {
		if state, ok := h.userStates[userID]; ok && state == "waiting_image" {
			h.handleImageSearch(ctx, message)
			return
		} else {
			// Если юзер кинул фотку просто так (без нажатия кнопки), подскажем ему
			msg := tgbotapi.NewMessage(message.Chat.ID, "👆 Чтобы искать по фото, сначала нажмите кнопку «📷 Поиск по фото» в меню.")
			h.bot.Send(msg)
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
		case "waiting_city":
			city := strings.TrimSpace(message.Text)
			h.repo.UpdateUserCity(ctx, userID, city)
			delete(h.userStates, userID)
			msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ваш город установлен: *%s*\nТеперь Avito будет искать товары рядом с вами!", city))
			msg.ParseMode = "Markdown"
			h.bot.Send(msg)
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

	// Новый красивый текст приветствия
	text := fmt.Sprintf(`👋 Привет, %s!

🛒 Я бот для умного поиска товаров на *Wildberries*, *Ozon* и *Avito*.

📦 Что я умею:
• 🔍 Искать товары по названию
• 📷 Искать похожие товары по фотографии
• 📊 Сравнивать цены и находить максимальные скидки
• 🤖 Выбирать лучшее с помощью AI-Агента

🎁 *У вас есть 5 бесплатных поисков каждый день!*
_(Лимиты сбрасываются каждую ночь)_

Используйте кнопки меню 👇`, message.From.FirstName)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown" // Включили Markdown для выделения жирным и курсивом
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

	can, left, err := h.subService.CanUserSearch(ctx, userID, subscription.SearchTypeWBText)
	if err != nil {
		log.Printf("[Handler] Error: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка. Попробуйте позже.")
		h.bot.Send(msg)
		return
	}
	if !can {
		h.bot.Send(tgbotapi.NewMessage(message.Chat.ID,
			"❌ Лимит 5 бесплатных поисков на сегодня исчерпан.\n\nОформите подписку 💎 или подождите до завтра."))
		return
	}

	if left > 0 && left <= 2 {
		h.bot.Send(tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("🆓 Осталось бесплатных поисков на сегодня: %d", left)))
	}

	searchMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Ищу товары...")
	sentMsg, _ := h.bot.Send(searchMsg)

	h.subService.UseSearch(ctx, userID, subscription.SearchTypeWBText)

	h.performSearch(ctx, message.Chat.ID, userID, query, sentMsg.MessageID)

	// Показываем оставшиеся поиски
	if left > 0 && left <= 5 {
		infoMsg := tgbotapi.NewMessage(message.Chat.ID,
			fmt.Sprintf("⚠️ Осталось бесплатных поисков: %d", left-1))
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
	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	tier := "free"
	userCity := "" // По умолчанию пусто
	if err == nil && user != nil {
		tier = user.GetTier()
		userCity = user.City // Берем город юзера!
	}

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
		// ПЕРЕДАЕМ TIER В АГРЕГАТОР (запускает нужные маркетплейсы)
		results = h.aggregator.Search(ctx, query, 10, tier, userCity)

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
		msg.ReplyMarkup = MainMenuKeyboard() // Убедись, что функция клавиатуры импортирована
		h.bot.Send(msg)
		return
	}

	// ==========================================
	// 🚀 МАГИЯ PRO-ПОДПИСКИ (AI-АГЕНТ)
	// ==========================================
	if tier == "pro" {
		aiWaitMsg := tgbotapi.NewMessage(chatID, "🤖 *AI-Агент* анализирует товары, сравнивает цены и отзывы. Подождите пару секунд...")
		aiWaitMsg.ParseMode = "Markdown"
		sentWait, _ := h.bot.Send(aiWaitMsg)

		aiResponse, err := h.aiAgent.Analyze(ctx, results)

		h.bot.Request(tgbotapi.NewDeleteMessage(chatID, sentWait.MessageID))

		if err == nil && aiResponse != "" {
			msg := tgbotapi.NewMessage(chatID, aiResponse)
			msg.ParseMode = "Markdown" // GPT возвращает красивый Markdown
			msg.DisableWebPagePreview = true
			msg.ReplyMarkup = MainMenuKeyboard()
			h.bot.Send(msg)

			// Сохраняем в историю алгоритмический анализ для совместимости
			h.lastAnalysis[userID] = h.analyzer.Analyze(results)
			return
		}

		log.Printf("[Handler] AI error fallback: %v", err)
		// Если ИИ отвалился по таймауту/ошибке, бот просто пойдет дальше и выдаст обычный анализ!
	}

	// ==========================================
	// ОБЫЧНЫЙ ВЫВОД ДЛЯ FREE И PREMIUM
	// ==========================================
	analysisResult := h.analyzer.Analyze(results)
	h.lastAnalysis[userID] = analysisResult

	h.sendSearchResultsWithAnalysis(chatID, query, results, analysisResult, fromCache)
}

func (h *Handler) sendSearchResultsWithAnalysis(chatID int64, query string, results *marketplace.AggregatedResult, analysis *analysis.AnalysisResult, fromCache bool) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🔍 *%s*\n", sanitizeString(query)))
	if fromCache {
		sb.WriteString("⚡️ Быстрый результат из кэша\n")
	}
	sb.WriteString("\n")

	sb.WriteString("📊 *НАЙДЕНО ТОВАРОВ:*\n")
	for mpName, products := range results.Results {
		mpEmoji := "📦"
		if mpName == "OZON" {
			mpEmoji = "🔵"
		} else if mpName == "Wildberries" {
			mpEmoji = "🟣"
		} else if mpName == "Avito" {
			mpEmoji = "🟢"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %d шт.\n", mpEmoji, mpName, len(products)))
	}
	sb.WriteString("\n")

	// ВЫВОДИМ ЛУЧШИХ ПО ПЛОЩАДКАМ
	sb.WriteString("🥇 *ЛУЧШИЕ ПО ПЛОЩАДКАМ:*\n\n")
	for mpName, best := range analysis.BestByMarketplace {
		mpEmoji := "📦"
		if mpName == "OZON" {
			mpEmoji = "🔵"
		} else if mpName == "Wildberries" {
			mpEmoji = "🟣"
		} else if mpName == "Avito" {
			mpEmoji = "🟢"
		}

		name := truncateUTF8(sanitizeString(best.Name), 40)
		sb.WriteString(fmt.Sprintf("%s *%s*\n%s\n", mpEmoji, mpName, name))
		if best.Price > 0 {
			sb.WriteString(fmt.Sprintf("💰 %.0f руб.", best.Price))
			if best.Discount > 0 {
				sb.WriteString(fmt.Sprintf(" (-%d%%)", best.Discount))
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString("💰 Цена по запросу на сайте\n")
		}
		if best.Condition != "" {
			sb.WriteString(fmt.Sprintf("⚠️ Состояние: %s\n", best.Condition))
		}
		sb.WriteString(fmt.Sprintf("🔗 [Перейти к товару](%s)\n\n", best.URL))
	}

	// ВЫВОДИМ АБСОЛЮТНОГО ПОБЕДИТЕЛЯ
	if analysis.BestOverall != nil {
		sb.WriteString("🏆 *АБСОЛЮТНО ЛУЧШИЙ ВЫБОР:*\n")
		best := analysis.BestOverall
		name := truncateUTF8(sanitizeString(best.Name), 45)
		sb.WriteString(fmt.Sprintf("%s\n💰 %.0f руб. — %s\n🔗 [Смотреть](%s)\n\n", name, best.Price, best.Reason, best.URL))
	}

	text := sb.String()
	if !utf8.ValidString(text) {
		text = sanitizeString(text)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
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
	msg := tgbotapi.NewMessage(message.Chat.ID, `📷 *Отправьте фото товара*

Я распознаю товар по картинке и найду его на *Wildberries*, *Ozon* и *Avito*.

💡 *Советы:*
• Фото должно быть чётким
• Товар должен быть хорошо виден
• Лучше фотографировать на светлом фоне (или отправлять картинку как файл)`)
	msg.ParseMode = "Markdown"
	h.bot.Send(msg)
}

func (h *Handler) handleImageSearch(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	delete(h.userStates, userID)

	canSearch, _, err := h.subService.CanUserSearch(ctx, userID, subscription.SearchTypeImage)
	if err != nil || !canSearch {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ У вас закончились бесплатные поиски по фото. Оформите подписку.")
		h.bot.Send(msg)
		return
	}

	// 1. Узнаем уровень подписки
	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	tier := "free"
	if err == nil && user != nil {
		tier = user.GetTier()
	}

	searchMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Распознаю изображение и ищу товары...")
	sentMsg, _ := h.bot.Send(searchMsg)

	// 2. ДОСТАЕМ ФАЙЛ (Поддержка и сжатых фото, и документов)
	var fileID string
	if message.Photo != nil && len(message.Photo) > 0 {
		// Берем самое большое качество
		fileID = message.Photo[len(message.Photo)-1].FileID
	} else if message.Document != nil {
		// Если прислали как файл (документ)
		mime := message.Document.MimeType
		if strings.HasPrefix(mime, "image/") {
			fileID = message.Document.FileID
		}
	}

	if fileID == "" {
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Пожалуйста, отправьте именно фотографию или картинку.")
		h.bot.Send(msg)
		return
	}

	file, err := h.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf("[Handler] Error getting file: %v", err)
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось загрузить фото от Telegram")
		h.bot.Send(msg)
		return
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", h.cfg.TelegramToken, file.FilePath)

	// 3. Запускаем поиск Яндекса
	imageResult, err := h.imageSearcher.SearchByImageURL(ctx, fileURL)
	if err != nil || !imageResult.Success || len(imageResult.Products) == 0 {
		log.Printf("[Handler] Image search error or empty: %v", err)
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))
		h.offerManualSearch(message.Chat.ID, userID)
		return
	}

	h.subService.UseSearch(ctx, userID, subscription.SearchTypeImage)
	h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID))

	// 4. Фильтруем результаты (Авито только для Premium/Pro)
	var filteredProducts []marketplace.Product
	for _, p := range imageResult.Products {
		if p.Marketplace == "Avito" && tier == "free" {
			continue // Бесплатным юзерам Авито не показываем
		}
		filteredProducts = append(filteredProducts, p)
	}

	// 5. Группируем результаты для агрегатора и ИИ
	resultsMap := make(map[string][]marketplace.Product)
	for _, p := range filteredProducts {
		resultsMap[p.Marketplace] = append(resultsMap[p.Marketplace], p)
	}

	aggregatedResult := &marketplace.AggregatedResult{
		Query:      imageResult.Query,
		Results:    resultsMap,
		TotalCount: len(filteredProducts),
	}

	// 6. МАГИЯ PRO-ПОДПИСКИ (AI-АГЕНТ)
	if tier == "pro" {
		aiWaitMsg := tgbotapi.NewMessage(message.Chat.ID, "🤖 *AI-Агент* анализирует найденные товары по фото. Подождите пару секунд...")
		aiWaitMsg.ParseMode = "Markdown"
		sentWait, _ := h.bot.Send(aiWaitMsg)

		aiResponse, err := h.aiAgent.Analyze(ctx, aggregatedResult)
		h.bot.Request(tgbotapi.NewDeleteMessage(message.Chat.ID, sentWait.MessageID))

		if err == nil && aiResponse != "" {
			msg := tgbotapi.NewMessage(message.Chat.ID, "📷 Найдено по изображению:\n\n"+aiResponse)
			msg.ParseMode = "Markdown"
			msg.DisableWebPagePreview = true
			msg.ReplyMarkup = MainMenuKeyboard()
			h.bot.Send(msg)
			return
		}
		log.Printf("[Handler] AI error fallback for image: %v", err)
	}

	// 7. ОБЫЧНЫЙ ВЫВОД ДЛЯ FREE И PREMIUM
	analysisResult := h.analyzer.Analyze(aggregatedResult)
	h.sendImageSearchResultsWithAnalysis(message.Chat.ID, imageResult.Query, aggregatedResult, analysisResult)

	// ═══════ Реферальный бонус ═══════
	bonusGiven, referrerID, _ := h.referralSvc.CheckSearchBonus(ctx, userID)
	if bonusGiven {
		bonus := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("🎉 Вы сделали %d поисков! +%d дней подписки по реф. программе!", service.ReferralSearchTarget, service.ReferralBonusDays))
		h.bot.Send(bonus)
		if referrerID > 0 {
			h.bot.Send(tgbotapi.NewMessage(referrerID, fmt.Sprintf("🎯 Ваш приглашённый сделал %d поисков! +%d дней подписки!", service.ReferralSearchTarget, service.ReferralBonusDays)))
		}
	}
}

func (h *Handler) sendImageSearchResultsWithAnalysis(chatID int64, query string, results *marketplace.AggregatedResult, analysis *analysis.AnalysisResult) {
	var sb strings.Builder

	sb.WriteString("📷 *Найдено по изображению*\n")
	if query != "" {
		sb.WriteString(fmt.Sprintf("Предположительный запрос: _%s_\n", sanitizeString(query)))
	}
	sb.WriteString("\n")

	sb.WriteString("📊 *НАЙДЕНО ТОВАРОВ:*\n")
	for mpName, products := range results.Results {
		mpEmoji := "📦"
		if mpName == "OZON" {
			mpEmoji = "🔵"
		} else if mpName == "Wildberries" {
			mpEmoji = "🟣"
		} else if mpName == "Avito" {
			mpEmoji = "🟢"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %d шт.\n", mpEmoji, mpName, len(products)))
	}
	sb.WriteString("\n")

	// ЛУЧШИЕ ПО ПЛОЩАДКАМ
	if len(analysis.BestByMarketplace) > 0 {
		sb.WriteString("🥇 *ЛУЧШИЕ ПО ПЛОЩАДКАМ:*\n\n")
		for mpName, best := range analysis.BestByMarketplace {
			mpEmoji := "📦"
			if mpName == "OZON" {
				mpEmoji = "🔵"
			} else if mpName == "Wildberries" {
				mpEmoji = "🟣"
			} else if mpName == "Avito" {
				mpEmoji = "🟢"
			}

			name := truncateUTF8(sanitizeString(best.Name), 40)
			sb.WriteString(fmt.Sprintf("%s *%s*\n%s\n", mpEmoji, mpName, name))
			if best.Price > 0 {
				sb.WriteString(fmt.Sprintf("💰 %.0f руб.", best.Price))
				if best.Discount > 0 {
					sb.WriteString(fmt.Sprintf(" (-%d%%)", best.Discount))
				}
				sb.WriteString("\n")
			} else {
				sb.WriteString("💰 Цена по запросу\n")
			}
			if best.Condition != "" {
				sb.WriteString(fmt.Sprintf("⚠️ Состояние: %s\n", best.Condition))
			}
			sb.WriteString(fmt.Sprintf("🔗 [Перейти к товару](%s)\n\n", best.URL))
		}
	}

	// АБСОЛЮТНЫЙ ПОБЕДИТЕЛЬ
	if analysis.BestOverall != nil {
		sb.WriteString("🏆 *АБСОЛЮТНО ЛУЧШИЙ ВЫБОР:*\n")
		best := analysis.BestOverall
		name := truncateUTF8(sanitizeString(best.Name), 45)
		sb.WriteString(fmt.Sprintf("%s\n💰 %.0f руб.\n🔗 [Смотреть](%s)\n\n", name, best.Price, best.URL))
	}

	text := sb.String()
	if !utf8.ValidString(text) {
		text = sanitizeString(text)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = MainMenuKeyboard()

	_, err := h.bot.Send(msg)
	if err != nil {
		log.Printf("[Handler] Error sending image results: %v", err)
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
		return
	}

	// Берем актуальные цены из конфига (переводим из копеек в рубли)
	premPrice := float64(h.cfg.SubscriptionPrice) / 100
	proPrice := float64(h.cfg.ProSubscriptionPrice) / 100

	// Формируем статус юзера
	subStatus := "❌ Статус: *Не активна*\n"
	if user.HasActiveSubscription() {
		tierName := "Premium 💎"
		if user.GetTier() == "pro" {
			tierName = "PRO 👑"
		}

		endDate := "♾️ Навсегда"
		if user.SubscriptionEnd != nil {
			endDate = user.SubscriptionEnd.Format("02.01.2006")
		}
		subStatus = fmt.Sprintf("✅ Статус: *Активна* (%s)\n📅 До: %s\n", tierName, endDate)
	}

	text := fmt.Sprintf(`💎 *Подписка MarketBot*

%s
📊 Поисков выполнено: %d

*Действующие тарифы:*
💎 *Premium* — %.0f руб/мес
✅ Безлимитный поиск
✅ Поиск по фото (WB + Ozon + Avito)

👑 *PRO* — %.0f руб/мес
✅ Всё из Premium
✅ AI-Аналитика товаров (GPT-4o)
✅ Определение Б/У или Новое

👇 Выберите тариф для оформления:`, subStatus, user.SearchCount, premPrice, proPrice)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"

	// Создаем кнопки для выбора
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("Купить Premium (%.0f₽)", premPrice), "buy_premium"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("Купить PRO (%.0f₽)", proPrice), "buy_pro"),
		),
	)
	msg.ReplyMarkup = keyboard
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
	if err := h.repo.ExtendSubscription(ctx, message.From.ID, promo.FreeDays, promo.Tier); err != nil {
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
	userID := message.From.ID

	// Проверяем, является ли пользователь админом
	isAdmin := false
	if userID == h.cfg.AdminTelegramID {
		isAdmin = true
	}

	canUse, daysLeft, err := h.referralSvc.CanUseReferral(ctx, userID)
	if err != nil {
		h.bot.Send(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка загрузки рефералов"))
		return
	}

	// Если это НЕ админ и нужно еще подождать — показываем заглушку
	if !isAdmin && daysLeft > 0 {
		text := fmt.Sprintf("⏳ Реферальная программа откроется через <b>%d дн.</b>\n\n"+
			"Пользуйтесь ботом, и скоро вы сможете приглашать друзей!", daysLeft)
		m := tgbotapi.NewMessage(message.Chat.ID, text)
		m.ParseMode = "HTML"
		h.bot.Send(m)
		return
	}

	link := h.referralSvc.GetReferralLink(userID)
	total, searchBonuses, _ := h.referralSvc.GetStats(ctx, userID)

	slotsLeft := service.ReferralMaxInvites - total
	if slotsLeft < 0 {
		slotsLeft = 0
	}

	// Для админа показываем бесконечное количество слотов
	if isAdmin {
		slotsLeft = 9999
	}

	limitText := ""
	if !isAdmin && !canUse && daysLeft == 0 {
		limitText = "\n\n⚠️ <i>Лимит приглашений исчерпан</i>"
	}

	maxInvites := service.ReferralMaxInvites
	if isAdmin {
		maxInvites = 9999
	}

	// ИСПОЛЬЗУЕМ HTML ТЕГИ ВМЕСТО MARKDOWN (<b> вместо *, <i> вместо _)
	text := fmt.Sprintf(`👥 <b>Реферальная программа</b>

🔗 Ваша ссылка:
%s

📊 <b>Статистика:</b>
👤 Приглашено: %d/%d
🎯 Активных (20+ поисков): %d
📭 Осталось слотов: %d

💎 <b>Бонусы:</b>
• +%d дней вам и другу за регистрацию
• +%d дней вам и другу за 20 поисков%s`,
		link, total, maxInvites, searchBonuses, slotsLeft,
		service.ReferralBonusDays, service.ReferralBonusDays, limitText)

	m := tgbotapi.NewMessage(message.Chat.ID, text)
	m.ParseMode = "HTML"           // ПЕРЕКЛЮЧИЛИ НА HTML
	m.DisableWebPagePreview = true // Чтобы ссылка не разворачивалась в огромное превью

	// Ловим и выводим ошибку, если Телеграм почему-то не принял сообщение
	_, err = h.bot.Send(m)
	if err != nil {
		log.Printf("[Referral] CRITICAL Error sending message: %v", err)
	}
}

// ==================== 👤 Профиль ====================

func (h *Handler) handleProfile(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		return
	}

	subStatus := fmt.Sprintf("❌ Нет (осталось %d поисков)", user.FreeSearchesLeft)
	if user.HasActiveSubscription() {
		subStatus = "✅ " + user.GetTier()
	}

	cityText := "Не указан (Поиск по всей РФ)"
	if user.City != "" {
		cityText = user.City
	}

	text := fmt.Sprintf(`👤 *Ваш Профиль*

Имя: %s %s
Поисков выполнено: %d
Подписка: %s
📍 *Ваш город:* %s`, user.FirstName, user.LastName, user.SearchCount, subStatus, cityText)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "Markdown"

	// Добавляем инлайн-кнопку для изменения города
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📍 Установить мой город", "set_city"),
		),
	)
	msg.ReplyMarkup = keyboard
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
	userID := callback.From.ID

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

	case "set_city":
		h.userStates[callback.From.ID] = "waiting_city"
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "📍 Напишите название вашего города (например: Нижний Новгород):")
		h.bot.Send(msg)
		h.bot.Request(tgbotapi.NewCallback(callback.ID, ""))

	case "buy_premium":
		paymentInfo, err := h.subService.CreateSubscriptionPayment(ctx, userID, callback.From.UserName, "premium")
		if err == nil {
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "💎 Оплата тарифа *Premium*:")
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = SubscriptionKeyboard(paymentInfo.PaymentURL) // Твоя функция клавиатуры с кнопкой URL
			h.bot.Send(msg)
		} else {
			log.Printf("[Handler] Error creating premium payment: %v", err)
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "❌ Ошибка создания платежа. Попробуйте позже.")
			h.bot.Send(msg)
		}
		h.bot.Request(tgbotapi.NewCallback(callback.ID, ""))

	case "buy_pro":
		paymentInfo, err := h.subService.CreateSubscriptionPayment(ctx, userID, callback.From.UserName, "pro")
		if err == nil {
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "👑 Оплата тарифа *PRO*:")
			msg.ParseMode = "Markdown"
			msg.ReplyMarkup = SubscriptionKeyboard(paymentInfo.PaymentURL)
			h.bot.Send(msg)
		} else {
			log.Printf("[Handler] Error creating pro payment: %v", err)
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "❌ Ошибка создания платежа. Попробуйте позже.")
			h.bot.Send(msg)
		}
		h.bot.Request(tgbotapi.NewCallback(callback.ID, ""))
	}

	// Закрываем крутилку на кнопке в любом случае
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
