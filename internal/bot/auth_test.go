package bot

import (
	"context"
	"errors"
	"testing"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/mock"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/require"
)

type authServiceMock struct {
	client    domain.RemoteStorage
	clientErr error

	authURL    string
	authURLErr error

	clientCalled bool
	clientChatID int64

	generateAuthURLCalled bool
	generateAuthURLChatID int64
}

func (m *authServiceMock) Client(
	ctx context.Context,
	chatID int64,
) (domain.RemoteStorage, error) {
	m.clientCalled = true
	m.clientChatID = chatID

	return m.client, m.clientErr
}

func (m *authServiceMock) GenerateAuthURL(
	ctx context.Context,
	chatID int64,
) (string, error) {
	m.generateAuthURLCalled = true
	m.generateAuthURLChatID = chatID

	return m.authURL, m.authURLErr
}

func TestAuthHandler_Handle_Authenticated(t *testing.T) {
	authService := &authServiceMock{
		client: &mock.RemoteStorage{},
	}

	handler := NewAuthHandler(authService)

	resp, err := handler.Handle(
		context.Background(),
		bot.ChatSession{
			ChatID: 123,
		},
		bot.Update{
			ChatID: 123,
			Text:   "/start",
		},
	)

	require.NoError(t, err)

	require.True(t, authService.clientCalled)
	require.Equal(t, int64(123), authService.clientChatID)

	require.False(t, authService.generateAuthURLCalled)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
}

func TestAuthHandler_Handle_NotAuthenticated(t *testing.T) {
	authService := &authServiceMock{
		clientErr: errors.New("github client is not authorized"),
		authURL:   "https://github.com/login/oauth/authorize?state=abc",
	}

	handler := NewAuthHandler(authService)

	resp, err := handler.Handle(
		context.Background(),
		bot.ChatSession{
			ChatID: 123,
		},
		bot.Update{
			ChatID: 123,
			Text:   "/start",
		},
	)

	require.NoError(t, err)

	require.True(t, authService.clientCalled)
	require.Equal(t, int64(123), authService.clientChatID)

	require.True(t, authService.generateAuthURLCalled)
	require.Equal(t, int64(123), authService.generateAuthURLChatID)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Contains(
		t,
		msg.Text,
		"https://github.com/login/oauth/authorize?state=abc",
	)
}

func TestAuthHandler_Handle_GenerateAuthURLError(t *testing.T) {
	expectedErr := errors.New("failed to generate auth URL")

	authService := &authServiceMock{
		clientErr:  errors.New("not authenticated"),
		authURLErr: expectedErr,
	}

	handler := NewAuthHandler(authService)

	resp, err := handler.Handle(
		context.Background(),
		bot.ChatSession{
			ChatID: 123,
		},
		bot.Update{
			ChatID: 123,
			Text:   "/start",
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)

	require.True(t, authService.clientCalled)
	require.True(t, authService.generateAuthURLCalled)
}
