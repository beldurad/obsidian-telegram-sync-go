package main

import (
	"log"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/sqlite"
)

func main() {
	cfg := config.MustLoad()

	db, err := sqlite.New(cfg.DatabaseConfig)

	if err != nil {
		log.Fatal("fail during db init: ", err)
	}

	botAPI := bot.New(cfg.Token)

	botAPI.StartListening()

	defer db.Close()

}
