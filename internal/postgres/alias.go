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

type AliasPageCountCache interface {
	Put(ctx context.Context, chatID int64, pageSize, count int) error
	Get(ctx context.Context, chatID int64, pageSize int) (count int, ok bool)
	Delete(ctx context.Context, chatID int64, pageSize int) error
}

type AliasStorage struct {
	db             *sql.DB
	pageCountCache AliasPageCountCache
}

func NewAliasStorage(db *sql.DB, pageCountCache AliasPageCountCache) *AliasStorage {
	return &AliasStorage{
		db:             db,
		pageCountCache: pageCountCache,
	}
}

func (s *AliasStorage) Save(ctx context.Context, a domain.Alias) (err error) {
	const op = "AliasStorage.Save"
	const query = `
	INSERT INTO alias (id, chat_id, path, path_type, alias)
	VALUES
	($1, $2, $3, $4, $5)
	`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%v: beginning transaction: %w: %w", op, domain.ErrDb, err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
			if err != nil {
				err = fmt.Errorf("%v: committing transaction: %w: %w", op, domain.ErrDb, err)
			}
		}
	}()
	_, err = tx.ExecContext(
		ctx,
		query,
		a.ID,
		a.ChatID,
		a.Path.Value,
		a.Path.Type,
		a.Alias,
	)
	if err != nil {
		pqErr := new(pq.Error)
		if errors.As(err, &pqErr) && pqErr.Code == pqerror.UniqueViolation {
			return fmt.Errorf("%v: saving alias, unique violation: %w: %w: %w", op, domain.ErrAlreadyExists, domain.ErrDb, err)
		}
		return fmt.Errorf("%v: saving alias: %w: %w", op, domain.ErrDb, err)
	}
	if err := s.pageCountCache.Delete(ctx, a.ChatID, domain.DefaultPageSize); err != nil {
		log.Printf("Error while deleting alias cache: %v", err)
	}
	return nil
}

func (s *AliasStorage) AliasPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Alias], error) {
	const op = "AliasStorage.AliasPage"

	var zero domain.Page[domain.Alias]

	if pageNum < 0 || pageSize < 0 {
		return zero, domain.ErrBadArgument
	}
	if pageSize == 0 {
		return zero, nil
	}

	page := domain.Page[domain.Alias]{}
	page.CurPage = pageNum

	if totalPages, ok := s.pageCountCache.Get(ctx, chatID, pageSize); ok {
		page.TotalPages = totalPages
	} else {
		const countQuery = `
		SELECT COUNT(id)
		FROM alias
		WHERE chat_id = $1
		`
		var count int
		err := s.db.QueryRowContext(
			ctx,
			countQuery, chatID,
		).Scan(&count)
		if err != nil {
			return zero, fmt.Errorf("%v: counting aliases: %w: %w", op, domain.ErrDb, err)
		}
		page.TotalPages = count / pageSize
		if count%pageSize != 0 {
			page.TotalPages++
		}
		err = s.pageCountCache.Put(ctx, chatID, pageSize, page.TotalPages)
		if err != nil {
			log.Printf("error while saving alias cache: %v", err)
		}
	}
	offset := pageNum * pageSize
	limit := pageSize
	const query = `
	SELECT id, chat_id, path, path_type, alias
	FROM alias
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`
	rows, err := s.db.QueryContext(ctx, query, chatID, offset, limit)
	if err != nil {
		return zero, fmt.Errorf("%v: querying aliases page: %w: %w", op, domain.ErrDb, err)
	}
	templates := make([]domain.Alias, pageSize)
	cur := 0
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(
			&templates[cur].ID,
			&templates[cur].ChatID,
			&templates[cur].Path.Value,
			&templates[cur].Path.Type,
			&templates[cur].Alias,
		)
		if err != nil {
			return zero, fmt.Errorf("%v: scanning alias row: %w: %w", op, domain.ErrDb, err)
		}
		cur++
	}
	page.Values = templates[:cur]
	return page, nil
}

func (s *AliasStorage) Alias(ctx context.Context, id string, chatID int64) (domain.Alias, error) {
	const op = "AliasStorage.Alias"
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return domain.Alias{}, fmt.Errorf("%v: parsing alias id: %w: %w", op, domain.ErrBadArgument, err)
	}
	const query = `
	SELECT chat_id, path, path_type, alias
	FROM alias
	WHERE id = $1 AND chat_id = $2
	`
	var alias domain.Alias
	if err := s.db.QueryRowContext(ctx, query, id, chatID).Scan(&alias.ChatID, &alias.Path.Value, &alias.Path.Type, &alias.Alias); err != nil {
		return domain.Alias{}, fmt.Errorf("%v: querying alias: %w: %w", op, domain.ErrDb, err)
	}

	alias.ID = parsedID
	return alias, nil
}
