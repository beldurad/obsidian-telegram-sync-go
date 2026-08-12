package postgres

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type aliasPageCountCacheMock struct {
	getFn func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) (int, bool)

	putFn func(
		ctx context.Context,
		chatID int64,
		pageSize int,
		count int,
	) error

	deleteFn func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) error

	getCalled    bool
	getChatID    int64
	getPageSize  int
	putCalled    bool
	putChatID    int64
	putPageSize  int
	putCount     int
	deleteCalled bool
	deleteChatID int64
	deleteSize   int
}

func (m *aliasPageCountCacheMock) Get(
	ctx context.Context,
	chatID int64,
	pageSize int,
) (int, bool) {
	m.getCalled = true
	m.getChatID = chatID
	m.getPageSize = pageSize

	if m.getFn != nil {
		return m.getFn(ctx, chatID, pageSize)
	}

	return 0, false
}

func (m *aliasPageCountCacheMock) Put(
	ctx context.Context,
	chatID int64,
	pageSize int,
	count int,
) error {
	m.putCalled = true
	m.putChatID = chatID
	m.putPageSize = pageSize
	m.putCount = count

	if m.putFn != nil {
		return m.putFn(ctx, chatID, pageSize, count)
	}

	return nil
}

func (m *aliasPageCountCacheMock) Delete(
	ctx context.Context,
	chatID int64,
	pageSize int,
) error {
	m.deleteCalled = true
	m.deleteChatID = chatID
	m.deleteSize = pageSize

	if m.deleteFn != nil {
		return m.deleteFn(ctx, chatID, pageSize)
	}

	return nil
}

func newAliasStorageTest(t *testing.T) (
	*sql.DB,
	sqlmock.Sqlmock,
	*aliasPageCountCacheMock,
	*AliasStorage,
) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	cache := &aliasPageCountCacheMock{}
	storage := NewAliasStorage(db, cache)

	return db, mock, cache, storage
}

