package sqlite

import (
	"context"
	"database/sql"

	app "github.com/beldurad/obsidian-telegram-sync-go/internal"
)

type TemplateService struct {
	db *sql.DB
}

func NewTemplateService(db *sql.DB) *TemplateService {
	return &TemplateService{db: db}
}

func (s *TemplateService) Save(ctx context.Context, t *app.Template) error {
	tx, err := s.db.Begin()
	if err != nil {
		return app.ErrInternal
	}
	if err = saveTemplate(ctx, tx, t); err != nil {
		return mapDbErr(err)
	}
	return nil
}

func saveTemplate(ctx context.Context, tx *sql.Tx, t *app.Template) error {
	const query = `
	INSERT INTO templates (alias, text)
	VALUES($1, $2)
	`
	_, err := tx.ExecContext(ctx, query, t.Alias, t.Text)
	return err
}
