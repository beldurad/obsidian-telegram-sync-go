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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type aliasesGetterMock struct {
	called   bool
	chatID   int64
	pageNum  int
	pageSize int

	page domain.Page[domain.Alias]
	err  error
}

func (m *aliasesGetterMock) AliasPage(
	ctx context.Context,
	chatID int64,
	pageNum,
	pageSize int,
) (domain.Page[domain.Alias], error) {
	m.called = true
	m.chatID = chatID
	m.pageNum = pageNum
	m.pageSize = pageSize

	return m.page, m.err
}

func TestGetAliasesHandler_Handle_FirstPage(t *testing.T) {
	getter := &aliasesGetterMock{
		page: domain.Page[domain.Alias]{
			Values: []domain.Alias{
				{
					Path:  "/foo",
					Alias: "foo",
				},
				{
					Path:  "/bar",
					Alias: "bar",
				},
			},
			TotalPages: 1,
		},
	}

	h := NewGetAliasesHandler(getter)

	session := bot.ChatSession{
		ChatID: 123,
		State:  bot.DefaultChatState,
	}

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		session,
	)

	resp, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   "/alias",
	})

	require.NoError(t, err)

	require.True(t, getter.called)
	require.Equal(t, int64(123), getter.chatID)
	require.Equal(t, 0, getter.pageNum)
	require.Equal(t, domain.DefaultPageSize, getter.pageSize)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateGetAlias, *resp.NewChatState)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&payload,
	))

	require.Equal(t, 0, payload.PageNum)
}

func TestGetAliasesHandler_Handle_FirstPage_SendsNewMessage(t *testing.T) {
	getter := &aliasesGetterMock{
		page: domain.Page[domain.Alias]{
			Values: []domain.Alias{
				{
					Path:  "/foo",
					Alias: "foo",
				},
				{
					Path:  "/bar",
					Alias: "bar",
				},
			},
			TotalPages: 1,
		},
	}

	h := NewGetAliasesHandler(getter)

	session := bot.ChatSession{
		ChatID: 123,
		State:  bot.DefaultChatState,
	}

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		session,
	)

	resp, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   "/alias",
	})

	require.NoError(t, err)

	_, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)
}

func TestGetAliasesHandler_Handle_NextPage(t *testing.T) {
	getter := &aliasesGetterMock{
		page: domain.Page[domain.Alias]{
			Values:     []domain.Alias{{Path: "/foo", Alias: "foo"}},
			TotalPages: 3,
		},
	}

	h := NewGetAliasesHandler(getter)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:           123,
		State:            StateGetAlias,
		LastBotMessageID: 42,
		Payload:          rawPayload,
	}

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		session,
	)

	resp, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   NextPageCommand,
	})

	require.NoError(t, err)

	require.Equal(t, 1, getter.pageNum)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&payload,
	))

	require.Equal(t, 1, payload.PageNum)
}

func TestGetAliasesHandler_Handle_PrevPage_DoesNotGoBelowZero(t *testing.T) {
	getter := &aliasesGetterMock{}

	h := NewGetAliasesHandler(getter)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)
	session := bot.ChatSession{
		ChatID:  123,
		State:   StateGetAlias,
		Payload: rawPayload,
	}

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		session,
	)

	_, err = h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   PrevPageCommand,
	})

	require.NoError(t, err)
	require.Equal(t, 0, getter.pageNum)
}

func TestGetAliasesHandler_Handle_InvalidPayload(t *testing.T) {
	getter := &aliasesGetterMock{}
	h := NewGetAliasesHandler(getter)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateGetAlias,
		Payload: []byte(`not-json`),
	}

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		session,
	)

	_, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   NextPageCommand,
	})

	require.Error(t, err)
	require.False(t, getter.called)
}

