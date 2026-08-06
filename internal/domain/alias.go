package domain

import "github.com/google/uuid"

type Alias struct {
	ID     uuid.UUID
	ChatID int64
	Path   string
	Alias  string
}
