package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"marketplace-bot/internal/config"
	"marketplace-bot/internal/database"
	"marketplace-bot/internal/marketplace"
	"marketplace-bot/internal/subscription"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Handler struct {
	bot        *tgbotapi.BotAPI
	repo       *database.Repository
	aggregator *marketplace.Aggregator
	subService *subscription.Service
	cfg        *config.Config

	// Состояния пользователей
	userStates map[int64]string
	lastSearch map[int64]string
}

func NewHandler(
	bot *tgbotapi.BotAPI,
	repo *database.Repository,
	aggregator *marketplace.Aggregator,
	subService *subscription.Service,
	cfg *config.Config,
) *Handler {
	return &Handler{
		bot:        bot,
		repo:       repo,
		aggregator: aggregator,
		subService: subService,
		cfg:        cfg,
		userStates: make(map[int64]string),
		lastSearch: make(map[int64]string),
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

	// Создаем/обновляем пользователя
	_, err := h.repo.CreateUser(ctx, userID, message.From.UserName, message.From.FirstName, message.From.LastName)
	if err != nil {
		log.Printf("Error creating user: %v", err)
	}

	// Проверяем, ожидаем ли мы поисковый запрос
	if state, ok := h.userStates[userID]; ok && state == "waiting_search" {
		h.handleSearchQuery(ctx, message)
		return
	}

	switch message.Text {
	case "/start":
		h.handleStart(message)
	case "🔍 Поиск товаров":
		h.handleSearchStart(message)
	case "💎 Подписка":
		h.handleSubscription(ctx, message)
	case "👤 Профиль":
		h.handleProfile(ctx, message)
	case "❓ Помощь":
		h.handleHelp(message)
	default:
		// Если это текст и мы не ждем поиск - это поисковый запрос
		if !strings.HasPrefix(message.Text, "/") {
			h.userStates[userID] = "waiting_search"
			h.handleSearchQuery(ctx, message)
		}
	}
}

func (h *Handler) handleStart(message *tgbotapi.Message) {
	text := fmt.Sprintf(`👋 Привет, %s!

🛒 Я бот для поиска товаров по маркетплейсам OZON и Wildberries.

📦 Что я умею:
• Искать товары на двух площадках одновременно
• Сравнивать цены
• Показывать скидки и рейтинги

🎁 У вас есть 5 бесплатных поисков!
После этого — подписка всего 100 ₽/месяц.

Используйте кнопки меню ниже 👇`, message.From.FirstName)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = MainMenuKeyboard()
	msg.ParseMode = "HTML"
	h.bot.Send(msg)
}

func (h *Handler) handleSearchStart(message *tgbotapi.Message) {
	h.userStates[message.From.ID] = "waiting_search"

	msg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Введите название товара для поиска:")
	h.bot.Send(msg)
}

func (h *Handler) handleSearchQuery(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID
	query := strings.TrimSpace(message.Text)

	delete(h.userStates, userID)

	if len(query) < 2 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Слишком короткий запрос. Введите минимум 2 символа.")
		h.bot.Send(msg)
		return
	}

	// Проверяем возможность поиска
	canSearch, freeLeft, err := h.subService.CanUserSearch(ctx, userID)
	if err != nil {
		log.Printf("Error checking search ability: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Произошла ошибка. Попробуйте позже.")
		h.bot.Send(msg)
		return
	}

	if !canSearch {
		text := `❌ У вас закончились бесплатные поиски!

💎 Оформите подписку всего за 100 ₽/месяц и получите:
• Безлимитный поиск
• Приоритетную поддержку

Нажмите "💎 Подписка" для оформления.`
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		h.bot.Send(msg)
		return
	}

	// Отправляем сообщение о поиске
	searchMsg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Ищу товары...")
	sentMsg, _ := h.bot.Send(searchMsg)

	// Используем поиск
	if err := h.subService.UseSearch(ctx, userID); err != nil {
		log.Printf("Error using search: %v", err)
	}

	// Выполняем поиск
	results := h.aggregator.Search(ctx, query, 5)

	// Сохраняем историю
	h.repo.SaveSearchHistory(ctx, userID, query, results.TotalCount)
	h.lastSearch[userID] = query

	// Удаляем сообщение о поиске
	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, sentMsg.MessageID)
	h.bot.Request(deleteMsg)

	// Формируем ответ
	if results.TotalCount == 0 {
		text := fmt.Sprintf("😔 По запросу \"%s\" ничего не найдено.\n\nПопробуйте изменить запрос.", query)
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		h.bot.Send(msg)
		return
	}

	// Отправляем результаты
	h.sendSearchResults(message.Chat.ID, query, results, freeLeft)
}