func TestGetAliasesHandler_Handle_GetterError(t *testing.T) {
	expectedErr := errors.New("database unavailable")

	getter := &aliasesGetterMock{
		err: expectedErr,
	}

	h := NewGetAliasesHandler(getter)

	session := bot.ChatSession{
		ChatID: 123,
		State:  bot.DefaultChatState,
	}

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		session,
	)

	_, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   string(CommandGetAliases),
	})

	require.ErrorIs(t, err, expectedErr)
}

func TestGetAliasesHandler_Handle_PaginationButtons(t *testing.T) {
	tests := []struct {
		name        string
		prevPageNum int
		curPageNum  int
		totalPages  int
		command     string
		wantNext    bool
		wantPrev    bool
	}{
		{
			name:        "first page",
			prevPageNum: 1,
			curPageNum:  0,
			totalPages:  3,
			command:     PrevPageCommand,
			wantNext:    true,
			wantPrev:    false,
		},
		{
			name:        "middle page",
			prevPageNum: 0,
			curPageNum:  1,
			totalPages:  3,
			command:     NextPageCommand,
			wantNext:    true,
			wantPrev:    true,
		},
		{
			name:        "last page",
			prevPageNum: 1,
			curPageNum:  2,
			totalPages:  3,
			command:     NextPageCommand,
			wantNext:    false,
			wantPrev:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getter := &aliasesGetterMock{}
			getter.page = domain.Page[domain.Alias]{
				TotalPages: tt.totalPages,
				CurPage:    tt.curPageNum,
			}
			values := []domain.Alias{
				{
					ID:     uuid.New(),
					ChatID: 123,
					Path:   "path",
					Alias:  "alias",
				},
			}
			getter.page.Values = values
			rawPayload, err := json.Marshal(pagePayload{
				PageNum: tt.prevPageNum,
			})
			require.NoError(t, err)
			session := bot.ChatSession{
				ChatID:  123,
				State:   StateGetAlias,
				Payload: rawPayload,
			}

			ctx := context.WithValue(
				context.Background(),
				bot.ChatSessionKey,
				session,
			)

			h := NewGetAliasesHandler(getter)

			resp, err := h.Handle(ctx, bot.Update{
				ChatID: session.ChatID,
				Text:   tt.command,
			})
			require.NoError(t, err)

			keyboard := extractInlineKeyboard(resp.Message).InlineKeyboard
			var hasNext, hasPrev bool
			for i := range keyboard {
				for j := range keyboard[i] {
					switch *keyboard[i][j].CallbackData {
					case NextPageCommand:
						hasNext = true
					case PrevPageCommand:
						hasPrev = true
					}
				}
			}
			if tt.wantNext {
				assert.True(t, hasNext, `should have "Next" button`)
			}
			if tt.wantPrev {
				assert.True(t, hasPrev, `should have "Prev" button`)
			}
		})
	}
}

func extractInlineKeyboard(c tgbotapi.Chattable) tgbotapi.InlineKeyboardMarkup {
	switch msg := c.(type) {
	case tgbotapi.MessageConfig:
		return msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	case tgbotapi.EditMessageTextConfig:
		return *msg.ReplyMarkup
	default:
		panic("cannot extract inline keyboard")
	}
}

func TestAliasBuilder_ToAlias(t *testing.T) {
	id := uuid.New()

	builder := aliasBuilder{
		ID:     id.String(),
		ChatID: 123,
		Path:   "/foo/bar",
		Alias:  "my-file",
		Type:   domain.TypeFile,
	}

	got, err := builder.toAlias()

	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Equal(t, int64(123), got.ChatID)
	require.Equal(t, "/foo/bar", got.Path)
	require.Equal(t, "my-file", got.Alias)
}

func TestAliasBuilder_ToAlias_InvalidID(t *testing.T) {
	builder := aliasBuilder{
		ID:     "invalid",
		ChatID: 123,
	}

	_, err := builder.toAlias()

	require.Error(t, err)
}

