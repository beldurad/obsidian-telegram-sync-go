package bot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type templateGetterMock struct {
	templatesPage domain.Page[domain.Template]
	err           error

	called   bool
	chatID   int64
	pageNum  int
	pageSize int
}

func (m *templateGetterMock) TemplatesPage(
	ctx context.Context,
	chatID int64,
	pageNum int,
	pageSize int,
) (domain.Page[domain.Template], error) {
	m.called = true
	m.chatID = chatID
	m.pageNum = pageNum
	m.pageSize = pageSize

	return m.templatesPage, m.err
}

func TestGetTemplateHandler_Handle_DefaultState(t *testing.T) {
	getter := &templateGetterMock{
		templatesPage: domain.Page[domain.Template]{
			Values: []domain.Template{
				{
					Name:  "greeting",
					Value: "Hello, world!",
				},
				{
					Name:  "signature",
					Value: "Best regards",
				},
			},
		},
	}

	handler := NewGetTemplateHandler(getter)

	session := bot.NewChatSession(123)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   CommandGetTemplates,
		},
	)

	require.NoError(t, err)

	require.True(t, getter.called)
	require.Equal(t, int64(123), getter.chatID)
	require.Equal(t, 0, getter.pageNum)
	require.Equal(t, domain.DefaultPageSize, getter.pageSize)

	require.Equal(t, StateGetTemplate, string(session.State()))

	var payload pagePayload
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, 0, payload.PageNum)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)
	require.Contains(t, msg.Text, "greeting")
	require.Contains(t, msg.Text, "Hello, world!")
	require.Contains(t, msg.Text, "signature")
	require.Contains(t, msg.Text, "Best regards")
}

func TestGetTemplateHandler_Handle_NextPage(t *testing.T) {
	getter := &templateGetterMock{}

	handler := NewGetTemplateHandler(getter)

	raw, err := json.Marshal(pagePayload{
		PageNum: 2,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateGetTemplate)
	session.SetPayload(raw)

	resp, err := handler.Handle(
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

	require.NoError(t, err)

	require.Equal(t, 3, getter.pageNum)

	var payload pagePayload
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, 3, payload.PageNum)

	require.Equal(t, StateGetTemplate, string(session.State()))
	_ = resp
}

func TestGetTemplateHandler_Handle_PrevPage(t *testing.T) {
	getter := &templateGetterMock{}

	handler := NewGetTemplateHandler(getter)

	raw, err := json.Marshal(pagePayload{
		PageNum: 2,
	})
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateGetTemplate)
	session.SetPayload(raw)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Raw: tgbotapi.Update{
				CallbackQuery: &tgbotapi.CallbackQuery{
					Data: PrevPageCallback,
				},
			},
		},
	)

	require.NoError(t, err)

	require.Equal(t, 1, getter.pageNum)

	var payload pagePayload
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, 1, payload.PageNum)
	_ = resp
}

func TestGetTemplateHandler_Handle_InvalidPayload(t *testing.T) {
	getter := &templateGetterMock{}

	handler := NewGetTemplateHandler(getter)

	session := bot.NewChatSession(123)
	session.SetState(StateGetTemplate)
	session.SetPayload([]byte("invalid-json"))

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   NextPageCallback,
		},
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)

	require.False(t, getter.called)
}

func TestGetTemplateHandler_Handle_TemplatesPageError(t *testing.T) {
	expectedErr := errors.New("database error")

	getter := &templateGetterMock{
		err: expectedErr,
	}

	handler := NewGetTemplateHandler(getter)

	session := bot.NewChatSession(123)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   CommandGetTemplates,
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)

	require.True(t, getter.called)
	require.Equal(t, 0, getter.pageNum)
}

type templateSaverMock struct {
	saveFn func(ctx context.Context, template domain.Template) error

	saveCalled bool
	saved      domain.Template
}

func (m *templateSaverMock) Save(
	ctx context.Context,
	template domain.Template,
) error {
	m.saveCalled = true
	m.saved = template

	if m.saveFn != nil {
		return m.saveFn(ctx, template)
	}

	return nil
}

