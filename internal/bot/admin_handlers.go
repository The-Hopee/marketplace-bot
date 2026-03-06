package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"marketplace-bot/internal/database"
	"marketplace-bot/internal/service"
)

type AdminState struct {
	Action string
	Data   map[string]string
}

type AdminHandlers struct {
	bot             *tgbotapi.BotAPI
	repo            *database.Repository
	broadcastSvc    *service.BroadcastService
	adSvc           *service.AdService
	adminStates     map[int64]*AdminState
	adminTelegramID int64
}

func NewAdminHandlers(
	bot *tgbotapi.BotAPI,
	repo *database.Repository,
	broadcastSvc *service.BroadcastService,
	adSvc *service.AdService,
	adminTelegramID int64, // ← НОВОЕ
) *AdminHandlers {
	log.Printf("[Admin] Admin Telegram ID from config: %d", adminTelegramID)
	return &AdminHandlers{
		bot:             bot,
		repo:            repo,
		broadcastSvc:    broadcastSvc,
		adSvc:           adSvc,
		adminStates:     make(map[int64]*AdminState),
		adminTelegramID: adminTelegramID,
	}
}

func (h *AdminHandlers) HandleAdminCommand(ctx context.Context, msg *tgbotapi.Message) bool {
	isAdmin, err := h.repo.IsAdmin(ctx, msg.From.ID)
	if err != nil {
		log.Printf("[Admin] IsAdmin check error for %d: %v", msg.From.ID, err)
	}

	log.Printf("[Admin] User %d isAdmin=%v, configAdminID=%d", msg.From.ID, isAdmin, h.adminTelegramID)

	// Авто-промоут по ID из конфига
	if !isAdmin && h.adminTelegramID > 0 && h.adminTelegramID == msg.From.ID {
		log.Printf("[Admin] Auto-promoting user %d as admin", msg.From.ID)
		if err := h.repo.AddAdmin(ctx, msg.From.ID); err != nil {
			log.Printf("[Admin] AddAdmin error: %v", err)
		}
		isAdmin = true
	}

	if !isAdmin {
		return false
	}

	// Многошаговые действия
	if state, ok := h.adminStates[msg.From.ID]; ok {
		return h.handleState(ctx, msg, state)
	}

	switch {
	case msg.Text == "/admin":
		h.showMenu(msg.Chat.ID)
	case msg.Text == "/stats":
		h.showStats(ctx, msg.Chat.ID)
	case msg.Text == "/ads":
		h.showAds(ctx, msg.Chat.ID)
	case msg.Text == "/addad":
		h.startAddAd(msg.From.ID, msg.Chat.ID)
	case strings.HasPrefix(msg.Text, "/deletead "):
		h.deleteAd(ctx, msg)
	case strings.HasPrefix(msg.Text, "/togglead "):
		h.toggleAd(ctx, msg)
	case msg.Text == "/broadcasts":
		h.showBroadcasts(ctx, msg.Chat.ID)
	case msg.Text == "/newbroadcast":
		h.startNewBroadcast(msg.From.ID, msg.Chat.ID)
	case strings.HasPrefix(msg.Text, "/startbroadcast "):
		h.startBroadcast(ctx, msg)
	case msg.Text == "/stopbroadcast":
		h.stopBroadcast(msg.Chat.ID)
	case msg.Text == "/resumebroadcast":
		h.resumeBroadcast(ctx, msg.Chat.ID)
	case msg.Text == "/promos":
		h.showPromos(ctx, msg.Chat.ID)
	case strings.HasPrefix(msg.Text, "/addpromo "):
		h.addPromo(ctx, msg)
	case strings.HasPrefix(msg.Text, "/delpromo "):
		h.deletePromo(ctx, msg)
	case strings.HasPrefix(msg.Text, "/togglepromo "):
		h.togglePromo(ctx, msg)
	default:
		return false
	}
	return true
}

// ==================== Меню ====================

