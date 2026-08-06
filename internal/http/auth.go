package http

import (
	"context"
	"net/http"
)

type AuthService interface {
	CompleteAuth(ctx context.Context, code string, state string) error
}

type AuthHandler struct {
	service AuthService
}

func (a AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}

	err := a.service.CompleteAuth(ctx, code, state)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Ошибка при завершении авторизации"))
	}

}