func TestAliasStorage_Save_Success(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	id := uuid.New()

	alias := domain.Alias{
		ID:     id,
		ChatID: 123,
		Path:   "/notes/test.md",
		Alias:  "test",
	}

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO alias (id, chat_id, path, alias)
	VALUES
	($1, $2, $3, $4)
	`)).
		WithArgs(
			id,
			int64(123),
			"/notes/test.md",
			"test",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err := storage.Save(context.Background(), alias)

	require.NoError(t, err)

	require.True(t, cache.deleteCalled)
	require.Equal(t, int64(123), cache.deleteChatID)
	require.Equal(t, domain.DefaultPageSize, cache.deleteSize)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Save_BeginError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	expectedErr := errors.New("begin error")

	mock.ExpectBegin().
		WillReturnError(expectedErr)

	err := storage.Save(
		context.Background(),
		domain.Alias{
			ID:     uuid.New(),
			ChatID: 123,
			Path:   "/test",
			Alias:  "test",
		},
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.False(t, cache.deleteCalled)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Save_ExecError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	id := uuid.New()
	expectedErr := errors.New("insert error")

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO alias (id, chat_id, path, alias)
	VALUES
	($1, $2, $3, $4)
	`)).
		WithArgs(
			id,
			int64(123),
			"/test",
			"test",
		).
		WillReturnError(expectedErr)

	mock.ExpectRollback()

	err := storage.Save(
		context.Background(),
		domain.Alias{
			ID:     id,
			ChatID: 123,
			Path:   "/test",
			Alias:  "test",
		},
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.False(t, cache.deleteCalled)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Save_CommitError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	id := uuid.New()
	expectedErr := errors.New("commit error")

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO alias (id, chat_id, path, alias)
	VALUES
	($1, $2, $3, $4)
	`)).
		WithArgs(
			id,
			int64(123),
			"/test",
			"test",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit().
		WillReturnError(expectedErr)

	err := storage.Save(
		context.Background(),
		domain.Alias{
			ID:     id,
			ChatID: 123,
			Path:   "/test",
			Alias:  "test",
		},
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.True(t, cache.deleteCalled)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Save_CacheDeleteError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	id := uuid.New()
	cacheErr := errors.New("cache delete error")

	cache.deleteFn = func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) error {
		return cacheErr
	}

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(`
	INSERT INTO alias (id, chat_id, path, alias)
	VALUES
	($1, $2, $3, $4)
	`)).
		WithArgs(
			id,
			int64(123),
			"/test",
			"test",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err := storage.Save(
		context.Background(),
		domain.Alias{
			ID:     id,
			ChatID: 123,
			Path:   "/test",
			Alias:  "test",
		},
	)

	require.NoError(t, err)
	require.True(t, cache.deleteCalled)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_NegativePage(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		-1,
		domain.DefaultPageSize,
	)

	require.ErrorIs(t, err, domain.ErrBadArgument)
	require.Equal(t, domain.Page[domain.Alias]{}, page)

	require.False(t, cache.getCalled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_NegativePageSize(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		0,
		-1,
	)

	require.ErrorIs(t, err, domain.ErrBadArgument)
	require.Equal(t, domain.Page[domain.Alias]{}, page)

	require.False(t, cache.getCalled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_CacheHit(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	id1 := uuid.New()
	id2 := uuid.New()

	cache.getFn = func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) (int, bool) {
		require.Equal(t, int64(123), chatID)
		require.Equal(t, domain.DefaultPageSize, pageSize)

		return 3, true
	}

	rows := sqlmock.NewRows(
		[]string{"id", "chat_id", "path", "alias"},
	).
		AddRow(
			id1,
			int64(123),
			"/a.md",
			"a",
		).
		AddRow(
			id2,
			int64(123),
			"/b.md",
			"b",
		)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT id, chat_id, path, alias
	FROM alias
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`)).
		WithArgs(
			int64(123),
			domain.DefaultPageSize,
			domain.DefaultPageSize,
		).
		WillReturnRows(rows)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		1,
		domain.DefaultPageSize,
	)

	require.NoError(t, err)

	require.Equal(t, 1, page.CurPage)
	require.Equal(t, 3, page.TotalPages)

	require.Len(t, page.Values, 2)

	require.Equal(t, domain.Alias{
		ID:     id1,
		ChatID: 123,
		Path:   "/a.md",
		Alias:  "a",
	}, page.Values[0])

	require.Equal(t, domain.Alias{
		ID:     id2,
		ChatID: 123,
		Path:   "/b.md",
		Alias:  "b",
	}, page.Values[1])

	require.False(t, cache.putCalled)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_CacheMiss(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	id := uuid.New()

	cache.getFn = func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) (int, bool) {
		return 0, false
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(id)
		FROM alias
		WHERE chat_id = $1
		`)).
		WithArgs(int64(123)).
		WillReturnRows(
			sqlmock.NewRows([]string{"count"}).
				AddRow(25),
		)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT id, chat_id, path, alias
	FROM alias
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`)).
		WithArgs(
			int64(123),
			0,
			domain.DefaultPageSize,
		).
		WillReturnRows(
			sqlmock.NewRows(
				[]string{"id", "chat_id", "path", "alias"},
			).
				AddRow(
					id,
					int64(123),
					"/test.md",
					"test",
				),
		)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		0,
		domain.DefaultPageSize,
	)

	require.NoError(t, err)

	require.Equal(t, 0, page.CurPage)
	require.Equal(
		t,
		int(math.Ceil(
			float64(25)/float64(domain.DefaultPageSize),
		)),
		page.TotalPages,
	)

	require.Len(t, page.Values, 1)
	require.Equal(t, id, page.Values[0].ID)

	require.True(t, cache.putCalled)
	require.Equal(t, int64(123), cache.putChatID)
	require.Equal(t, domain.DefaultPageSize, cache.putPageSize)
	require.Equal(
		t,
		int(math.Ceil(
			float64(25)/float64(domain.DefaultPageSize),
		)),
		cache.putCount,
	)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_CountError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	expectedErr := errors.New("count error")

	cache.getFn = func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) (int, bool) {
		return 0, false
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(id)
		FROM alias
		WHERE chat_id = $1
		`)).
		WithArgs(int64(123)).
		WillReturnError(expectedErr)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		0,
		domain.DefaultPageSize,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.Equal(t, domain.Page[domain.Alias]{}, page)
	require.False(t, cache.putCalled)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_CachePutError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	cache.putFn = func(
		ctx context.Context,
		chatID int64,
		pageSize int,
		count int,
	) error {
		return errors.New("cache put error")
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(id)
		FROM alias
		WHERE chat_id = $1
		`)).
		WithArgs(int64(123)).
		WillReturnRows(
			sqlmock.NewRows([]string{"count"}).
				AddRow(5),
		)

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT id, chat_id, path, alias
	FROM alias
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`)).
		WithArgs(
			int64(123),
			0,
			domain.DefaultPageSize,
		).
		WillReturnRows(
			sqlmock.NewRows(
				[]string{"id", "chat_id", "path", "alias"},
			),
		)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		0,
		domain.DefaultPageSize,
	)

	require.NoError(t, err)
	require.True(t, cache.putCalled)

	require.Equal(t, 1, page.TotalPages)
	require.Empty(t, page.Values)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_AliasPage_QueryError(t *testing.T) {
	_, mock, cache, storage := newAliasStorageTest(t)

	expectedErr := errors.New("query error")

	cache.getFn = func(
		ctx context.Context,
		chatID int64,
		pageSize int,
	) (int, bool) {
		return 3, true
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT id, chat_id, path, alias
	FROM alias
	WHERE chat_id = $1
	OFFSET $2
	LIMIT $3
	`)).
		WithArgs(
			int64(123),
			0,
			domain.DefaultPageSize,
		).
		WillReturnError(expectedErr)

	page, err := storage.AliasPage(
		context.Background(),
		123,
		0,
		domain.DefaultPageSize,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.Equal(t, domain.Page[domain.Alias]{}, page)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Alias_Success(t *testing.T) {
	_, mock, _, storage := newAliasStorageTest(t)

	id := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id, path, alias
	FROM alias
	WHERE id = $1 AND chat_id = $2
	`)).
		WithArgs(
			id.String(),
			int64(123),
		).
		WillReturnRows(
			sqlmock.NewRows(
				[]string{"chat_id", "path", "alias"},
			).
				AddRow(
					int64(123),
					"/notes/test.md",
					"test",
				),
		)

	result, err := storage.Alias(
		context.Background(),
		id.String(),
		123,
	)

	require.NoError(t, err)

	require.Equal(t, domain.Alias{
		ID:     id,
		ChatID: 123,
		Path:   "/notes/test.md",
		Alias:  "test",
	}, result)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Alias_InvalidUUID(t *testing.T) {
	_, mock, _, storage := newAliasStorageTest(t)

	result, err := storage.Alias(
		context.Background(),
		"not-a-uuid",
		123,
	)

	require.ErrorIs(t, err, domain.ErrBadArgument)
	require.Equal(t, domain.Alias{}, result)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Alias_QueryError(t *testing.T) {
	_, mock, _, storage := newAliasStorageTest(t)

	id := uuid.New()
	expectedErr := errors.New("query error")

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id, path, alias
	FROM alias
	WHERE id = $1 AND chat_id = $2
	`)).
		WithArgs(
			id.String(),
			int64(123),
		).
		WillReturnError(expectedErr)

	result, err := storage.Alias(
		context.Background(),
		id.String(),
		123,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.ErrorIs(t, err, expectedErr)

	require.Equal(t, domain.Alias{}, result)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAliasStorage_Alias_ScanError(t *testing.T) {
	_, mock, _, storage := newAliasStorageTest(t)

	id := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`
	SELECT chat_id, path, alias
	FROM alias
	WHERE id = $1 AND chat_id = $2
	`)).
		WithArgs(
			id.String(),
			int64(123),
		).
		WillReturnRows(
			sqlmock.NewRows(
				[]string{"chat_id", "path", "alias"},
			).
				AddRow(
					"not-an-int64",
					"/test.md",
					"test",
				),
		)

	result, err := storage.Alias(
		context.Background(),
		id.String(),
		123,
	)

	require.ErrorIs(t, err, domain.ErrDb)
	require.Equal(t, domain.Alias{}, result)

	require.NoError(t, mock.ExpectationsWereMet())
}
