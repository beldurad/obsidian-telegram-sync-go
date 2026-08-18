package bot

import (
	"context"
	"errors"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var ErrCantHandle = fmt.Errorf("Handler can't handle")

type AlreadyExistsErrorHandler struct{}

func (a *AlreadyExistsErrorHandler) Handle(ctx context.Context, u bot.Update, err error) tgbotapi.Chattable {
	return tgbotapi.NewMessage(u.ChatID, "Ресурс уже существует")
}

func (a *AlreadyExistsErrorHandler) Match(err error) bool {
	return errors.Is(err, domain.ErrAlreadyExists)
}
