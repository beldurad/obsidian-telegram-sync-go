package postgres

import (
	"context"
	"database/sql"
	"math"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

type TemplatePageNumCache interface {
	Put(ctx context.Context, chatID int64, pageSize, count int) error
	Get(ctx context.Context, chatID int64, pageSize int) (count int, ok bool)
}

type TemplateStorage struct {
	db           *sql.DB
	pageNumCache TemplatePageNumCache
}

func NewTemplateStorage(db *sql.DB, pageNumCache TemplatePageNumCache) *TemplateStorage {
	return &TemplateStorage{
		db:           db,
		pageNumCache: pageNumCache,
	}
}

func (s *TemplateStorage) Save(ctx context.Context, t domain.Template) error {
	const query = `
	INSERT INTO template (id, chat_id, name, value)
	VALUES
	($1, $2, $3, $4)
	`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		query,
		t.ID,
		t.ChatID,
		t.Name,
		t.Value,
	)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *TemplateStorage) TemplatesPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Template], error) {
	page := domain.Page[domain.Template]{}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return page, err
	}
	if pageNum, ok := s.pageNumCache.Get(ctx, chatID, pageSize); ok {
		page.TotalPages = pageNum
	} else {
		const countQuery = `
		SELECT COUNT(id)
		FROM template
		WHERE chat_id = $1
		`
		var count int
		err := tx.QueryRowContext(
			ctx,
			countQuery, chatID,
		).Scan(&count)
		if err != nil {
			return page, err
		}
		page.TotalPages = int(
			math.Ceil(
				float64(count) / float64(pageSize),
			),
		)
		s.pageNumCache.Put(ctx, chatID, pageSize, page.TotalPages)
	}
	offset := pageNum * pageSize
	limit := pageSize
	const query = `
	SELECT id, chat_id, name, value
	FROM template
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`
	rows, err := tx.QueryContext(ctx, query, chatID, offset, limit)
	templates := make([]domain.Template, pageSize)
	cur := 0
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(
			&templates[cur].ID,
			&templates[cur].ChatID,
			&templates[cur].Name,
			&templates[cur].Value,
		)
		if err != nil {
			page.TotalPages = 0
			return page, err
		}
		cur++
	}
	page.Values = templates
	return page, nil
}