func TestNewAliasBuilder(t *testing.T) {
	builder := newAliasBuilder(123)

	require.NotEmpty(t, builder.ID)
	require.Equal(t, int64(123), builder.ChatID)
	require.Equal(t, domain.TypeFile, builder.Type)

	_, err := uuid.Parse(builder.ID)
	require.NoError(t, err)
}

func TestPathButtons(t *testing.T) {
	tests := []struct {
		name     string
		dirElems domain.Page[domain.DirElem]
		payload  pathChoosingPayload
		expected tgbotapi.InlineKeyboardMarkup
	}{
		{
			name: "root directory",
			payload: pathChoosingPayload{
				CurPath: "",
			},
			dirElems: domain.Page[domain.DirElem]{
				Values: []domain.DirElem{
					{
						Type: "DIR",
						Name: "home",
					},
				},
			},
			expected: tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
					{
						tgbotapi.NewInlineKeyboardButtonData(
							"DIR: home",
							"/home",
						),
					},
					{
						tgbotapi.NewInlineKeyboardButtonData(
							"Выбрать текущую директорию как путь",
							string(currentDirCommand),
						),
					},
				},
			},
		},
		{
			name: "non-root directory",
			payload: pathChoosingPayload{
				CurPath: "/home/user",
			},
			dirElems: domain.Page[domain.DirElem]{
				Values: []domain.DirElem{
					{
						Type: "DIR",
						Name: "documents",
					},
				},
			},
			expected: tgbotapi.InlineKeyboardMarkup{
				InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
					{
						tgbotapi.NewInlineKeyboardButtonData(
							"..",
							string(prevPathCommand),
						),
					},
					{
						tgbotapi.NewInlineKeyboardButtonData(
							"DIR: documents",
							"/documents",
						),
					},
					{
						tgbotapi.NewInlineKeyboardButtonData(
							"Выбрать текущую директорию как путь",
							string(currentDirCommand),
						),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := pathButtons(tt.dirElems, tt.payload)

			assert.Equal(t, tt.expected, actual)
		})
	}
}

type vaultGetterMock struct {
	vault domain.UserVault
	err   error

	called bool
	chatID int64
}

func (m *vaultGetterMock) Vault(
	ctx context.Context,
	chatID int64,
) (domain.UserVault, error) {
	m.called = true
	m.chatID = chatID

	return m.vault, m.err
}

type aliasSaverMock struct {
	err error

	called bool
	alias  domain.Alias
}

func (m *aliasSaverMock) Save(
	ctx context.Context,
	alias domain.Alias,
) error {
	m.called = true
	m.alias = alias

	return m.err
}

func TestAddAliasHandler_Handle_DefaultState(t *testing.T) {
	client := &mock.RemoteStorage{
		DirectoryFn: func(
			owner, repo, path string,
			pageNum, pageSize int,
		) (domain.Page[domain.DirElem], error) {
			return domain.Page[domain.DirElem]{}, nil
		},
	}

	vault := &vaultGetterMock{
		vault: domain.UserVault{
			Owner: "test-owner",
			Repo:  "test-repo",
		},
	}

	h := NewAddAliasHandler(
		vault,
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&aliasSaverMock{},
	)

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		bot.ChatSession{
			ChatID: 123,
			State:  bot.DefaultChatState,
		})

	resp, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   "foo",
	})

	require.NoError(t, err)

	require.True(t, vault.called)
	require.Equal(t, int64(123), vault.chatID)

	require.True(t, client.DirectoryCalled)
	require.Equal(t, "test-owner", client.DirectoryOwner)
	require.Equal(t, "test-repo", client.DirectoryRepo)
	require.Equal(t, "/foo", client.DirectoryPath)
	require.Equal(t, 0, client.DirectoryPage)
	require.Equal(t, domain.DefaultPageSize, client.DirectorySize)

	require.Equal(t, resp.NewChatState, &StateWaitPath)
	require.NotEmpty(t, resp.NewPayload)

	_, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)
}

