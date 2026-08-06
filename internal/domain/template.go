package domain

import (
	"time"

	"github.com/google/uuid"
)

type Template struct {
	ID        uuid.UUID
	ChatID    int64
	Name      string
	Value     string
	CreatedAt time.Time
}
