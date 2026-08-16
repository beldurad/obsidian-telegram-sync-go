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

type noteAliasesMock struct {
	page    domain.Page[domain.Alias]
	pageErr error

	alias    domain.Alias
	aliasErr error

	pageCalled bool
	chatID     int64
	pageNum    int
	pageSize   int

	aliasCalled bool
	aliasID     string
	aliasChatID int64
}

func (m *noteAliasesMock) AliasPage(
	ctx context.Context,
	chatID int64,
	pageNum,
	pageSize int,
) (domain.Page[domain.Alias], error) {
	m.pageCalled = true
	m.chatID = chatID
	m.pageNum = pageNum
	m.pageSize = pageSize

	return m.page, m.pageErr
}

func (m *noteAliasesMock) Alias(
	ctx context.Context,
	id string,
	chatID int64,
) (domain.Alias, error) {
	m.aliasCalled = true
	m.aliasID = id
	m.aliasChatID = chatID

	return m.alias, m.aliasErr
}

type noteTemplatesMock struct {
	page    domain.Page[domain.Template]
	pageErr error

	template    domain.Template
	templateErr error

	pageCalled bool
	chatID     int64
	pageNum    int
	pageSize   int

	templateCalled bool
	templateID     string
	templateChatID int64
}

func (m *noteTemplatesMock) TemplatesPage(
	ctx context.Context,
	chatID int64,
	pageNum,
	pageSize int,
) (domain.Page[domain.Template], error) {
	m.pageCalled = true
	m.chatID = chatID
	m.pageNum = pageNum
	m.pageSize = pageSize

	return m.page, m.pageErr
}

func (m *noteTemplatesMock) Template(
	ctx context.Context,
	id string,
	chatID int64,
) (domain.Template, error) {
	m.templateCalled = true
	m.templateID = id
	m.templateChatID = chatID

	return m.template, m.templateErr
}

type noteStorageMock struct {
	mock.RemoteStorage

	FileFn     func(owner, repo, path string) (domain.DirElem, error)
	SaveNoteFn func(
		ctx context.Context,
		owner, repo string,
		note domain.Note,
	) error

	FileCalled bool
	FileOwner  string
	FileRepo   string
	FilePath   string

	SaveNoteCalled bool
	SaveNoteOwner  string
	SaveNoteRepo   string
	SavedNote      domain.Note
}

func (m *noteStorageMock) File(
	owner string,
	repo string,
	path string,
) (domain.DirElem, error) {
	m.FileCalled = true
	m.FileOwner = owner
	m.FileRepo = repo
	m.FilePath = path

	if m.FileFn != nil {
		return m.FileFn(owner, repo, path)
	}

	return domain.DirElem{}, nil
}

func (m *noteStorageMock) SaveNote(
	ctx context.Context,
	owner string,
	repo string,
	note domain.Note,
) error {
	m.SaveNoteCalled = true
	m.SaveNoteOwner = owner
	m.SaveNoteRepo = repo
	m.SavedNote = note

	if m.SaveNoteFn != nil {
		return m.SaveNoteFn(ctx, owner, repo, note)
	}

	return nil
}

func noteAddHandlerForTest(
	aliases *noteAliasesMock,
	templates *noteTemplatesMock,
	storage *noteStorageMock,
	vault *vaultGetterMock,
) *AddNoteHandler {
	return NewAddNoteHandler(
		aliases,
		templates,
		&mock.RemoteStorageGetter{Storage: storage},
		vault,
	)
}

func TestNoteAddPayload_ToNote(t *testing.T) {
	payload := noteAddPayload{
		Path:     "/notes/inbox",
		Template: "daily",
		Text:     "content",
	}

	require.Equal(t, domain.Note{
		Path:     "/notes/inbox",
		Template: "daily",
		Text:     "content",
	}, payload.toNote())
}

func TestNoteAddHandler_Handle_DefaultState(t *testing.T) {
	aliases := &noteAliasesMock{
		page: domain.Page[domain.Alias]{
			Values: []domain.Alias{
				{Path: "/foo", Alias: "foo"},
				{Path: "/bar", Alias: "bar"},
			},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	resp, err := h.Handle(
		context.Background(),
		bot.ChatSession{
			ChatID: 123,
			State:  bot.DefaultChatState,
		},
		bot.Update{
			ChatID: 123,
			Text:   CommandAddNote,
		},
	)

	require.NoError(t, err)

	require.True(t, aliases.pageCalled)
	require.Equal(t, int64(123), aliases.chatID)
	require.Equal(t, 0, aliases.pageNum)
	require.Equal(t, domain.DefaultPageSize, aliases.pageSize)

	require.Nil(t, resp.NewChatState)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&payload,
	))
	require.Equal(t, 0, payload.PageNum)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Выберите путь", msg.Text)
}

