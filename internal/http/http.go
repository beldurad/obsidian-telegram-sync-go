package http

import (
	"errors"
	"log"
	"net/http"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
)

func StartServer(cfg config.ServerConfig, authService AuthService) *http.Server {
	mux := http.NewServeMux()
	authHandler := NewAuthHandler(authService)
	mux.Handle(CallbackEndpoint, authHandler)
	server := http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	return &server
}
