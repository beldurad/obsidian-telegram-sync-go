package bot

import (
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func buttonsWithPagination[T any](page domain.Page[T], toButton func(obj T) tgbotapi.InlineKeyboardButton) tgbotapi.InlineKeyboardMarkup {
	keyboard := buttons(page.Values, toButton)
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, pageButtons(page))
	return keyboard
}

func buttons[T any](objects []T, toButton func(obj T) tgbotapi.InlineKeyboardButton) tgbotapi.InlineKeyboardMarkup {
	keyboard := make([][]tgbotapi.InlineKeyboardButton, len(objects))
	for i := range objects {
		keyboard[i] = []tgbotapi.InlineKeyboardButton{
			toButton(objects[i]),
		}
	}
	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}
