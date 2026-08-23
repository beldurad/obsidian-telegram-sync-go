package domain

import "fmt"

var (
	ErrClient        = fmt.Errorf("Client error")
	ErrNotFound      = fmt.Errorf("Resource not found")
	ErrNotDirectory  = fmt.Errorf("Resource is not directory")
	ErrUnknown       = fmt.Errorf("Unknown error")
	ErrDb            = fmt.Errorf("DB error")
	ErrBadArgument   = fmt.Errorf("Bad argument error")
	ErrAlreadyExists = fmt.Errorf("Resource already exists")
	ErrUnauthorized  = fmt.Errorf("User unauthorized")
	ErrNoRights      = fmt.Errorf("Bot has no right to do this")
)
