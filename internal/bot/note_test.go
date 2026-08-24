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
	m.page.CurPage = pageNum

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
	m.page.CurPage = pageNum

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

func noteAddHandlerForTest(
	aliases *noteAliasesMock,
	templates *noteTemplatesMock,
	storage *mock.RemoteStorage,
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
			TotalPages: 1,
			CurPage:    0,
			Values: []domain.Alias{
				{
					Path:  domain.Path{Value: "/foo", Type: domain.PathTypeFile},
					Alias: "foo",
				},
				{
					Path:  domain.Path{Value: "/bar", Type: domain.PathTypeFile},
					Alias: "bar",
				},
			},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	session := bot.NewChatSession(123)

	resp, err := h.Handle(
		context.Background(),
		session,
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

	require.Equal(t, StateNoteWaitAlias, string(session.State()))

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
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
			Values: []domain.Alias{{
				Path:  domain.Path{Value: "/foo", Type: domain.PathTypeFile},
				Alias: "foo",
			}},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	session := bot.NewChatSession(123)

	resp, err := h.Handle(
		context.Background(),
		session,
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
			case NextPageCallback:
				hasNext = true
			case PrevPageCallback:
				hasPrev = true
			}
		}
	}
	assert.True(t, hasNext, `should have "Next" button`)
	assert.False(t, hasPrev, `should not have "Prev" button`)
}

func TestNoteAddHandler_Handle_NextPage(t *testing.T) {
	aliases := &noteAliasesMock{
		page: domain.Page[domain.Alias]{
			TotalPages: 2,
			CurPage:    0,
			Values: []domain.Alias{{
				Path:  domain.Path{Value: "/foo", Type: domain.PathTypeFile},
				Alias: "foo",
			}},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum:    0,
		TotalPages: 2,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitAlias)
	session.SetPayload(rawPayload)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: NextPageCallback,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCallback,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 1, aliases.pageNum)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
		&payload,
	))
	require.Equal(t, 1, payload.PageNum)

	_, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)
}

func TestNoteAddHandler_Handle_PrevPage(t *testing.T) {
	aliases := &noteAliasesMock{
		page: domain.Page[domain.Alias]{
			TotalPages: 2,
			CurPage:    1,
			Values: []domain.Alias{{
				Path:  domain.Path{Value: "/foo", Type: domain.PathTypeFile},
				Alias: "foo",
			}},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 1,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitAlias)
	session.SetPayload(rawPayload)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: PrevPageCallback,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: PrevPageCallback,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 0, aliases.pageNum)

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
		&payload,
	))
	require.Equal(t, 0, payload.PageNum)
	_ = resp
}

func TestNoteAddHandler_Handle_SelectAlias(t *testing.T) {
	aliasID := uuid.New()

	aliases := &noteAliasesMock{
		alias: domain.Alias{
			ID:     aliasID,
			ChatID: 123,
			Path: domain.Path{
				Value: "/notes/inbox",
				Type:  domain.PathTypeFile,
			},
			Alias: "inbox",
		},
	}
	templates := &noteTemplatesMock{
		page: domain.Page[domain.Template]{
			TotalPages: 1,
			CurPage:    0,
			Values: []domain.Template{{
				ID:   uuid.New(),
				Name: "daily",
			}},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		templates,
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitAlias)
	session.SetPayload(rawPayload)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: aliasID.String(),
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

	require.Equal(t, StateNoteWaitTemplate, string(session.State()))

	var payload noteAddPayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
		&payload,
	))
	require.Equal(t, "/notes/inbox", payload.Path)
	require.Equal(t, domain.PathTypeFile, payload.PathType)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Выберите шаблон", msg.Text)
	require.NotNil(t, msg.ReplyMarkup)
}

