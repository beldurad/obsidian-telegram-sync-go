package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/google/uuid"
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
	const query = `
	INSERT INTO alias (id, chat_id, path, alias)
	VALUES
	($1, $2, $3, $4)
	`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w:%w", domain.ErrDb, err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
			if err != nil {
				err = fmt.Errorf("%w:%w", domain.ErrDb, err)
			}
		}
	}()
	_, err = tx.ExecContext(
		ctx,
		query,
		a.ID,
		a.ChatID,
		a.Path,
		a.Alias,
	)
	if err != nil {
		return fmt.Errorf("%w:%w", domain.ErrDb, err)
	}
	if err := s.pageCountCache.Delete(ctx, a.ChatID, domain.DefaultPageSize); err != nil {
		log.Printf("Error while deleting alias cache: %v", err)
	}
	return nil
}

func (s *AliasStorage) AliasPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Alias], error) {
	if pageNum < 0 || pageSize < 0 {
		return domain.Page[domain.Alias]{}, domain.ErrBadArgument
	}
	if pageSize == 0 {
		return domain.Page[domain.Alias]{}, nil
	}
	page := domain.Page[domain.Alias]{}
	page.CurPage = pageNum
	if pageNum, ok := s.pageCountCache.Get(ctx, chatID, pageSize); ok {
		page.TotalPages = pageNum
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
			return domain.Page[domain.Alias]{}, fmt.Errorf("%w:%w", domain.ErrDb, err)
		}
		page.TotalPages = int(
			math.Ceil(
				float64(count) / float64(pageSize),
			),
		)
		err = s.pageCountCache.Put(ctx, chatID, pageSize, page.TotalPages)
		if err != nil {
			log.Printf("error while saving alias cache: %v", err)
		}
	}
	offset := pageNum * pageSize
	limit := pageSize
	const query = `
	SELECT id, chat_id, path, alias
	FROM alias
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`
	rows, err := s.db.QueryContext(ctx, query, chatID, offset, limit)
	if err != nil {
		return domain.Page[domain.Alias]{}, fmt.Errorf("%w:%w", domain.ErrDb, err)
	}
	templates := make([]domain.Alias, pageSize)
	cur := 0
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(
			&templates[cur].ID,
			&templates[cur].ChatID,
			&templates[cur].Path,
			&templates[cur].Alias,
		)
		if err != nil {
			return domain.Page[domain.Alias]{}, fmt.Errorf("%w:%w", domain.ErrDb, err)
		}
		cur++
	}
	page.Values = templates[:cur]
	return page, nil
}

func (s *AliasStorage) Alias(ctx context.Context, id string, chatID int64) (domain.Alias, error) {
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return domain.Alias{}, fmt.Errorf("%w:%w", domain.ErrBadArgument, err)
	}
	const query = `
	SELECT chat_id, path, alias
	FROM alias
	WHERE id = $1 AND chat_id = $2
	`
	var alias domain.Alias
	if err := s.db.QueryRowContext(ctx, query, id, chatID).Scan(&alias.ChatID, &alias.Path, &alias.Alias); err != nil {
		return domain.Alias{}, fmt.Errorf("%w:%w", domain.ErrDb, err)
	}

	alias.ID = parsedID
	return alias, nil
}
