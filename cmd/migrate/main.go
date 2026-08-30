package main

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	log := slog.Default()

	cfg, err := config.LoadMigrate()
	if err != nil {
		log.Error("loading config", "error", err)
		return
	}
	m, err := migrate.New(
		fmt.Sprintf("file://%s", cfg.MigrationsDir),
		cfg.URL(),
	)
	if err != nil {
		log.Error("create migrate", "error", err)
		return
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Error("run migrations", "error", err)
		return
	}

	log.Info("migrations completed successfully")
}
