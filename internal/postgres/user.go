package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
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
	const query = `
	SELECT id
	FROM vault
	WHERE chat_id = $1
	`

	var id string
	err := u.db.QueryRowContext(ctx, query, chatID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (u *UserVaultStorage) Save(ctx context.Context, vault domain.UserVault) error {
	const query = `
	INSERT INTO vault (chat_id, owner, repo)
	VALUES ($1, $2, $3)
	ON CONFLICT (chat_id) DO UPDATE SET owner=$2, repo=$3
	`

	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		tx.Commit()
	}()
	if _, err := tx.ExecContext(ctx, query, vault.ChatID, vault.Owner, vault.Repo); err != nil {
		return err
	}
	return nil

}

func (u *UserVaultStorage) Vault(ctx context.Context, chatID int64) (domain.UserVault, error) {
	const query = `
	SELECT chat_id, owner, repo
	FROM vault
	WHERE chat_id = $1
	`
	var vault domain.UserVault

	if err := u.db.QueryRowContext(ctx, query, chatID).Scan(&vault); err != nil {
		return domain.UserVault{}, err
	}
	return vault, nil
}
