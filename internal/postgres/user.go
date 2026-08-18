package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
)

type UserVaultStorage struct {
	db *sql.DB
}

func NewUserVaultStorage(db *sql.DB) *UserVaultStorage {
	return &UserVaultStorage{
		db: db,
	}
}

func (u *UserVaultStorage) ExistsByChatID(ctx context.Context, chatID int64) (bool, error) {
	const op = "UserVaultStorage.ExistsByChatID"
	const query = `
	SELECT chat_id
	FROM vault
	WHERE chat_id = $1
	`
	err := u.db.QueryRowContext(ctx, query, chatID).Scan(&chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("%v: checking vault exists: %w: %w", op, domain.ErrDb, err)
	}
	return true, nil
}

func (u *UserVaultStorage) Save(ctx context.Context, vault domain.UserVault) (err error) {
	const op = "UserVaultStorage.Save"
	const query = `
	INSERT INTO vault (chat_id, owner, repo)
	VALUES ($1, $2, $3)
	ON CONFLICT (chat_id) DO UPDATE SET owner=$2, repo=$3
	`

	tx, err := u.db.BeginTx(ctx, nil)
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
	_, err = tx.ExecContext(ctx, query, vault.ChatID, vault.Owner, vault.Repo)
	if err != nil {
		pqErr := new(pq.Error)
		if errors.As(err, &pqErr) && pqErr.Code == pqerror.UniqueViolation {
			return fmt.Errorf("%v: saving vault, unique violation: %w: %w: %w", op, domain.ErrAlreadyExists, domain.ErrDb, err)
		}
		return fmt.Errorf("%v: saving vault: %w: %w", op, domain.ErrDb, err)
	}
	return nil

}

func (u *UserVaultStorage) Vault(ctx context.Context, chatID int64) (domain.UserVault, error) {
	const op = "UserVaultStorage.Vault"
	const query = `
	SELECT chat_id, owner, repo
	FROM vault
	WHERE chat_id = $1
	`
	var vault domain.UserVault

	if err := u.db.QueryRowContext(ctx, query, chatID).Scan(&vault.ChatID, &vault.Owner, &vault.Repo); err != nil {
		return domain.UserVault{}, fmt.Errorf("%v: querying vault: %w: %w", op, domain.ErrDb, err)
	}
	return vault, nil
}