func TestNoteAddHandler_Handle_DefaultState_NextButton(t *testing.T) {
	aliases := &noteAliasesMock{
		page: domain.Page[domain.Alias]{
			TotalPages: 3,
			CurPage:    0,
			Values:     []domain.Alias{{Path: "/foo", Alias: "foo"}},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	resp, err := h.Handle(
		context.Background(),
		bot.ChatSession{
			ChatID: 123,
			State:  bot.DefaultChatState,
		},
		bot.Update{
			ChatID: 123,
			Text:   CommandAddNote,
		},
	)

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
	assert.True(t, hasNext, `should have "Next" button`)
	assert.False(t, hasPrev, `should not have "Prev" button`)
}

func TestNoteAddHandler_Handle_NextPage(t *testing.T) {
	aliases := &noteAliasesMock{}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:           123,
		State:            StateNoteWaitAlias,
		LastBotMessageID: 456,
		Payload:          rawPayload,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCommand,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 1, aliases.pageNum)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&payload,
	))
	require.Equal(t, 1, payload.PageNum)

	_, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)
}

func TestNoteAddHandler_Handle_PrevPage(t *testing.T) {
	aliases := &noteAliasesMock{}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 1,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitAlias,
		Payload: rawPayload,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: PrevPageCommand,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 0, aliases.pageNum)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&payload,
	))
	require.Equal(t, 0, payload.PageNum)
}

func TestNoteAddHandler_Handle_SelectAlias(t *testing.T) {
	aliasID := uuid.New()

	aliases := &noteAliasesMock{
		alias: domain.Alias{
			ID:     aliasID,
			ChatID: 123,
			Path:   "/notes/inbox",
			Alias:  "inbox",
		},
	}
	templates := &noteTemplatesMock{}

	h := noteAddHandlerForTest(
		aliases,
		templates,
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:           123,
		State:            StateNoteWaitAlias,
		LastBotMessageID: 456,
		Payload:          rawPayload,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: aliasID.String(),
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, aliases.aliasCalled)
	require.Equal(t, aliasID.String(), aliases.aliasID)
	require.Equal(t, int64(123), aliases.aliasChatID)

	require.True(t, templates.pageCalled)
	require.Equal(t, 0, templates.pageNum)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateNoteWaitTemplate, *resp.NewChatState)

	var payload noteAddPayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&payload,
	))
	require.Equal(t, "/notes/inbox", payload.Path)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, 456, msg.MessageID)
	require.Equal(t, "Выберите шаблон", msg.Text)
	require.NotNil(t, msg.ReplyMarkup)
}

func TestNoteAddHandler_Handle_UnknownState_SelectsAlias(t *testing.T) {
	aliasID := uuid.New()

	aliases := &noteAliasesMock{
		alias: domain.Alias{
			ID:     aliasID,
			ChatID: 123,
			Path:   "/notes/inbox",
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   bot.ChatState("UNKNOWN_STATE"),
		Payload: rawPayload,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: aliasID.String(),
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, aliases.aliasCalled)
	require.Equal(t, aliasID.String(), aliases.aliasID)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateNoteWaitTemplate, *resp.NewChatState)
}

func TestNoteAddHandler_Handle_InvalidPayload(t *testing.T) {
	aliases := &noteAliasesMock{}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitAlias,
		Payload: []byte("not-json"),
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCommand,
				},
			},
		},
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)
	require.False(t, aliases.pageCalled)
}

