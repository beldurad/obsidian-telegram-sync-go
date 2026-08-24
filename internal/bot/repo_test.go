package bot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/mock"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/require"
)

type userVaultServiceMock struct {
	saveFn func(domain.UserVault) error

	existsByChatIDFn func(chatID int64) (bool, error)

	saveCalled bool
	savedVault domain.UserVault

	existsCalled bool
	existsChatID int64
}

func (m *userVaultServiceMock) Save(
	ctx context.Context,
	vault domain.UserVault,
) error {
	m.saveCalled = true
	m.savedVault = vault

	if m.saveFn != nil {
		return m.saveFn(vault)
	}

	return nil
}

func (m *userVaultServiceMock) ExistsByChatID(
	ctx context.Context,
	chatID int64,
) (bool, error) {
	m.existsCalled = true
	m.existsChatID = chatID

	if m.existsByChatIDFn != nil {
		return m.existsByChatIDFn(chatID)
	}

	return false, nil
}

func TestRepoSetHandler_Handle_ClientError(t *testing.T) {
	expectedErr := errors.New("client error")

	clientGetter := &mock.RemoteStorageGetter{
		Err: expectedErr,
	}

	handler := NewRepoSetHandler(
		clientGetter,
		&userVaultServiceMock{},
	)

	session := bot.NewChatSession(123)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)

	require.True(t, clientGetter.Called)
	require.Equal(t, int64(123), clientGetter.ChatID)
}

func TestRepoSetHandler_Handle_UserInfoError(t *testing.T) {
	expectedErr := errors.New("user info error")

	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{}, expectedErr
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&userVaultServiceMock{},
	)

	session := bot.NewChatSession(123)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
	require.True(t, client.UserInfoCalled)
}

func TestRepoSetHandler_Handle_DefaultState(t *testing.T) {
	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
		UserReposFn: func(
			username string,
			pageNum int,
			pageSize int,
		) (domain.Page[domain.RemoteRepo], error) {
			return domain.Page[domain.RemoteRepo]{
				Values: []domain.RemoteRepo{
					{Name: "repo-1"},
					{Name: "repo-2"},
				},
			}, nil
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&userVaultServiceMock{},
	)

	session := bot.NewChatSession(123)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{},
	)

	require.NoError(t, err)

	require.True(t, client.UserInfoCalled)
	require.True(t, client.UserReposCalled)

	require.Equal(t, "john", client.UserReposUsername)
	require.Equal(t, 0, client.UserReposPage)
	require.Equal(t, domain.DefaultPageSize, client.UserReposSize)

	require.Equal(t, RepoSetState, session.State())

	var payload pagePayload
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, 0, payload.PageNum)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
}

func TestRepoSetHandler_Handle_NextPage(t *testing.T) {
	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&userVaultServiceMock{},
	)

	raw, err := json.Marshal(pagePayload{
		PageNum:    2,
		TotalPages: 5,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(RepoSetState)
	session.SetPayload(raw)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			CallbackData: NextPageCallback,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCallback,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 3, client.UserReposPage)

	var payload pagePayload
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, 3, payload.PageNum)
	_ = resp
}

func TestRepoSetHandler_Handle_PrevPage(t *testing.T) {
	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&userVaultServiceMock{},
	)

	raw, err := json.Marshal(pagePayload{
		PageNum: 2,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(RepoSetState)
	session.SetPayload(raw)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			CallbackData: PrevPageCallback,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: PrevPageCallback,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 1, client.UserReposPage)

	var payload pagePayload
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, 1, payload.PageNum)
	_ = resp
}

