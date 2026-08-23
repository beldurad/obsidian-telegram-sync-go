package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
)

func StartServer(cfg config.Config, authService AuthService, log *slog.Logger) *http.Server {
	authHandler := NewAuthHandler(authService, log)
	http.Handle(CallbackEndpoint, authHandler)
	server := http.Server{
		Addr: cfg.Addr,
	}
	go func() {
		err := server.ListenAndServeTLS(
			cfg.CertFile,
			cfg.KeyFile,
		)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	return &server
}
