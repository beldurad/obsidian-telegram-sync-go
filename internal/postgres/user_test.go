package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/stretchr/testify/require"
)

func newUserVaultStorageTest(t *testing.T) (
	*sql.DB,
	sqlmock.Sqlmock,
	*UserVaultStorage,
) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	storage := NewUserVaultStorage(db)

	return db, mock, storage
}

func TestUserVaultStorage_ExistsByChatID_True(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id
	FROM vault
	WHERE chat_id = $1
	`)).
		WithArgs(int64(123)).
		WillReturnRows(
			sqlmock.NewRows([]string{"chat_id"}).
				AddRow(int64(123)),
		)

	exists, err := storage.ExistsByChatID(
		context.Background(),
		123,
	)

	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_ExistsByChatID_False(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id
	FROM vault
	WHERE chat_id = $1
	`)).
		WithArgs(int64(123)).
		WillReturnRows(
			sqlmock.NewRows([]string{"chat_id"}),
		)

	exists, err := storage.ExistsByChatID(
		context.Background(),
		123,
	)

	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_ExistsByChatID_QueryError(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	expectedErr := errors.New("query error")

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id
	FROM vault
	WHERE chat_id = $1
	`)).
		WithArgs(int64(123)).
		WillReturnError(expectedErr)

	exists, err := storage.ExistsByChatID(
		context.Background(),
		123,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)
	require.False(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_ExistsByChatID_ScanError(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id
	FROM vault
	WHERE chat_id = $1
	`)).
		WithArgs(int64(123)).
		WillReturnRows(
			sqlmock.NewRows([]string{"chat_id"}).
				AddRow("not-an-int64"),
		)

	exists, err := storage.ExistsByChatID(
		context.Background(),
		123,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.False(t, exists)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_Save_Success(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO vault (chat_id, owner, repo)
	VALUES ($1, $2, $3)
	ON CONFLICT (chat_id) DO UPDATE SET owner=$2, repo=$3
	`)).
		WithArgs(
			int64(123),
			"owner",
			"repo",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err := storage.Save(
		context.Background(),
		domain.UserVault{
			ChatID: 123,
			Owner:  "owner",
			Repo:   "repo",
		},
	)

	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_Save_BeginError(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	expectedErr := errors.New("begin error")

	mock.ExpectBegin().
		WillReturnError(expectedErr)

	err := storage.Save(
		context.Background(),
		domain.UserVault{
			ChatID: 123,
			Owner:  "owner",
			Repo:   "repo",
		},
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_Save_ExecError(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	expectedErr := errors.New("insert error")

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO vault (chat_id, owner, repo)
	VALUES ($1, $2, $3)
	ON CONFLICT (chat_id) DO UPDATE SET owner=$2, repo=$3
	`)).
		WithArgs(
			int64(123),
			"owner",
			"repo",
		).
		WillReturnError(expectedErr)

	mock.ExpectRollback()

	err := storage.Save(
		context.Background(),
		domain.UserVault{
			ChatID: 123,
			Owner:  "owner",
			Repo:   "repo",
		},
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_Save_CommitError(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	expectedErr := errors.New("commit error")

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO vault (chat_id, owner, repo)
	VALUES ($1, $2, $3)
	ON CONFLICT (chat_id) DO UPDATE SET owner=$2, repo=$3
	`)).
		WithArgs(
			int64(123),
			"owner",
			"repo",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit().
		WillReturnError(expectedErr)

	err := storage.Save(
		context.Background(),
		domain.UserVault{
			ChatID: 123,
			Owner:  "owner",
			Repo:   "repo",
		},
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_Vault_Success(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id, owner, repo
	FROM vault
	WHERE chat_id = $1
	`)).
		WithArgs(int64(123)).
		WillReturnRows(
			sqlmock.NewRows(
				[]string{"chat_id", "owner", "repo"},
			).
				AddRow(
					int64(123),
					"owner",
					"repo",
				),
		)

	result, err := storage.Vault(
		context.Background(),
		123,
	)

	require.NoError(t, err)

	require.Equal(t, domain.UserVault{
		ChatID: 123,
		Owner:  "owner",
		Repo:   "repo",
	}, result)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUserVaultStorage_Vault_QueryError(t *testing.T) {
	_, mock, storage := newUserVaultStorageTest(t)

	expectedErr := errors.New("query error")

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id, owner, repo
	FROM vault
	WHERE chat_id = $1
	`)).
		WithArgs(int64(123)).
		WillReturnError(expectedErr)

	result, err := storage.Vault(
		context.Background(),
		123,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.Equal(t, domain.UserVault{}, result)

	require.NoError(t, mock.ExpectationsWereMet())
}
