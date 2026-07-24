package bot_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type telegramMock struct {
	updates chan tgbotapi.Update

	mu   sync.Mutex
	sent []tgbotapi.Chattable

	msgID int
}

func newTelegramMock() *telegramMock {
	return &telegramMock{
		updates: make(chan tgbotapi.Update, 10),
	}
}

func (m *telegramMock) GetUpdatesChan() <-chan tgbotapi.Update {
	return m.updates
}

func (m *telegramMock) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.msgID == 0 {
		m.msgID = 100
	}

	m.sent = append(m.sent, c)

	return tgbotapi.Message{
		MessageID: m.msgID,
	}, nil
}

type sessionMock struct {
	session *bot.ChatSession

	updated *bot.ChatSession
}

func (m *sessionMock) SessionByChatID(chatID int64) (*bot.ChatSession, error) {
	return m.session, nil
}

func (m *sessionMock) UpdateSession(
	chatID int64,
	new *bot.ChatSession,
) error {
	m.updated = new
	return nil
}

type handlerMock struct {
	called bool

	update bot.Update

	response bot.Response
}

func (h *handlerMock) Handle(
	ctx context.Context,
	u bot.Update,
) (bot.Response, error) {

	h.called = true
	h.update = u

	resp := h.response

	if resp == (bot.Response{}) {
		resp = bot.Response{
			Message: tgbotapi.NewMessage(
				u.ChatID,
				"ok",
			),
		}
	}

	return resp, nil
}

func runBot(
	b *bot.Bot,
	updatesChan chan tgbotapi.Update,
	updates ...tgbotapi.Update,
) {
	ctx, cancel := context.WithCancel(context.Background())

	go b.StartListening(ctx)

	for _, u := range updates {
		updatesChan <- u
	}

	time.Sleep(50 * time.Millisecond)

	cancel()
}

func TestBot_CommandHandler(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
		},
	}

	b := bot.New(session, tg)

	handler := &handlerMock{}

	b.AddHandlerForCommand(
		"/start",
		handler,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	assert.True(t, handler.called)
	assert.Equal(t, int64(123), handler.update.ChatID)
}

func TestBot_StateHandler(t *testing.T) {

	tg := newTelegramMock()

	testState := bot.ChatState("waiting_name")

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
			State:  testState,
		},
	}

	b := bot.New(session, tg)

	handler := &handlerMock{}

	b.AddHandlerForState(
		testState,
		handler,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "Alex",
		},
	}

	runBot(b, tg.updates, update)

	assert.True(t, handler.called)
	assert.Equal(t, "Alex", handler.update.Text)
}

func TestBot_BothHandlerHasPriority(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
			State:  "register",
		},
	}

	b := bot.New(session, tg)

	commandHandler := &handlerMock{}
	stateHandler := &handlerMock{}
	bothHandler := &handlerMock{}

	b.AddHandlerForCommand(
		"/start",
		commandHandler,
	)

	b.AddHandlerForState(
		"register",
		stateHandler,
	)

	b.AddHandler(
		"/start",
		"register",
		bothHandler,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	assert.True(t, bothHandler.called)

	assert.False(t, commandHandler.called)
	assert.False(t, stateHandler.called)
}

func TestBot_StateFallbackWhenBothMissing(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
			State:  "register",
		},
	}

	b := bot.New(session, tg)

	stateHandler := &handlerMock{}

	b.AddHandlerForState(
		"register",
		stateHandler,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/unknown",
		},
	}

	runBot(b, tg.updates, update)

	assert.True(t, stateHandler.called)
}

func TestBot_SessionChangesAfterHandle(t *testing.T) {

	newState := bot.ChatState("waiting")

	tg := newTelegramMock()

	tg.msgID = 200

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID:           123,
			State:            bot.DefaultChatState,
			LastBotMessageID: 456,
		},
	}

	b := bot.New(session, tg)

	handler := &handlerMock{
		response: bot.Response{
			Message: tgbotapi.NewMessage(
				123,
				"ok",
			),
			NewChatState: &newState,
		},
	}

	b.AddHandlerForCommand(
		"/test",
		handler,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/test",
		},
	}

	runBot(b, tg.updates, update)

	assert.Equal(
		t,
		bot.ChatSession{
			ChatID:           123,
			State:            newState,
			LastBotMessageID: tg.msgID,
		},
		*session.updated,
	)
}

type errorHandlerMock struct {
	matched bool
	called  bool

	response bot.Response
}

func (h *errorHandlerMock) Match(err error) bool {
	h.matched = true
	return true
}

func (h *errorHandlerMock) Handle(
	chatID int64,
	err error,
) bot.Response {

	h.called = true

	return h.response
}

type failingHandler struct {
	err error
}

func (h *failingHandler) Handle(
	ctx context.Context,
	u bot.Update,
) (bot.Response, error) {

	return bot.Response{}, h.err
}

