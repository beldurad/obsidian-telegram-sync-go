package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
)

type TemplatePageCountCache interface {
	Put(ctx context.Context, chatID int64, pageSize, count int) error
	Get(ctx context.Context, chatID int64, pageSize int) (count int, ok bool)
	Delete(ctx context.Context, chatID int64, pageSize int) error
}

type TemplateStorage struct {
	db             *sql.DB
	pageCountCache TemplatePageCountCache
}

func NewTemplateStorage(db *sql.DB, pageNumCache TemplatePageCountCache) *TemplateStorage {
	return &TemplateStorage{
		db:             db,
		pageCountCache: pageNumCache,
	}
}

func (s *TemplateStorage) Save(ctx context.Context, t domain.Template) (err error) {
	const op = "TemplateStorage.Save"
	const query = `
	INSERT INTO template (id, chat_id, name, value)
	VALUES
	($1, $2, $3, $4)
	`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%v: beginning transaction: %w: %w", op, domain.ErrDb, err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
		if err != nil {
			err = fmt.Errorf("%v: committing transaction: %w: %w", op, domain.ErrDb, err)
		}
	}()
	_, err = tx.ExecContext(
		ctx,
		query,
		t.ID,
		t.ChatID,
		t.Name,
		t.Value,
	)
	if err != nil {
		pqErr := new(pq.Error)
		if errors.As(err, &pqErr) && pqErr.Code == pqerror.UniqueViolation {
			return fmt.Errorf("%v: saving template, unique violation: %w: %w: %w", op, domain.ErrAlreadyExists, domain.ErrDb, err)
		}
		return fmt.Errorf("%v: saving template: %w: %w", op, domain.ErrDb, err)
	}

	err = s.pageCountCache.Delete(ctx, t.ChatID, domain.DefaultPageSize)
	if err != nil {
		log.Printf("Error while deleting template cache: %v", err)
	}
	return nil
}

func (s *TemplateStorage) TemplatesPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Template], error) {
	const op = "TemplateStorage.TemplatesPage"
	var zero domain.Page[domain.Template]

	if pageNum < 0 || pageSize < 0 {
		return zero, domain.ErrBadArgument
	}
	if pageSize == 0 {
		return zero, nil
	}
	page := domain.Page[domain.Template]{CurPage: pageNum}
	page.CurPage = pageNum

	if pageNum, ok := s.pageCountCache.Get(ctx, chatID, pageSize); ok {
		page.TotalPages = pageNum
	} else {
		const countQuery = `
		SELECT COUNT(id)
		FROM template
		WHERE chat_id = $1
		`
		var count int
		err := s.db.QueryRowContext(
			ctx,
			countQuery, chatID,
		).Scan(&count)
		if err != nil {
			return zero, fmt.Errorf("%v: counting templates: %w: %w", op, domain.ErrDb, err)
		}
		page.TotalPages = count / pageSize
		if count%pageSize != 0 {
			page.TotalPages++
		}
		err = s.pageCountCache.Put(ctx, chatID, pageSize, page.TotalPages)
		if err != nil {
			log.Printf("error while saving template cache: %v", err)
		}
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
	rows, err := s.db.QueryContext(ctx, query, chatID, offset, limit)
	if err != nil {
		return zero, fmt.Errorf("%v: querying templates page: %w: %w", op, domain.ErrDb, err)
	}
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
			return zero, fmt.Errorf("%v: scanning template row: %w: %w", op, domain.ErrDb, err)
		}
		cur++
	}
	page.Values = templates[:cur]
	return page, nil
}

func (s *TemplateStorage) Template(ctx context.Context, id string, chatID int64) (domain.Template, error) {
	const op = "TemplateStorage.Template"
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return domain.Template{}, fmt.Errorf("%v: parsing template id: %w: %w", op, domain.ErrBadArgument, err)
	}

	const query = `
	SELECT chat_id, name, value, created_at
	FROM template
	WHERE id = $1 AND chat_id = $2
	`

	var template domain.Template

	if err := s.db.QueryRowContext(ctx, query, id, chatID).Scan(&template.ChatID, &template.Name, &template.Value, &template.CreatedAt); err != nil {
		return domain.Template{}, fmt.Errorf("%v: querying template: %w: %w", op, domain.ErrDb, err)
	}
	template.ID = parsedID
	return template, nil
}
