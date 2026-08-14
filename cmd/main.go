package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	appgithub "github.com/beldurad/obsidian-telegram-sync-go/internal/github"
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

	oauthService := appgithub.NewOAuthService(&oauthCfg, oauthContextStorage, oauthTokenStorage)

	aliasGetHandler := bot.NewGetAliasesHandler(aliasStorage)
	aliasAddHandler := bot.NewAddAliasHandler(userVaultStorage, oauthService, aliasStorage)
	authHandler := bot.NewAuthHandler(oauthService)
	repoHandler := bot.NewRepoSetHandler(oauthService, userVaultStorage)
	templateGetHandler := bot.NewGetTemplateHandler(templateStorage)
	templateAddHandler := bot.NewTemplateAddHandler(templateStorage)
	noteAddHandler := bot.NewAddNoteHandler(aliasStorage, templateStorage, oauthService, userVaultStorage)

	sessionService := cache.NewChatSessionService()

	b := bot.Init(cfg.TelegramConfig, sessionService)

	aliasGetHandler.Register(b)
	aliasAddHandler.Register(b)
	authHandler.Register(b)
	templateGetHandler.Register(b)
	templateAddHandler.Register(b)
	repoHandler.Register(b)
	noteAddHandler.Register(b)

	logMiddleware := bot.NewLogMiddleware(slog.Default())
	b.Use(logMiddleware.Middleware())

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
