package sqlite

import (
	"errors"
	"fmt"

	app "github.com/beldurad/obsidian-telegram-sync-go/internal"
	"gorm.io/gorm"
)

func mapDbErr(err error) error {
	if errors.Is(gorm.ErrCheckConstraintViolated, err) {
		return fmt.Errorf("%s: %s", app.ErrUniqueViolation, err)
	}
	return fmt.Errorf("%s: %s", app.ErrInternal, err)
}