func TestNoteAddHandler_Handle_UnknownState_ListsAliases(t *testing.T) {
	aliases := &noteAliasesMock{
		page: domain.Page[domain.Alias]{
			TotalPages: 1,
			CurPage:    0,
			Values: []domain.Alias{
				{
					Path:  domain.Path{Value: "/foo", Type: domain.PathTypeFile},
					Alias: "foo",
				},
			},
		},
	}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(bot.ChatState("UNKNOWN_STATE"))
	session.SetPayload(rawPayload)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "whatever",
		},
	)

	require.NoError(t, err)

	require.False(t, aliases.aliasCalled)
	require.True(t, aliases.pageCalled)
	require.Equal(t, int64(123), aliases.chatID)
	require.Equal(t, 0, aliases.pageNum)
	require.Equal(t, domain.DefaultPageSize, aliases.pageSize)

	require.Equal(t, StateNoteWaitAlias, string(session.State()))

	var payload pagePayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
		&payload,
	))
	require.Equal(t, 0, payload.PageNum)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Выберите путь", msg.Text)
}

func TestNoteAddHandler_Handle_InvalidPayload(t *testing.T) {
	aliases := &noteAliasesMock{}

	h := noteAddHandlerForTest(
		aliases,
		&noteTemplatesMock{},
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitAlias)
	session.SetPayload([]byte("not-json"))

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCallback,
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
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitAlias)
	session.SetPayload(rawPayload)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: uuid.NewString(),
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
			alias: domain.Alias{
				Path: domain.Path{
					Value: "/notes/inbox",
					Type:  domain.PathTypeFile,
				},
			},
		},
		templates,
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	rawPayload, err := json.Marshal(pagePayload{
		PageNum: 0,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitAlias)
	session.SetPayload(rawPayload)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: uuid.NewString(),
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
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	session := bot.NewChatSession(123)

	resp, err := h.Handle(
		context.Background(),
		session,
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
		&mock.RemoteStorage{},
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

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitTemplate)
	session.SetPayload(raw)

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

	require.Equal(t, StateNoteWaitText, string(session.State()))

	var got noteAddPayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
		&got,
	))
	require.Equal(t, "/notes/inbox", got.Path)
	require.Equal(t, "{{date}}", got.Template)
	require.Equal(t, 0, got.TemplatePage.PageNum)

	msg, ok := resp.Message.(tgbotapi.EditMessageTextConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Введите текст заметки", msg.Text)
	require.Nil(t, msg.ReplyMarkup)
}

func TestNoteAddHandler_Handle_Template_NextPage(t *testing.T) {
	totalPages := 3
	curPage := 1
	templates := &noteTemplatesMock{
		page: domain.Page[domain.Template]{
			TotalPages: totalPages,
			CurPage:    curPage,
			Values: []domain.Template{{
				ID:   uuid.New(),
				Name: "daily",
			}},
		},
	}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		templates,
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
		TemplatePage: pagePayload{
			PageNum:    curPage,
			TotalPages: totalPages,
		},
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitTemplate)
	session.SetPayload(raw)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID:       123,
			CallbackData: NextPageCallback,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCallback,
				},
			},
		},
	)

	require.NoError(t, err)

	require.True(t, templates.pageCalled)
	require.Equal(t, 2, templates.pageNum)

	var got noteAddPayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
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
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitTemplate)
	session.SetPayload([]byte("not-json"))

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: NextPageCallback,
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
		&mock.RemoteStorage{},
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/inbox",
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitTemplate)
	session.SetPayload(raw)

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

func TestNoteAddHandler_Handle_Text_FilePath_UpdatesNote(t *testing.T) {
	storage := &mock.RemoteStorage{}
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
		Path:     "/notes/inbox.md",
		PathType: domain.PathTypeFile,
		Template: "daily",
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitText)
	session.SetPayload(raw)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.NoError(t, err)

	require.True(t, storage.UpdateNoteCalled)
	require.False(t, storage.CreateNoteCalled)
	require.Equal(t, "test-owner", storage.UpdateNoteOwner)
	require.Equal(t, "test-repo", storage.UpdateNoteRepo)
	require.Equal(t, domain.Note{
		Path:     "/notes/inbox.md",
		Template: "daily",
		Text:     "content",
	}, storage.UpdatedNote)

	require.Equal(t, bot.DefaultChatState, session.State())

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Заметка успешно обновлена", msg.Text)
}

