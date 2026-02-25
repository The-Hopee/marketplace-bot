// internal/bot/keyboards.go
package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func MainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🔍 Поиск товаров"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("💎 Подписка"),
			tgbotapi.NewKeyboardButton("👤 Профиль"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❓ Помощь"),
		),
	)
}

func SearchResultKeyboard(hasMore bool) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	if hasMore {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📄 Показать ещё", "show_more"),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔄 Сортировка по цене", "sort_price"),
		tgbotapi.NewInlineKeyboardButtonData("⭐ По рейтингу", "sort_rating"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func SubscriptionKeyboard(paymentURL string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("💳 Оплатить 100 ₽", paymentURL),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Проверить оплату", "check_payment"),
		),
	)
}

func BackKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Назад", "back_to_menu"),
		),
	)
}

func ProductKeyboard(productURL string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🛒 Перейти к товару", productURL),
		),
	)
}
