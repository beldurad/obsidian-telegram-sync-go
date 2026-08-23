package client

import (
	"net/http"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

func ConvertStatusCodeToError(code int) error {
	if code < 400 {
		return nil
	}
	switch code {
	default:
		return domain.ErrClient

	case http.StatusBadRequest:
		return domain.ErrBadArgument

	case http.StatusUnauthorized:
		return domain.ErrUnauthorized

	case http.StatusForbidden:
		return domain.ErrNoRights

	case http.StatusNotFound:
		return domain.ErrNotFound
	}
}
