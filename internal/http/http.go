package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
)

func StartServer(cfg config.Config, authService AuthService, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	authHandler := NewAuthHandler(authService, log)
	mux.Handle(CallbackEndpoint, authHandler)
	server := http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
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
