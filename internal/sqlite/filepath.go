package sqlite

import (
	"context"
	"database/sql"

	app "github.com/beldurad/obsidian-telegram-sync-go/internal"
)

type FilePathService struct {
	db *sql.DB
}

func NewFilePathService(db *sql.DB) *TemplateService {
	return &TemplateService{db: db}
}

func (s *FilePathService) Save(ctx context.Context, f *app.FilePath) error {
	tx, err := s.db.Begin()
	if err != nil {
		return app.ErrInternal
	}
	if err = saveFilePath(ctx, tx, f); err != nil {
		return mapDbErr(err)
	}
	return nil
}

func saveFilePath(ctx context.Context, tx *sql.Tx, f *app.FilePath) error {
	const query = `
	INSERT INTO filepath (alias, path)
	VALUES($1, $2)
	`
	_, err := tx.ExecContext(ctx, query, f.Alias, f.Path)
	return err
}
