package bot

import (
	"context"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ===== STATES =====

const StateWaitingRepo = "WAITING_REPO"

// ===== COMMANDS =====

var CommandStart = bot.Command("/start")

// ===== AUTH =====

type AuthService interface {
	GenerateAuthURL(ctx context.Context, chatID int64) (string, error)
	Client(ctx context.Context, chatID int64) (domain.RemoteStorage, error)
}

type AuthHandler struct {
	authService AuthService
}

func NewAuthHandler(authService AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Register(b *bot.Bot, m ...bot.Middleware) {
	b.AddHandlerForCommand(CommandStart, h, m...)
}

func (h *AuthHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	chatSession := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)

	_, err := h.authService.Client(ctx, chatSession.ChatID)
	if err != nil {
		return h.startAuth(ctx, chatSession)
	}
	return h.menu(chatSession)
}

func (h *AuthHandler) startAuth(ctx context.Context, chatSession bot.ChatSession) (bot.Response, error) {
	url, err := h.authService.GenerateAuthURL(ctx, chatSession.ChatID)
	if err != nil {
		return bot.Response{}, err
	}

	msgCfg := tgbotapi.NewMessage(chatSession.ChatID, fmt.Sprintf("Перейдите по этой ссылке для Github-авторизации: %s", url))

	return bot.Response{
		Message: msgCfg,
	}, nil
}

func (h *AuthHandler) menu(session bot.ChatSession) (bot.Response, error) {

	msgCfg := tgbotapi.NewMessage(
		session.ChatID,
		`
		/start /menu - это сообщение
		/template - вывести свои шаблоны текстов
		/alias - вывести все свои алиасы для файловых путей
		/add-template - добавить новый шаблон
		/add-alias - добавить новый алиас
		`,
	)

	return bot.Response{
		Message: msgCfg,
	}, nil

}