func TestNoteAddHandler_Handle_SelectAlias_AliasError(t *testing.T) {
	expectedErr := errors.New("alias service unavailable")

	aliases := &noteAliasesMock{
		aliasErr: expectedErr,
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitAlias,
		Payload: rawPayload,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: uuid.NewString(),
				},
			},
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestNoteAddHandler_Handle_SelectAlias_TemplatesError(t *testing.T) {
	expectedErr := errors.New("templates service unavailable")

	templates := &noteTemplatesMock{
		pageErr: expectedErr,
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{
			alias: domain.Alias{Path: "/notes/inbox"},
		},
		templates,
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitAlias,
		Payload: rawPayload,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: uuid.NewString(),
				},
			},
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestNoteAddHandler_Handle_AliasPageError(t *testing.T) {
	expectedErr := errors.New("database unavailable")

	aliases := &noteAliasesMock{
		pageErr: expectedErr,
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	resp, err := h.Handle(
		context.Background(),
		bot.ChatSession{
			ChatID: 123,
			State:  bot.DefaultChatState,
		},
		bot.Update{
			ChatID: 123,
			Text:   CommandAddNote,
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestNoteAddHandler_Handle_SelectTemplate(t *testing.T) {
	templateID := uuid.New()

	templates := &noteTemplatesMock{
		template: domain.Template{
			ID:     templateID,
			ChatID: 123,
			Name:   "daily",
			Value:  "{{date}}",
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		templates,
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	payload := noteAddPayload{
		Path:      "/notes/inbox",
		AliasPage: pagePayload{PageNum: 1},
		TemplatePage: pagePayload{
			PageNum: 1,
		},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:           123,
		State:            StateNoteWaitTemplate,
		LastBotMessageID: 456,
		Payload:          raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: templateID.String(),
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, templates.templateCalled)
	require.Equal(t, templateID.String(), templates.templateID)
	require.Equal(t, int64(123), templates.templateChatID)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateNoteWaitText, *resp.NewChatState)

	var got noteAddPayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&got,
	))
	require.Equal(t, "/notes/inbox", got.Path)
	require.Equal(t, "{{date}}", got.Template)
	require.Equal(t, 0, got.TemplatePage.PageNum)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, 456, msg.MessageID)
	require.Equal(t, "Введите текст заметки", msg.Text)
	require.Nil(t, msg.ReplyMarkup)
}

func TestNoteAddHandler_Handle_Template_NextPage(t *testing.T) {
	templates := &noteTemplatesMock{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		templates,
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
		TemplatePage: pagePayload{
			PageNum: 1,
		},
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitTemplate,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCommand,
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, templates.pageCalled)
	require.Equal(t, 2, templates.pageNum)

	var got noteAddPayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&got,
	))
	require.Equal(t, 2, got.TemplatePage.PageNum)

	_, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)
}

func TestNoteAddHandler_Handle_Template_InvalidPayload(t *testing.T) {
	templates := &noteTemplatesMock{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		templates,
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitTemplate,
		Payload: []byte("not-json"),
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCommand,
				},
			},
		},
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)
	require.False(t, templates.pageCalled)
}

