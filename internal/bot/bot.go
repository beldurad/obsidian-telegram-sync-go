package bot

import (
	"log/slog"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/foundation/telegram"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
)

func Init(cfg config.Config, sessionService bot.ChatSessionService, log *slog.Logger) *bot.Bot {
	client, err := telegram.New(cfg)
	if err != nil {
		panic(err)
	}
	return bot.New(sessionService, client, log)
}