func (h *Handler) sendSearchResults(chatID int64, query string, results *marketplace.AggregatedResult, freeLeft int) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 Результаты по запросу: <b>%s</b>\n\n", query))

	for mpName, products := range results.Results {
		if len(products) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("📦 <b>%s</b>:\n", mpName))

		for i, p := range products {
			if i >= 3 { // Показываем только топ-3 с каждого маркетплейса
				break
			}

			name := p.Name
			if len(name) > 50 {
				name = name[:50] + "..."
			}

			sb.WriteString(fmt.Sprintf("├ <a href=\"%s\">%s</a>\n", p.URL, name))

			if p.OldPrice > 0 && p.OldPrice > p.Price {
				sb.WriteString(fmt.Sprintf("│  💰 <b>%.0f ₽</b> <s>%.0f ₽</s>", p.Price, p.OldPrice))
				if p.Discount > 0 {
					sb.WriteString(fmt.Sprintf(" (-%d%%)", p.Discount))
				}
				sb.WriteString("\n")
			} else {
				sb.WriteString(fmt.Sprintf("│  💰 <b>%.0f ₽</b>\n", p.Price))
			}

			if p.Rating > 0 {
				sb.WriteString(fmt.Sprintf("│  ⭐ %.1f", p.Rating))
				if p.ReviewCount > 0 {
					sb.WriteString(fmt.Sprintf(" (%d отзывов)", p.ReviewCount))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("│\n")
		}
		sb.WriteString("\n")
	}

	// Добавляем информацию об оставшихся поисках
	if freeLeft > 0 {
		sb.WriteString(fmt.Sprintf("📊 Осталось бесплатных поисков: %d\n", freeLeft-1))
	} else if freeLeft == 0 {
		sb.WriteString("⚠️ Это был последний бесплатный поиск!\n")
	}

	// Показываем ошибки, если есть
	if len(results.Errors) > 0 {
		sb.WriteString("\n⚠️ Не удалось получить данные: ")
		for name, errMsg := range results.Errors {
			sb.WriteString(fmt.Sprintf("%s (%s) ", name, errMsg))
		}
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = MainMenuKeyboard()
	h.bot.Send(msg)
}

func (h *Handler) handleSubscription(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Произошла ошибка. Попробуйте позже.")
		h.bot.Send(msg)
		return
	}

	if user.HasActiveSubscription() {
		text := fmt.Sprintf(`✅ У вас активная подписка!

📅 Действует до: %s

Вам доступен безлимитный поиск по маркетплейсам.`,
			user.SubscriptionEnd.Format("02.01.2006 15:04"))
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		h.bot.Send(msg)
		return
	}

	// Создаем платеж
	paymentInfo, err := h.subService.CreateSubscriptionPayment(ctx, userID, message.From.UserName)
	if err != nil {
		log.Printf("Error creating payment: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка создания платежа. Попробуйте позже.")
		h.bot.Send(msg)
		return
	}

	text := `💎 <b>Подписка MarketBot</b>

💰 Стоимость: <b>100 ₽/месяц</b>

Что вы получите:
✅ Безлимитный поиск товаров
✅ Поиск по OZON и Wildberries
✅ Сравнение цен
✅ Информация о скидках

Нажмите кнопку ниже для оплаты 👇`

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = SubscriptionKeyboard(paymentInfo.PaymentURL)
	h.bot.Send(msg)
}

func (h *Handler) handleProfile(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Произошла ошибка. Попробуйте позже.")
		h.bot.Send(msg)
		return
	}

	var subStatus string
	if user.HasActiveSubscription() {
		subStatus = fmt.Sprintf("✅ Активна (до %s)", user.SubscriptionEnd.Format("02.01.2006"))
	} else {
		subStatus = fmt.Sprintf("❌ Не активна (бесплатных поисков: %d)", user.FreeSearchesLeft)
	}

	text := fmt.Sprintf(`👤 <b>Ваш профиль</b>

📝 Имя: %s %s
🆔 ID: <code>%d</code>
📊 Всего поисков: %d

💎 Подписка: %s

📅 Дата регистрации: %s`,
		user.FirstName, user.LastName,
		user.TelegramID,
		user.SearchCount,
		subStatus,
		user.CreatedAt.Format("02.01.2006"),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "HTML"
	h.bot.Send(msg)
}

func (h *Handler) handleHelp(message *tgbotapi.Message) {
	text := `❓ <b>Помощь</b>

🔍 <b>Как искать товары:</b>
1. Нажмите "🔍 Поиск товаров"
2. Введите название товара
3. Получите результаты с OZON и Wildberries

💎 <b>Подписка:</b>
• Стоимость: 100 ₽/месяц
• Безлимитный поиск
• Оплата через T-Bank

📞 <b>Поддержка:</b>
По всем вопросам: @your_support_username

🤖 Версия бота: 1.0.0`

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = "HTML"
	h.bot.Send(msg)
}

func (h *Handler) handleCallback(callback *tgbotapi.CallbackQuery) {
	ctx := context.Background()

	switch callback.Data {
	case "check_payment":
		h.handleCheckPayment(ctx, callback)
	case "back_to_menu":
		h.handleBackToMenu(callback)
	case "show_more":
		h.handleShowMore(ctx, callback)
	}

	// Отвечаем на callback, чтобы убрать "часики"
	h.bot.Request(tgbotapi.NewCallback(callback.ID, ""))
}

func (h *Handler) handleCheckPayment(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID

	// Проверяем статус подписки
	user, err := h.repo.GetUserByTelegramID(ctx, userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	if user.HasActiveSubscription() {
		text := "✅ Оплата прошла успешно! Подписка активирована."
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
		msg.ReplyMarkup = MainMenuKeyboard()
		h.bot.Send(msg)
	} else {
		h.bot.Request(tgbotapi.NewCallback(callback.ID, "⏳ Оплата пока не получена"))
	}
}

func (h *Handler) handleBackToMenu(callback *tgbotapi.CallbackQuery) {
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "📱 Главное меню")
	msg.ReplyMarkup = MainMenuKeyboard()
	h.bot.Send(msg)
}

func (h *Handler) handleShowMore(ctx context.Context, callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID

	if query, ok := h.lastSearch[userID]; ok {
		// Повторяем поиск с большим лимитом
		results := h.aggregator.Search(ctx, query, 10)
		h.sendSearchResults(callback.Message.Chat.ID, query, results, -1)
	}
}
