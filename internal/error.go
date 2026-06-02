package app

import "errors"

var (
	ErrUniqueViolation = errors.New("Argument is not unique")
	ErrInvalidState    = errors.New("Argument is in invalid state")
	ErrInternal        = errors.New("Internal server error")
)
