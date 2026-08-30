package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

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
		os.Exit(1)
	}
	m, err := migrate.New(
		fmt.Sprintf("file://%s", cfg.MigrationsDir),
		cfg.URL(),
	)
	if err != nil {
		log.Error("create migrate", "error", err)
		os.Exit(1)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Error("run migrations", "error", err)
		os.Exit(1)
	}

	log.Info("migrations completed successfully")
}
