package bot

import (
	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	NextPageCallback   = "next"
	NextPageButtonText = ">"

	PrevPageCallback   = "prev"
	PrevPageButtonText = "<"
)

func isPageUpdate(u bot.Update) bool {
	return u.CallbackData == NextPageCallback || u.CallbackData == PrevPageCallback
}

type pagePayload struct {
	TotalPages int `json:"total"`
	PageNum    int `json:"page"`
}

func (p pagePayload) handlePageUpdate(u bot.Update) pagePayload {
	switch u.CallbackData {
	case NextPageCallback:
		p.PageNum = min(p.PageNum+1, p.TotalPages-1)
	case PrevPageCallback:
		p.PageNum = max(0, p.PageNum-1)
	}
	return p
}

func payloadFromPage[T any](page domain.Page[T]) pagePayload {
	var payload pagePayload
	payload.TotalPages = page.TotalPages
	payload.PageNum = page.CurPage
	return payload
}

func pageButtons[T any](page domain.Page[T]) []tgbotapi.InlineKeyboardButton {
	buttons := tgbotapi.NewInlineKeyboardRow()
	if page.HasPrev() {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(PrevPageButtonText, PrevPageCallback))
	}
	if page.HasNext() {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(NextPageButtonText, NextPageCallback))
	}
	return buttons
}
