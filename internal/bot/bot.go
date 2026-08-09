package bot

import (
	"log"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/foundation/telegram"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
)

func Init(cfg config.TelegramConfig, sessionService bot.ChatSessionService) *bot.Bot {
	client, err := telegram.New(cfg.Token)
	if err != nil {
		log.Fatal(err)
	}
	return bot.New(sessionService, client)
}