func TestAddAliasHandler_Handle_WaitPath_UsesPayload(t *testing.T) {
	client := &mock.RemoteStorage{}

	vault := &vaultGetterMock{
		vault: domain.UserVault{
			Owner: "owner",
			Repo:  "repo",
		},
	}

	h := NewAddAliasHandler(
		vault,
		&mock.RemoteStorageGetter{Storage: client},
		&aliasSaverMock{},
	)

	payload := pathChoosingPayload{
		CurPath: "/foo",
		PageNum: 2,
	}

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	ctx := context.WithValue(
		context.Background(),
		bot.ChatSessionKey,
		bot.ChatSession{
			ChatID:  123,
			State:   StateWaitPath,
			Payload: raw,
		})

	resp, err := h.Handle(ctx, bot.Update{
		ChatID: 123,
		Text:   "bar",
	})

	require.NoError(t, err)

	require.Equal(t, "/foo/bar", client.DirectoryPath)
	require.Equal(t, 0, client.DirectoryPage)

	var gotPayload pathChoosingPayload
	require.NoError(
		t,
		json.Unmarshal([]byte(resp.NewPayload), &gotPayload),
	)

	require.Equal(t, "/foo/bar", gotPayload.CurPath)
	require.Equal(t, 0, gotPayload.PageNum)
}

func TestAddAliasHandler_Handle_SelectCurrentDirectory(t *testing.T) {
	client := &mock.RemoteStorage{}

	h := NewAddAliasHandler(
		&vaultGetterMock{
			vault: domain.UserVault{
				Owner: "owner",
				Repo:  "repo",
			},
		},
		&mock.RemoteStorageGetter{
			Storage: client,
		},
		&aliasSaverMock{},
	)

	raw, err := json.Marshal(pathChoosingPayload{
		CurPath: "/foo/bar",
		PageNum: 3,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:           123,
		State:            StateWaitPath,
		LastBotMessageID: 456,
		Payload:          raw,
	}

	resp, err := h.Handle(
		context.WithValue(
			context.Background(),
			bot.ChatSessionKey,
			session,
		),
		bot.Update{
			ChatID: 123,
			Text:   string(currentDirCommand),
		},
	)

	require.NoError(t, err)

	require.False(t, client.DirectoryCalled)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateWaitAlias, *resp.NewChatState)

	var payload aliasBuilder
	require.NoError(
		t,
		json.Unmarshal([]byte(resp.NewPayload), &payload),
	)

	require.Equal(t, int64(123), payload.ChatID)
	require.Equal(t, "/foo/bar", payload.Path)
	require.Equal(t, domain.TypeDir, payload.Type)
	require.NotEmpty(t, payload.ID)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, 456, msg.MessageID)
	require.Equal(t, "Введите алиас для пути", msg.Text)
}

func TestAddAliasHandler_HandlePathSet_FileSelected(t *testing.T) {
	client := &mock.RemoteStorage{
		DirectoryFn: func(
			owner, repo, path string,
			pageNum, pageSize int,
		) (domain.Page[domain.DirElem], error) {
			return domain.Page[domain.DirElem]{},
				domain.ErrNotDirectory
		},
	}

	h := NewAddAliasHandler(
		&vaultGetterMock{
			vault: domain.UserVault{
				Owner: "owner",
				Repo:  "repo",
			},
		},
		&mock.RemoteStorageGetter{Storage: client},
		&aliasSaverMock{},
	)

	raw, err := json.Marshal(pathChoosingPayload{
		CurPath: "/foo",
		PageNum: 1,
	})
	require.NoError(t, err)

	resp, err := h.Handle(
		context.WithValue(
			context.Background(),
			bot.ChatSessionKey,
			bot.ChatSession{
				ChatID:           123,
				State:            StateWaitPath,
				LastBotMessageID: 456,
				Payload:          raw,
			}),
		bot.Update{
			ChatID: 123,
			Text:   "bar.txt",
		},
	)

	require.NoError(t, err)

	require.Equal(t, "/foo/bar.txt", client.DirectoryPath)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateWaitAlias, *resp.NewChatState)

	var payload aliasBuilder
	require.NoError(
		t,
		json.Unmarshal([]byte(resp.NewPayload), &payload),
	)

	require.Equal(t, "/foo/bar.txt", payload.Path)
	require.Equal(t, domain.TypeFile, payload.Type)
	require.Equal(t, int64(123), payload.ChatID)
	require.NotEmpty(t, payload.ID)

	_, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)
}