func TestNoteAddHandler_Handle_Text_DirPath_AsksFilename(t *testing.T) {
	storage := &mock.RemoteStorage{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path:     "/notes/inbox",
		PathType: domain.PathTypeDir,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitText)
	session.SetPayload(raw)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "content",
		},
	)

	require.NoError(t, err)

	require.False(t, storage.UpdateNoteCalled)
	require.False(t, storage.CreateNoteCalled)

	require.Equal(t, StateNoteWaitFilename, string(session.State()))

	var got noteAddPayload
	require.NoError(t, json.Unmarshal(
		session.Payload(),
		&got,
	))
	require.Equal(t, "content", got.Text)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Введите название файла", msg.Text)
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
		Path:     "/notes/inbox",
		PathType: domain.PathTypeFile,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitText)
	session.SetPayload(raw)

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

func TestNoteAddHandler_Handle_Filename(t *testing.T) {
	storage := &mock.RemoteStorage{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{
			vault: domain.UserVault{
				Owner: "test-owner",
				Repo:  "test-repo",
			},
		},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes/",
		Text: "content",
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitFilename)
	session.SetPayload(raw)

	resp, err := h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.NoError(t, err)

	require.True(t, storage.CreateNoteCalled)
	require.False(t, storage.UpdateNoteCalled)
	require.Equal(t, "test-owner", storage.CreateNoteOwner)
	require.Equal(t, "test-repo", storage.CreateNoteRepo)
	require.Equal(t, domain.Note{
		Path: "/notes/new.md",
		Text: "content",
	}, storage.CreatedNote)

	require.Equal(t, bot.DefaultChatState, session.State())

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Equal(t, "Заметка успешно сохранена", msg.Text)
}

func TestNoteAddHandler_Handle_Filename_NoTrailingSlash(t *testing.T) {
	storage := &mock.RemoteStorage{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{
			vault: domain.UserVault{
				Owner: "test-owner",
				Repo:  "test-repo",
			},
		},
	)

	raw, err := json.Marshal(noteAddPayload{
		Path: "/notes",
		Text: "content",
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitFilename)
	session.SetPayload(raw)

	_, err = h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.NoError(t, err)
	require.True(t, storage.CreateNoteCalled)
	require.Equal(t, "/notes/new.md", storage.CreatedNote.Path)
	require.Equal(t, bot.DefaultChatState, session.State())
}

func TestNoteAddHandler_Handle_Filename_Root(t *testing.T) {
	storage := &mock.RemoteStorage{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{
			vault: domain.UserVault{
				Owner: "test-owner",
				Repo:  "test-repo",
			},
		},
	)

	raw, err := json.Marshal(noteAddPayload{
		Text: "content",
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitFilename)
	session.SetPayload(raw)

	_, err = h.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "new.md",
		},
	)

	require.NoError(t, err)
	require.True(t, storage.CreateNoteCalled)
	require.Equal(t, "new.md", storage.CreatedNote.Path)
	require.Equal(t, bot.DefaultChatState, session.State())
}

func TestNoteAddHandler_Handle_Filename_InvalidPayload(t *testing.T) {
	storage := &mock.RemoteStorage{}

	h := noteAddHandlerForTest(
		&noteAliasesMock{},
		&noteTemplatesMock{},
		storage,
		&vaultGetterMock{},
	)

	session := bot.NewChatSession(123)
	session.SetState(StateNoteWaitFilename)
	session.SetPayload([]byte("not-json"))

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
	require.False(t, storage.CreateNoteCalled)
}