func TestTemplateAddHandler_Handle_DefaultState(t *testing.T) {
	saver := &templateSaverMock{}
	handler := NewTemplateAddHandler(saver)

	const chatID int64 = 123

	session := bot.NewChatSession(chatID)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: chatID,
			Text:   CommandAddTemplate,
		},
	)

	require.NoError(t, err)
	require.False(t, saver.saveCalled)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, chatID, msg.ChatID)

	require.Equal(t, StateWaitTemplateValue, string(session.State()))

	var payload templateBuilder
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &payload),
	)

	require.Equal(t, chatID, payload.ChatID)
	require.Empty(t, payload.Value)
	require.Empty(t, payload.Name)

	require.NotEmpty(t, payload.ID)
	require.NotEmpty(t, payload.CreatedAt)

	_, err = uuid.Parse(payload.ID)
	require.NoError(t, err)

	_, err = time.Parse(time.DateOnly, payload.CreatedAt)
	require.NoError(t, err)
}

func TestTemplateAddHandler_Handle_ValueState(t *testing.T) {
	saver := &templateSaverMock{}
	handler := NewTemplateAddHandler(saver)

	payload := templateBuilder{
		ID:        uuid.NewString(),
		ChatID:    123,
		CreatedAt: "2026-08-11",
	}

	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateWaitTemplateValue)
	session.SetPayload(rawPayload)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "Hello, {}!",
		},
	)

	require.NoError(t, err)
	require.False(t, saver.saveCalled)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, int64(123), msg.ChatID)

	require.Equal(t, StateWaitTemplateName, string(session.State()))

	var newPayload templateBuilder
	require.NoError(
		t,
		json.Unmarshal(session.Payload(), &newPayload),
	)

	require.Equal(t, payload.ID, newPayload.ID)
	require.Equal(t, payload.ChatID, newPayload.ChatID)
	require.Equal(t, payload.CreatedAt, newPayload.CreatedAt)

	require.Equal(t, "Hello, {}!", newPayload.Value)
	require.Empty(t, newPayload.Name)
}

func TestTemplateAddHandler_Handle_ValueState_InvalidPayload(t *testing.T) {
	saver := &templateSaverMock{}
	handler := NewTemplateAddHandler(saver)

	session := bot.NewChatSession(123)
	session.SetState(StateWaitTemplateValue)
	session.SetPayload([]byte("invalid-json"))

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "Hello, {}!",
		},
	)

	require.Error(t, err)
	require.Equal(t, bot.Response{}, resp)
	require.False(t, saver.saveCalled)
}

func TestTemplateAddHandler_Handle_NameState(t *testing.T) {
	saver := &templateSaverMock{}
	handler := NewTemplateAddHandler(saver)

	id := uuid.New()
	const chatID int64 = 123
	const createdAt = "2026-08-11"

	payload := templateBuilder{
		ID:        id.String(),
		ChatID:    chatID,
		Value:     "Hello, {}!",
		CreatedAt: createdAt,
	}

	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	ctx := context.Background()

	session := bot.NewChatSession(chatID)
	session.SetState(StateWaitTemplateName)
	session.SetPayload(rawPayload)

	resp, err := handler.Handle(
		ctx,
		session,
		bot.Update{
			ChatID: chatID,
			Text:   "greeting",
		},
	)

	require.NoError(t, err)

	require.True(t, saver.saveCalled)

	require.Equal(t, domain.Template{
		ID:        id,
		ChatID:    chatID,
		Name:      "greeting",
		Value:     "Hello, {}!",
		CreatedAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
	}, saver.saved)

	msg, ok := resp.Message.(tgbotapi.MessageConfig)
	require.True(t, ok)

	require.Equal(t, chatID, msg.ChatID)

	require.Equal(t, bot.DefaultChatState, session.State())
	require.Empty(t, session.Payload())
}

func TestTemplateAddHandler_Handle_NameState_SaveError(t *testing.T) {
	expectedErr := errors.New("database error")

	saver := &templateSaverMock{
		saveFn: func(ctx context.Context, template domain.Template) error {
			return expectedErr
		},
	}

	handler := NewTemplateAddHandler(saver)

	payload := templateBuilder{
		ID:        uuid.NewString(),
		ChatID:    123,
		Value:     "Hello, {}!",
		CreatedAt: "2026-08-11",
	}

	rawPayload, err := json.Marshal(payload)
	require.NoError(t, err)

	session := bot.NewChatSession(123)
	session.SetState(StateWaitTemplateName)
	session.SetPayload(rawPayload)

	resp, err := handler.Handle(
		context.Background(),
		session,
		bot.Update{
			ChatID: 123,
			Text:   "greeting",
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, bot.Response{}, resp)

	require.True(t, saver.saveCalled)
	require.Equal(t, "greeting", saver.saved.Name)
}
