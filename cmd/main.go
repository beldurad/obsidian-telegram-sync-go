package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	foundationbot "github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/foundation/telegram"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/client/github"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/http"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/postgres"
)

func main() {

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("error loading config", "error", err)
		return
	}

	db, err := postgres.New(cfg.DatabaseConfig)
	if err != nil {
		log.Error("while postgres init", "error", err)
		return
	}

	aliasPageCountCache := cache.NewPageCountCache()
	templatePageCountCache := cache.NewPageCountCache()
	remoteContentCache := cache.NewRemoteContentCache()
	sessionService := cache.NewChatSessionService()

	aliasStorage := postgres.NewAliasStorage(db, aliasPageCountCache)
	templateStorage := postgres.NewTemplateStorage(db, templatePageCountCache)
	userVaultStorage := postgres.NewUserVaultStorage(db)

	oauthTokenStorage := cache.NewRemoteTokenStorage()
	oauthContextStorage := cache.NewRemoteConnectCtxStorage()

	oauthService := github.NewOAuthService(cfg.GithubConfig, oauthContextStorage, oauthTokenStorage, remoteContentCache)

	startHandler := bot.NewStartHandler()
	aliasGetHandler := bot.NewGetAliasesHandler(aliasStorage)
	aliasAddHandler := bot.NewAddAliasHandler(userVaultStorage, oauthService, aliasStorage)
	authHandler := bot.NewAuthHandler(oauthService)
	repoHandler := bot.NewRepoSetHandler(oauthService, userVaultStorage)
	templateGetHandler := bot.NewGetTemplateHandler(templateStorage)
	templateAddHandler := bot.NewTemplateAddHandler(templateStorage)
	noteAddHandler := bot.NewAddNoteHandler(aliasStorage, templateStorage, oauthService, userVaultStorage)

	tgClient, err := telegram.New(cfg.Token, cfg.WebhookURL, cfg.WebhookEndpoint)
	if err != nil {
		log.Error("error while initializing telegram client")
	}

	b := foundationbot.New(sessionService, tgClient, log)
	b.AddHandler(startHandler)
	b.AddHandler(aliasGetHandler)
	b.AddHandler(aliasAddHandler)
	b.AddHandler(authHandler)
	b.AddHandler(repoHandler)
	b.AddHandler(templateGetHandler)
	b.AddHandler(templateAddHandler)
	b.AddHandler(noteAddHandler)

	logMiddleware := bot.NewLogMiddleware(slog.Default())
	b.Use(logMiddleware.Middleware())

	server := http.StartServer(cfg, oauthService, log)

	sigCtx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	go b.StartListening(sigCtx)
	defer cancel()
	<-sigCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	if err := db.Close(); err != nil {
		log.Error("graceful shutdown failed: %v", "error", err)
	}
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed: %v", "error", err)
		if err := server.Close(); err != nil {
			log.Error("server close failed: %v", "error", err)
		}
	}
	log.Info("server stopped")

}