func TestAddAliasHandler_handleAliasSet_Success(t *testing.T) {
	id := uuid.New()

	payload, err := json.Marshal(aliasBuilder{
		ID:     id.String(),
		ChatID: 123,
		Path:   "/foo/bar.txt",
		Type:   domain.TypeFile,
	})
	require.NoError(t, err)

	saver := &aliasSaverMock{}

	handler := &AddAliasHandler{
		saver: saver,
	}

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateWaitAlias,
		Payload: payload,
	}

	resp, err := handler.handleAliasSet(
		context.Background(),
		bot.Update{
			ChatID: 123,
			Text:   "my-alias",
		},
		session,
	)

	require.NoError(t, err)
	require.True(t, saver.called)

	require.Equal(t, domain.Alias{
		ID:     id,
		ChatID: 123,
		Path:   "/foo/bar.txt",
		Alias:  "my-alias",
	}, saver.alias)

	require.NotNil(t, resp.Message)

	newState := resp.NewChatState
	require.NotNil(t, newState)
	require.Equal(t, bot.DefaultChatState, *newState)
}

func TestAddAliasHandler_handleAliasSet_InvalidPayload(t *testing.T) {
	saver := &aliasSaverMock{}

	handler := &AddAliasHandler{
		saver: saver,
	}

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateWaitAlias,
		Payload: []byte("invalid json"),
	}

	resp, err := handler.handleAliasSet(
		context.Background(),
		bot.Update{
			ChatID: 123,
			Text:   "my-alias",
		},
		session,
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)
	require.False(t, saver.called)
}

func TestAddAliasHandler_handleAliasSet_InvalidUUID(t *testing.T) {
	payload := `{
		"id": "not-a-uuid",
		"chat_id": 123,
		"path": "/foo/bar.txt",
		"type": "file"
	}`

	saver := &aliasSaverMock{}

	handler := &AddAliasHandler{
		saver: saver,
	}

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateWaitAlias,
		Payload: []byte(payload),
	}

	resp, err := handler.handleAliasSet(
		context.Background(),
		bot.Update{
			ChatID: 123,
			Text:   "my-alias",
		},
		session,
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)
	require.False(t, saver.called)
}

func TestAddAliasHandler_handleAliasSet_SaveError(t *testing.T) {
	id := uuid.New()
	expectedErr := errors.New("save alias: database unavailable")

	payload, err := json.Marshal(aliasBuilder{
		ID:     id.String(),
		ChatID: 123,
		Path:   "/foo/bar.txt",
		Type:   domain.TypeFile,
	})
	require.NoError(t, err)

	saver := &aliasSaverMock{
		err: expectedErr,
	}

	handler := &AddAliasHandler{
		saver: saver,
	}

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateWaitAlias,
		Payload: payload,
	}

	resp, err := handler.handleAliasSet(
		context.Background(),
		bot.Update{
			ChatID: 123,
			Text:   "my-alias",
		},
		session,
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
	require.True(t, saver.called)

	require.Equal(t, id, saver.alias.ID)
	require.Equal(t, int64(123), saver.alias.ChatID)
	require.Equal(t, "/foo/bar.txt", saver.alias.Path)
	require.Equal(t, "my-alias", saver.alias.Alias)
}