func (h *AdminHandlers) showMenu(chatID int64) {
	text := `🔐 *Админ-панель*

📊 /stats — Статистика

🎟 *Промокоды:*
/promos — Список
/addpromo CODE ДНИ [ЛИМИТ]
/delpromo CODE
/togglepromo CODE

📢 *Реклама:*
/ads — Список
/addad — Добавить
/deletead ID
/togglead ID

📬 *Рассылки:*
/broadcasts — Список
/newbroadcast — Новая
/startbroadcast ID
/stopbroadcast
/resumebroadcast`

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

// ==================== Статистика ====================

func (h *AdminHandlers) showStats(ctx context.Context, chatID int64) {
	total, _ := h.repo.GetTotalUsersCount(ctx)
	subs, _ := h.repo.GetActiveSubscribersCount(ctx)
	revenue, _ := h.repo.GetTotalRevenue(ctx)

	text := fmt.Sprintf(`📊 *Статистика*

👥 Всего пользователей: *%d*
💎 Активных подписок: *%d*
💰 Выручка: *%.2f ₽*`, total, subs, float64(revenue)/100)

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

// ==================== Реклама ====================

func (h *AdminHandlers) showAds(ctx context.Context, chatID int64) {
	ads, _ := h.repo.GetAllAds(ctx)
	if len(ads) == 0 {
		h.bot.Send(tgbotapi.NewMessage(chatID, "Нет рекламных объявлений"))
		return
	}
	var sb strings.Builder
	sb.WriteString("📢 *Реклама:*\n\n")
	for _, a := range ads {
		st := "✅"
		if !a.IsActive {
			st = "❌"
		}
		ctr := 0.0
		if a.ViewsCount > 0 {
			ctr = float64(a.ClicksCount) / float64(a.ViewsCount) * 100
		}
		sb.WriteString(fmt.Sprintf("%s *#%d* %s\n   👁 %d | 👆 %d | CTR %.1f%%\n\n",
			st, a.ID, a.Name, a.ViewsCount, a.ClicksCount, ctr))
	}

	m := tgbotapi.NewMessage(chatID, sb.String())
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

func (h *AdminHandlers) startAddAd(userID, chatID int64) {
	h.adminStates[userID] = &AdminState{Action: "add_ad_name", Data: make(map[string]string)}
	h.bot.Send(tgbotapi.NewMessage(chatID, "📝 Введи название рекламы:"))
}

func (h *AdminHandlers) deleteAd(ctx context.Context, msg *tgbotapi.Message) {
	id, _ := strconv.ParseInt(strings.TrimPrefix(msg.Text, "/deletead "), 10, 64)
	_ = h.repo.DeleteAd(ctx, id)
	h.adSvc.RefreshCache(ctx)
	h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ Реклама #%d удалена", id)))
}

func (h *AdminHandlers) toggleAd(ctx context.Context, msg *tgbotapi.Message) {
	id, _ := strconv.ParseInt(strings.TrimPrefix(msg.Text, "/togglead "), 10, 64)
	ad, err := h.repo.GetAdByID(ctx, id)
	if err != nil {
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Не найдено"))
		return
	}
	ad.IsActive = !ad.IsActive
	_ = h.repo.UpdateAd(ctx, ad)
	h.adSvc.RefreshCache(ctx)

	st := "включена ✅"
	if !ad.IsActive {
		st = "выключена ❌"
	}
	h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("Реклама #%d %s", id, st)))
}

// ==================== Рассылки ====================

func (h *AdminHandlers) showBroadcasts(ctx context.Context, chatID int64) {
	list, _ := h.repo.GetAllBroadcasts(ctx)
	if len(list) == 0 {
		h.bot.Send(tgbotapi.NewMessage(chatID, "Нет рассылок"))
		return
	}

	var sb strings.Builder
	sb.WriteString("📬 *Рассылки:*\n\n")
	for _, b := range list {
		icon := "📝"
		switch b.Status {
		case database.BroadcastRunning:
			icon = "▶️"
		case database.BroadcastPaused:
			icon = "⏸️"
		case database.BroadcastCompleted:
			icon = "✅"
		}
		progress := ""
		if b.TotalUsers > 0 {
			progress = fmt.Sprintf(" (%d/%d)", b.SentCount, b.TotalUsers)
		}
		sb.WriteString(fmt.Sprintf("%s *#%d* %s%s\n", icon, b.ID, b.Name, progress))
	}

	m := tgbotapi.NewMessage(chatID, sb.String())
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

func (h *AdminHandlers) startNewBroadcast(userID, chatID int64) {
	h.adminStates[userID] = &AdminState{Action: "broadcast_name", Data: make(map[string]string)}
	h.bot.Send(tgbotapi.NewMessage(chatID, "📝 Введи название рассылки:"))
}

func (h *AdminHandlers) startBroadcast(ctx context.Context, msg *tgbotapi.Message) {
	id, _ := strconv.ParseInt(strings.TrimPrefix(msg.Text, "/startbroadcast "), 10, 64)
	if h.broadcastSvc.IsRunning() {
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Уже есть активная рассылка"))
		return
	}
	if err := h.broadcastSvc.StartBroadcast(ctx, id); err != nil {
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ "+err.Error()))
		return
	}
	h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("▶️ Рассылка #%d запущена!", id)))
}

