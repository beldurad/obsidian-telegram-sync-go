package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/http"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/postgres"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

func main() {
	cfg := config.MustLoad()
	loadSecrets(&cfg)

	db := postgres.New(cfg.DatabaseConfig)

	aliasPageCountCache := cache.NewPageCountCache()
	templatePageCountCache := cache.NewPageCountCache()

	aliasStorage := postgres.NewAliasStorage(db, aliasPageCountCache)
	templateStorage := postgres.NewTemplateStorage(db, templatePageCountCache)
	userVaultStorage := postgres.NewUserVaultStorage(db)

	oauthTokenStorage := cache.NewOAuthTokenStorage()
	oauthContextStorage := cache.NewOauthContextStorage()

	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     github.Endpoint,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       strings.Split(cfg.Scopes, " "),
	}

	oauthService := domain.NewOAuthService(&oauthCfg, oauthContextStorage, oauthTokenStorage)

	aliasGetHandler := bot.NewGetAliasesHandler(aliasStorage)
	aliasAddHandler := bot.NewAddAliasHandler(userVaultStorage, oauthService, aliasStorage)
	authHandler := bot.NewAuthHandler(oauthService)
	repoHandler := bot.NewRepoSetHandler(oauthService, userVaultStorage)
	templateGetHandler := bot.NewGetTemplateHandler(templateStorage)
	templateAddHandler := bot.NewTemplateAddHandler(templateStorage)
	noteAddHandler := bot.NewAddNoteHandler(aliasStorage, templateStorage, oauthService, userVaultStorage)

	sessionService := cache.NewChatSessionService()

	b := bot.Init(cfg.TelegramConfig, sessionService)

	b.AddHandlerForCommand(bot.CommandGetAliases, aliasGetHandler)
	b.AddHandlerForState(bot.StateGetAlias, aliasGetHandler)

	b.AddHandlerForCommand(bot.CommandAddAlias, aliasAddHandler)
	b.AddHandlerForState(bot.StateWaitPath, aliasAddHandler)
	b.AddHandlerForState(bot.StateWaitAlias, aliasAddHandler)

	b.AddHandlerForCommand(bot.CommandGetTemplates, templateGetHandler)
	b.AddHandlerForState(bot.StateGetTemplate, templateGetHandler)

	b.AddHandlerForCommand(bot.CommandAddTemplate, templateAddHandler)
	b.AddHandlerForState(bot.StateWaitTemplateValue, templateAddHandler)
	b.AddHandlerForState(bot.StateWaitTemplateName, templateAddHandler)

	b.AddHandlerForCommand(bot.CommandStart, authHandler)

	b.AddHandlerForCommand(bot.RepoSetCommand, repoHandler)
	b.AddHandlerForState(bot.RepoSetState, repoHandler)

	b.AddHandlerForCommand(bot.CommandAddNote, noteAddHandler)
	b.AddHandlerForState(bot.StateNoteWaitAlias, noteAddHandler)
	b.AddHandlerForState(bot.StateNoteWaitTemplate, noteAddHandler)
	b.AddHandlerForState(bot.StateNoteWaitText, noteAddHandler)
	b.AddHandlerForState(bot.StateNoteWaitFilename, noteAddHandler)

	server := http.StartServer(cfg.ServerConfig, oauthService)

	sigCtx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	b.StartListening(sigCtx)
	go func() {
		defer cancel()
		<-sigCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()
		if err := db.Close(); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
			if err := server.Close(); err != nil {
				log.Printf("server close failed: %v", err)
			}
		}
		log.Println("server stopped")
	}()

}

func loadSecrets(cfg *config.Config) {
	dir := os.Getenv("SECRETS_DIR")
	if dir == "" {
		dir = cfg.SecretsDir
	}
	if dir == "" {
		log.Fatal("SECRETS_DIR is not set")
	}
	readSecret := func(name string) string {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Fatalf("cannot read secret %s: %v", name, err)
		}
		return strings.TrimSpace(string(content))
	}
	cfg.DatabaseConfig.Password = readSecret("db_pass")
	cfg.TelegramConfig.Token = readSecret("tg_token")
	cfg.GithubConfig.ClientID = readSecret("github_id")
	cfg.GithubConfig.ClientSecret = readSecret("github_secret")
}