type failingSessionService struct {
	err error
}

func (s *failingSessionService) SessionByChatID(
	chatID int64,
) (*bot.ChatSession, error) {

	return nil, s.err
}

func (s *failingSessionService) UpdateSession(
	chatID int64,
	new *bot.ChatSession,
) error {
	return nil
}

func TestBot_SessionError(t *testing.T) {

	tg := newTelegramMock()

	b := bot.New(
		&failingSessionService{
			err: errors.New("db error"),
		},
		tg,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	require.Len(t, tg.sent, 1)

	msgCfg, ok := tg.sent[0].(tgbotapi.MessageConfig)
	require.True(t, ok)

	assert.Equal(
		t,
		bot.ErrInternalServer.Error(),
		msgCfg.Text,
	)
}

func TestBot_ErrorHandlerCalled(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
		},
	}

	b := bot.New(session, tg)

	expectedText := "custom error"

	eh := &errorHandlerMock{
		response: bot.Response{
			Message: tgbotapi.NewMessage(
				123,
				expectedText,
			),
		},
	}

	b.AddErrorHandler(eh)

	b.AddHandlerForCommand(
		"/start",
		&failingHandler{
			err: errors.New("boom"),
		},
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	assert.True(t, eh.called)

	require.Len(t, tg.sent, 1)

	msgCfg := tg.sent[0].(tgbotapi.MessageConfig)

	assert.Equal(
		t,
		expectedText,
		msgCfg.Text,
	)
}

type neverMatchErrorHandler struct{}

func (h *neverMatchErrorHandler) Match(err error) bool {
	return false
}

func (h *neverMatchErrorHandler) Handle(
	chatID int64,
	err error,
) bot.Response {
	panic("should not be called")
}

func TestBot_DefaultErrorHandler(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
		},
	}

	b := bot.New(session, tg)

	b.AddErrorHandler(
		&neverMatchErrorHandler{},
	)

	b.AddHandlerForCommand(
		"/start",
		&failingHandler{
			err: errors.New("boom"),
		},
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	require.Len(t, tg.sent, 1)

	msgCfg := tg.sent[0].(tgbotapi.MessageConfig)

	assert.Equal(
		t,
		bot.ErrUnknown.Error(),
		msgCfg.Text,
	)
}

type failingTelegramMock struct {
	updates chan tgbotapi.Update
}

func (m *failingTelegramMock) GetUpdatesChan() <-chan tgbotapi.Update {
	return m.updates
}

func (m *failingTelegramMock) Send(
	tgbotapi.Chattable,
) (tgbotapi.Message, error) {

	return tgbotapi.Message{},
		errors.New("telegram error")
}

type stateChangingHandler struct{}

func (h *stateChangingHandler) Handle(
	ctx context.Context,
	u bot.Update,
) (bot.Response, error) {

	newState := bot.ChatState("next")

	return bot.Response{
		Message: tgbotapi.NewMessage(
			u.ChatID,
			"ok",
		),
		NewChatState: &newState,
	}, nil
}

type trackingSessionMock struct {
	session *bot.ChatSession

	updateCalled bool
}

func (m *trackingSessionMock) SessionByChatID(
	chatID int64,
) (*bot.ChatSession, error) {

	return m.session, nil
}

func (m *trackingSessionMock) UpdateSession(
	chatID int64,
	new *bot.ChatSession,
) error {

	m.updateCalled = true
	return nil
}

func TestBot_SendErrorDoesNotSaveState(t *testing.T) {

	tg := &failingTelegramMock{
		updates: make(chan tgbotapi.Update, 1),
	}

	session := &trackingSessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
		},
	}

	b := bot.New(session, tg)

	b.AddHandlerForCommand(
		"/start",
		&stateChangingHandler{},
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	assert.False(
		t,
		session.updateCalled,
	)
}

type concurrentHandler struct {
	mu sync.Mutex

	currentHandling int

	maxHandling int
}

func (c *concurrentHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {

	c.mu.Lock()
	c.currentHandling++
	if c.currentHandling > c.maxHandling {
		c.maxHandling = c.currentHandling
	}
	c.mu.Unlock()

	time.Sleep(25 * time.Millisecond)

	c.mu.Lock()
	c.currentHandling--
	c.mu.Unlock()

	return bot.Response{
		Message: tgbotapi.NewMessage(
			u.ChatID,
			"ok",
		),
	}, nil

}

func TestBot_ChatsCanBeHandledByOnlyOneHandlerAtATime(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: &bot.ChatSession{
			ChatID: 123,
		},
	}

	b := bot.New(session, tg)

	handler := &concurrentHandler{}

	b.AddHandlerForCommand(
		"/start",
		handler,
	)

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update, update)

	assert.Equal(t, 1, handler.maxHandling)
}