func (h *AdminHandlers) stopBroadcast(chatID int64) {
	if !h.broadcastSvc.IsRunning() {
		h.bot.Send(tgbotapi.NewMessage(chatID, "❌ Нет активной рассылки"))
		return
	}
	h.broadcastSvc.StopBroadcast()
	h.bot.Send(tgbotapi.NewMessage(chatID, "⏸️ Рассылка остановлена"))
}

func (h *AdminHandlers) resumeBroadcast(ctx context.Context, chatID int64) {
	if err := h.broadcastSvc.ResumeBroadcast(ctx); err != nil {
		h.bot.Send(tgbotapi.NewMessage(chatID, "❌ "+err.Error()))
		return
	}
	h.bot.Send(tgbotapi.NewMessage(chatID, "▶️ Рассылка продолжена"))
}

// ==================== Промокоды ====================

func (h *AdminHandlers) showPromos(ctx context.Context, chatID int64) {
	list, _ := h.repo.GetAllPromocodes(ctx)
	if len(list) == 0 {
		h.bot.Send(tgbotapi.NewMessage(chatID, "Нет промокодов"))
		return
	}
	var sb strings.Builder
	sb.WriteString("🎟 *Промокоды:*\n\n")
	for _, p := range list {
		st := "✅"
		if !p.IsActive {
			st = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %s — %d дн.", st, p.Code, p.FreeDays))
		sb.WriteString(fmt.Sprintf(" (исп: %d", p.UsedCount))
		if p.MaxUses != nil {
			sb.WriteString(fmt.Sprintf("/%d", *p.MaxUses))
		}
		sb.WriteString(")\n")
	}

	m := tgbotapi.NewMessage(chatID, sb.String())
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

// /addpromo CODE DAYS [LIMIT]
// Пример: /addpromo VIP30 30 50
func (h *AdminHandlers) addPromo(ctx context.Context, msg *tgbotapi.Message) {
	parts := strings.Fields(msg.Text)
	if len(parts) < 3 {
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID,
			"Формат: /addpromo CODE ДНИ [ЛИМИТ]\nПример: /addpromo VIP30 30 50"))
		return
	}

	code := strings.ToUpper(parts[1])
	days, err := strconv.Atoi(parts[2])
	if err != nil || days < 1 {
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Дни должны быть > 0"))
		return
	}

	maxUses := 0
	if len(parts) >= 4 {
		maxUses, _ = strconv.Atoi(parts[3])
	}

	if err := h.repo.CreatePromocode(ctx, code, days, maxUses); err != nil {
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ "+err.Error()))
		return
	}

	text := fmt.Sprintf("✅ Промокод создан\n\nКод: `%s`\nДней подписки: %d", code, days)
	if maxUses > 0 {
		text += fmt.Sprintf("\nЛимит: %d", maxUses)
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	m.ParseMode = "Markdown"
	h.bot.Send(m)
}

func (h *AdminHandlers) deletePromo(ctx context.Context, msg *tgbotapi.Message) {
	code := strings.ToUpper(strings.TrimPrefix(msg.Text, "/delpromo "))
	_ = h.repo.DeletePromocode(ctx, code)
	h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ Промокод %s удалён", code)))
}

func (h *AdminHandlers) togglePromo(ctx context.Context, msg *tgbotapi.Message) {
	code := strings.ToUpper(strings.TrimPrefix(msg.Text, "/togglepromo "))
	_ = h.repo.TogglePromocode(ctx, code)
	h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ Промокод %s переключён", code)))
}

// ==================== Многошаговые действия ====================

func (h *AdminHandlers) handleState(ctx context.Context, msg *tgbotapi.Message, state *AdminState) bool {
	switch state.Action {

	// — Добавление рекламы —
	case "add_ad_name":
		state.Data["name"] = msg.Text
		state.Action = "add_ad_text"
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "📝 Текст рекламы (Markdown):"))
		return true

	case "add_ad_text":
		state.Data["text"] = msg.Text
		state.Action = "add_ad_button"
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "📝 Кнопка (текст|url) или «нет»:"))
		return true

	case "add_ad_button":
		if msg.Text != "нет" && msg.Text != "-" {
			parts := strings.SplitN(msg.Text, "|", 2)
			if len(parts) == 2 {
				state.Data["button_text"] = strings.TrimSpace(parts[0])
				state.Data["button_url"] = strings.TrimSpace(parts[1])
			}
		}

		ad := &database.Ad{
			Name:     state.Data["name"],
			Text:     state.Data["text"],
			IsActive: true,
			Priority: 1,
		}
		if bt, ok := state.Data["button_text"]; ok {
			ad.ButtonText = &bt
			bu := state.Data["button_url"]
			ad.ButtonURL = &bu
		}

		delete(h.adminStates, msg.From.ID)

		if err := h.repo.CreateAd(ctx, ad); err != nil {
			h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Ошибка"))
		} else {
			h.adSvc.RefreshCache(ctx)
			h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("✅ Реклама #%d создана!", ad.ID)))
		}
		return true

	// — Создание рассылки —
	case "broadcast_name":
		state.Data["name"] = msg.Text
		state.Action = "broadcast_text"
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "📝 Текст рассылки:"))
		return true

	case "broadcast_text":
		state.Data["text"] = msg.Text
		state.Action = "broadcast_button"
		h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "📝 Кнопка (текст|url) или «нет»:"))
		return true
	case "broadcast_button":
		if msg.Text != "нет" && msg.Text != "-" {
			parts := strings.SplitN(msg.Text, "|", 2)
			if len(parts) == 2 {
				state.Data["button_text"] = strings.TrimSpace(parts[0])
				state.Data["button_url"] = strings.TrimSpace(parts[1])
			}
		}

		b := &database.Broadcast{
			Name:   state.Data["name"],
			Text:   state.Data["text"],
			Status: database.BroadcastDraft,
		}
		if bt, ok := state.Data["button_text"]; ok {
			b.ButtonText = &bt
			bu := state.Data["button_url"]
			b.ButtonURL = &bu
		}

		delete(h.adminStates, msg.From.ID)

		if err := h.repo.CreateBroadcast(ctx, b); err != nil {
			h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Ошибка"))
		} else {
			h.bot.Send(tgbotapi.NewMessage(msg.Chat.ID,
				fmt.Sprintf("✅ Рассылка #%d создана!\nЗапустить: /startbroadcast %d", b.ID, b.ID)))
		}
		return true
	}

	return false
}