func TestNoteAddHandler_Handle_Template_TemplateError(t *testing.T) {
	expectedErr := errors.New("template service unavailable")

	templates := &noteTemplatesMock{
		templateErr: expectedErr,
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		templates,
		&noteStorageMock{},
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitTemplate,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: uuid.NewString(),
				},
			},
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestNoteAddHandler_Handle_Text_ExistingFile(t *testing.T) {
	storage := &noteStorageMock{
		FileFn: func(
			owner, repo, path string,
		) (domain.DirElem, error) {
			return domain.DirElem{Type: domain.TypeFile}, nil
		},
	}
	vault := &vaultGetterMock{
		vault: domain.UserVault{
			Owner: "test-owner",
			Repo:  "test-repo",
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		vault,
	)

	raw, err := json.Marshal(noteAddPayload{
		Path:     "/notes/inbox",
		Template: "daily",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitText,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.NoError(t, err)

	require.True(t, storage.FileCalled)
	require.Equal(t, "test-owner", storage.FileOwner)
	require.Equal(t, "test-repo", storage.FileRepo)
	require.Equal(t, "/notes/inbox", storage.FilePath)

	require.True(t, storage.SaveNoteCalled)
	require.Equal(t, "test-owner", storage.SaveNoteOwner)
	require.Equal(t, "test-repo", storage.SaveNoteRepo)
	require.Equal(t, domain.Note{
		Path:     "/notes/inbox",
		Template: "daily",
		Text:     "content",
	}, storage.SavedNote)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, bot.DefaultChatState, *resp.NewChatState)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Заметка успешно сохранена", msg.Text)
}

func TestNoteAddHandler_Handle_Text_FileNotFound(t *testing.T) {
	storage := &noteStorageMock{
		FileFn: func(
			owner, repo, path string,
		) (domain.DirElem, error) {
			return domain.DirElem{}, errors.New("file not found")
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitText,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.NoError(t, err)

	require.False(t, storage.SaveNoteCalled)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateNoteWaitFilename, *resp.NewChatState)

	var got noteAddPayload
	require.NoError(t, json.Unmarshal(
		[]byte(resp.NewPayload),
		&got,
	))
	require.Equal(t, "content", got.Text)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Введите название файла", msg.Text)
}

func TestNoteAddHandler_Handle_Text_NotFileType(t *testing.T) {
	storage := &noteStorageMock{
		FileFn: func(
			owner, repo, path string,
		) (domain.DirElem, error) {
			return domain.DirElem{Type: domain.TypeDir}, nil
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitText,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.NoError(t, err)

	require.False(t, storage.SaveNoteCalled)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, StateNoteWaitFilename, *resp.NewChatState)
}

func TestNoteAddHandler_Handle_Text_ClientError(t *testing.T) {
	expectedErr := errors.New("client error")

	h := NewAddNoteHandler(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		&mock.RemoteStorageGetter{Err: expectedErr},
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitText,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestNoteAddHandler_Handle_Text_VaultError(t *testing.T) {
	expectedErr := errors.New("vault error")

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		&noteStorageMock{},
		&vaultGetterMock{err: expectedErr},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitText,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
}

func TestNoteAddHandler_Handle_Text_SaveNoteError(t *testing.T) {
	expectedErr := errors.New("save note failed")

	storage := &noteStorageMock{
		FileFn: func(
			owner, repo, path string,
		) (domain.DirElem, error) {
			return domain.DirElem{Type: domain.TypeFile}, nil
		},
		SaveNoteFn: func(
			ctx context.Context,
			owner, repo string,
			note domain.Note,
		) error {
			return expectedErr
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitText,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
	require.True(t, storage.SaveNoteCalled)
}

func TestNoteAddHandler_Handle_Filename(t *testing.T) {
	storage := &noteStorageMock{}
	vault := &vaultGetterMock{
		vault: domain.UserVault{
			Owner: "test-owner",
			Repo:  "test-repo",
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		vault,
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/",
		Text: "content",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitFilename,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.NoError(t, err)

	require.True(t, storage.SaveNoteCalled)
	require.Equal(t, "test-owner", storage.SaveNoteOwner)
	require.Equal(t, "test-repo", storage.SaveNoteRepo)
	require.Equal(t, domain.Note{
		Path: "/notes/new.md",
		Text: "content",
	}, storage.SavedNote)

	require.NotNil(t, resp.NewChatState)
	require.Equal(t, bot.DefaultChatState, *resp.NewChatState)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Заметка успешно сохранена", msg.Text)
}

func TestNoteAddHandler_Handle_Filename_NoTrailingSlash(t *testing.T) {
	storage := &noteStorageMock{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes",
		Text: "content",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitFilename,
		Payload: raw,
	}

	_, err = h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.NoError(t, err)
	require.Equal(t, "/notes/new.md", storage.SavedNote.Path)
}

func TestNoteAddHandler_Handle_Filename_Root(t *testing.T) {
	storage := &noteStorageMock{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Text: "content",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitFilename,
		Payload: raw,
	}

	_, err = h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.NoError(t, err)
	require.Equal(t, "new.md", storage.SavedNote.Path)
}

func TestNoteAddHandler_Handle_Filename_InvalidPayload(t *testing.T) {
	storage := &noteStorageMock{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitFilename,
		Payload: []byte("not-json"),
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)
	require.False(t, storage.SaveNoteCalled)
}

func TestNoteAddHandler_Handle_Filename_SaveNoteError(t *testing.T) {
	expectedErr := errors.New("save note failed")

	storage := &noteStorageMock{
		SaveNoteFn: func(
			ctx context.Context,
			owner, repo string,
			note domain.Note,
		) error {
			return expectedErr
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/",
	})
	require.NoError(t, err)

	session := bot.ChatSession{
		ChatID:  123,
		State:   StateNoteWaitFilename,
		Payload: raw,
	}

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)
	require.True(t, storage.SaveNoteCalled)
}
