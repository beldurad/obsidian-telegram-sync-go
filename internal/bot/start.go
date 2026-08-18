package bot

import (
	"context"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var mainMenu = fmt.Sprintf("%s - Вывод всех команд\n", CommandStart) +
	fmt.Sprintf("%s - Вывод всех команд\n", CommandMenu) +
	fmt.Sprintf("%s - Соединить чат с Github-аккаунтом\n", CommandConnectGithub) +
	fmt.Sprintf("%s - Установить репозиторий, в котором хранится Obsidian Vault\n", CommandSetRepo) +
	fmt.Sprintf("%s - Вывод алиасов путей пользователя\n", CommandGetAliases) +
	fmt.Sprintf("%s - Вывод шаблонов пользователя\n", CommandGetTemplates) +
	fmt.Sprintf("%s - Создать новый алиас пути\n", CommandAddAlias) +
	fmt.Sprintf("%s - Создать новый текстовый шаблон\n", CommandAddTemplate) +
	fmt.Sprintf("%s - Создать заметку\n", CommandAddNote)

var CommandMenu bot.Command = "/menu"
var CommandStart bot.Command = "/start"

type StartHandler struct{}

func NewStartHandler() *StartHandler {
	return &StartHandler{}
}

func (h *StartHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	return bot.Response{
		Message: tgbotapi.NewMessage(u.ChatID, mainMenu),
	}, nil
}

func (h *StartHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	return u.Text == string(CommandStart) || u.Text == string(CommandMenu)
}

var _ bot.Handler = &StartHandler{}
