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

var CommandConnectGithub = bot.Command("/connect_github")

// ===== AUTH =====

type AuthService interface {
	GenerateAuthURL(ctx context.Context, chatID int64) (string, error)
	Client(ctx context.Context, chatID int64) (domain.RemoteStorage, error)
}

type AuthHandler struct {
	authService AuthService
}

var _ bot.Handler = &AuthHandler{}

func NewAuthHandler(authService AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	return u.Text == string(CommandConnectGithub)
}

func (h *AuthHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (resp bot.Response, err error) {
	defer func() {
		if err != nil {
			s.ToDefault()
		}
	}()
	_, err = h.authService.Client(ctx, u.ChatID)
	if err != nil {
		resp, err = h.startAuth(ctx, s)
		return
	}
	msgCfg := tgbotapi.NewMessage(u.ChatID, "Вы уже подключены")
	resp, err = bot.Response{
		Message: msgCfg,
	}, nil
	return
}

func (h *AuthHandler) startAuth(ctx context.Context, chatSession *bot.ChatSession) (bot.Response, error) {
	const op = "startAuth"
	chatID := chatSession.ChatID()

	url, err := h.authService.GenerateAuthURL(ctx, chatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: generating auth url: %w", op, err)
	}

	msgCfg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Перейдите по этой ссылке для Github-авторизации: %s", url))

	return bot.Response{
		Message: msgCfg,
	}, nil
}
