package http

import (
	"context"
	"log/slog"
	"net/http"
)

const CallbackEndpoint = "/callback"

type AuthService interface {
	CompleteAuth(ctx context.Context, code string, state string) error
}

type AuthHandler struct {
	service AuthService
	log     *slog.Logger
}

func NewAuthHandler(service AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{
		service: service,
		log:     log,
	}
}

func (a *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
		return
	}

	err := a.service.CompleteAuth(ctx, code, state)

	if err != nil {
		a.log.Error("error while handling http request", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Ошибка при завершении авторизации"))
	}

}