func TestRepoSetHandler_Handle_ChosenRepo_Success(t *testing.T) {
	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
		RepoExistsFn: func(owner, repo string) (bool, error) {
			require.Equal(t, "john", owner)
			require.Equal(t, "my-repo", repo)

			return true, nil
		},
	}

	vaultService := &userVaultServiceMock{}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		vaultService,
	)

	session := bot.NewChatSession(123)
	session.SetState(RepoSetState)

	ctx := context.Background()

	resp, err := handler.Handle(
		ctx,
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: "my-repo",
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: "my-repo",
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, client.RepoExistsCalled)
	require.Equal(t, "john", client.RepoExistsOwner)
	require.Equal(t, "my-repo", client.RepoExistsRepo)

	require.True(t, vaultService.saveCalled)

	require.Equal(t, domain.UserVault{
		ChatID: 123,
		Owner:  "john",
		Repo:   "my-repo",
	}, vaultService.savedVault)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Репозиторий успешно установлен", msg.Text)
}

func TestRepoSetHandler_Handle_ChosenRepo_NotFound(t *testing.T) {
	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
		RepoExistsFn: func(owner, repo string) (bool, error) {
			return false, nil
		},
	}

	vaultService := &userVaultServiceMock{}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		vaultService,
	)

	session := bot.NewChatSession(123)
	session.SetState(RepoSetState)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: "missing-repo",
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: "missing-repo",
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, client.RepoExistsCalled)
	require.False(t, vaultService.saveCalled)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(
		t,
		"Такого репозитория не существует попробуйте еще раз командой /set-repo",
		msg.Text,
	)
}

func TestRepoSetHandler_Handle_ChosenRepo_RepoExistsError(t *testing.T) {
	expectedErr := errors.New("github error")

	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
		RepoExistsFn: func(owner, repo string) (bool, error) {
			return false, expectedErr
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&userVaultServiceMock{},
	)

	session := bot.NewChatSession(123)
	session.SetState(RepoSetState)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: "repo",
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: "repo",
				},
			},
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestRepoSetHandler_Handle_ChosenRepo_SaveError(t *testing.T) {
	expectedErr := errors.New("database error")

	client := &mock.RemoteStorage{
		UserInfoFn: func() (domain.RemoteUser, error) {
			return domain.RemoteUser{
				Username: "john",
			}, nil
		},
		RepoExistsFn: func(owner, repo string) (bool, error) {
			return true, nil
		},
	}

	vaultService := &userVaultServiceMock{
		saveFn: func(vault domain.UserVault) error {
			return expectedErr
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		vaultService,
	)

	session := bot.NewChatSession(123)
	session.SetState(RepoSetState)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: "repo",
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: "repo",
				},
			},
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)

	require.True(t, vaultService.saveCalled)
}

func TestRepoSetMiddleware_Exists(t *testing.T) {
	vaultService := &userVaultServiceMock{
		existsByChatIDFn: func(chatID int64) (bool, error) {
			return true, nil
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{},
		vaultService,
	)

	nextCalled := false

	next := bot.HandlerFunc{
		HandleFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
			nextCalled = true

			return bot.Response{
				Message: tgbotapi.NewMessage(u.ChatID, "next"),
			}, nil
		},
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return true
		},
	}

	middleware := handler.RepoSetMiddleware()
	wrapped := middleware(next)

	session := bot.NewChatSession(123)

	resp, err := wrapped.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
		},
	)

	require.NoError(t, err)

	require.True(t, vaultService.existsCalled)
	require.Equal(t, int64(123), vaultService.existsChatID)

	require.True(t, nextCalled)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)
	require.Equal(t, "next", msg.Text)
}

func TestRepoSetMiddleware_NotExists(t *testing.T) {
	vaultService := &userVaultServiceMock{
		existsByChatIDFn: func(chatID int64) (bool, error) {
			return false, nil
		},
	}

	handler := NewRepoSetHandler(
		&mock.RemoteStorageGetter{},
		vaultService,
	)

	nextCalled := false

	next := bot.HandlerFunc{
		HandleFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
			nextCalled = true
			return bot.Response{}, nil
		},
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return true
		},
	}

	wrapped := handler.RepoSetMiddleware()(next)

	session := bot.NewChatSession(123)

	resp, err := wrapped.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
		},
	)

	require.NoError(t, err)

	require.False(t, nextCalled)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(
		t,
		"Установите репозиторий в котором находится ваще хранилище через /set-repo",
		msg.Text,
	)
}
