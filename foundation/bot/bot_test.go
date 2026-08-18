package bot_test

import (
	"context"
	"errors"
	"log/slog"
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

	msgID   int
	sendErr error
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

	if m.sendErr != nil {
		return tgbotapi.Message{}, m.sendErr
	}

	if m.msgID == 0 {
		m.msgID = 100
	}

	m.sent = append(m.sent, c)

	return tgbotapi.Message{
		MessageID: m.msgID,
	}, nil
}

func (m *telegramMock) Sent() []tgbotapi.Chattable {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.sent
}

type sessionMock struct {
	session      *bot.ChatSession
	sessionErr   error
	updateCalled bool

	updated chan *bot.ChatSession
}

func (m *sessionMock) SessionByChatID(chatID int64) (*bot.ChatSession, error) {
	return m.session, m.sessionErr
}

func (m *sessionMock) UpdateSession(new *bot.ChatSession) error {
	m.updateCalled = true

	select {
	case m.updated <- new:
	default:
	}
	return nil
}

type handlerMock struct {
	called   chan struct{}
	response bot.Response
	onHandle func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error)
}

func (h *handlerMock) Handle(
	ctx context.Context,
	s *bot.ChatSession,
	u bot.Update,
) (bot.Response, error) {
	if h.onHandle != nil {
		return h.onHandle(ctx, s, u)
	}

	select {
	case h.called <- struct{}{}:
	default:
	}

	if h.response.Message != nil {
		return h.response, nil
	}

	return bot.Response{
		Message: tgbotapi.NewMessage(u.ChatID, "ok"),
	}, nil
}

func newHandlerMock() *handlerMock {
	return &handlerMock{
		called: make(chan struct{}, 1),
	}
}

type errorHandlerMock struct {
	matched     bool
	called      chan struct{}
	matchResult bool
	response    bot.Response
}

func (h *errorHandlerMock) Handle(
	ctx context.Context,
	s *bot.ChatSession,
	u bot.Update,
	err error,
) bot.Response {
	select {
	case h.called <- struct{}{}:
	default:
	}

	return h.response
}

func (h *errorHandlerMock) Match(err error) bool {
	h.matched = true
	return h.matchResult
}

func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatal("expected signal was not received")
	}
}

func assertNotSignaled(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal("unexpected signal received")
	default:
	}
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
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	handler := newHandlerMock()
	var capturedUpdate bot.Update

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			capturedUpdate = u
			return u.Text == "/start"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, handler.called)
	assert.Equal(t, int64(123), capturedUpdate.ChatID)
}

func TestBot_StateHandler(t *testing.T) {

	tg := newTelegramMock()

	testState := bot.ChatState("waiting_name")

	session := &sessionMock{
		session: func() *bot.ChatSession {
			s := bot.NewChatSession(123)
			s.SetState(testState)
			return s
		}(),
	}

	b := bot.New(session, tg, slog.Default())

	handler := newHandlerMock()

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return s.State() == testState
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "Alex",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, handler.called)
}

func TestBot_BothHandlerHasPriority(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: func() *bot.ChatSession {
			s := bot.NewChatSession(123)
			s.SetState("register")
			return s
		}(),
	}

	b := bot.New(session, tg, slog.Default())

	commandHandler := newHandlerMock()
	stateHandler := newHandlerMock()
	bothHandler := newHandlerMock()

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: bothHandler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/start" && s.State() == "register"
		},
	})

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: commandHandler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/start"
		},
	})

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: stateHandler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return s.State() == "register"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, bothHandler.called)

	assertNotSignaled(t, commandHandler.called)
	assertNotSignaled(t, stateHandler.called)
}

func TestBot_StateFallbackWhenBothMissing(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: func() *bot.ChatSession {
			s := bot.NewChatSession(123)
			s.SetState("register")
			return s
		}(),
	}

	b := bot.New(session, tg, slog.Default())

	stateHandler := newHandlerMock()

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: stateHandler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return s.State() == "register"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/unknown",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, stateHandler.called)
}

func TestBot_SessionChangesAfterHandle(t *testing.T) {

	newState := bot.ChatState("waiting")

	tg := newTelegramMock()

	session := &sessionMock{
		session: bot.NewChatSession(123),
		updated: make(chan *bot.ChatSession, 1),
	}

	b := bot.New(session, tg, slog.Default())

	handler := newHandlerMock()
	handler.onHandle = func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
		s.SetState(newState)
		return bot.Response{
			Message: tgbotapi.NewMessage(123, "ok"),
		}, nil
	}

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/test"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/test",
		},
	}

	runBot(b, tg.updates, update)

	select {
	case updated := <-session.updated:
		assert.Equal(t, newState, updated.State())
	case <-time.After(1 * time.Second):
		t.Fatal("session was not updated")
	}
}

func TestBot_SessionError(t *testing.T) {

	tg := newTelegramMock()

	b := bot.New(
		&sessionMock{
			session:    bot.NewChatSession(123),
			sessionErr: errors.New("db error"),
		},
		tg,
		slog.Default(),
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

	require.Len(t, tg.Sent(), 1)

	msgCfg, ok := tg.Sent()[0].(tgbotapi.MessageConfig)
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
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	expectedText := "custom error"

	eh := &errorHandlerMock{
		called:      make(chan struct{}, 1),
		matchResult: true,
		response: bot.Response{
			Message: tgbotapi.NewMessage(
				123,
				expectedText,
			),
		},
	}

	b.AddErrorHandler(eh)

	handler := newHandlerMock()
	handler.onHandle = func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
		return bot.Response{}, errors.New("boom")
	}

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/start"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, eh.called)

	require.Len(t, tg.Sent(), 1)

	msgCfg := tg.Sent()[0].(tgbotapi.MessageConfig)

	assert.Equal(
		t,
		expectedText,
		msgCfg.Text,
	)
}

func TestBot_DefaultErrorHandler(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	b.AddErrorHandler(&errorHandlerMock{
		called:      make(chan struct{}, 1),
		matchResult: false,
	})

	handler := newHandlerMock()
	handler.onHandle = func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
		return bot.Response{}, errors.New("boom")
	}

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/start"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	require.Len(t, tg.Sent(), 1)

	msgCfg := tg.Sent()[0].(tgbotapi.MessageConfig)

	assert.Equal(
		t,
		bot.ErrUnknown.Error(),
		msgCfg.Text,
	)
}

func TestBot_SendErrorDoesNotSaveState(t *testing.T) {

	tg := newTelegramMock()
	tg.sendErr = errors.New("telegram error")

	session := &sessionMock{
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	handler := newHandlerMock()
	handler.onHandle = func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
		s.SetState("next")
		return bot.Response{
			Message: tgbotapi.NewMessage(u.ChatID, "ok"),
		}, nil
	}

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/start"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update)

	assert.False(t, session.updateCalled)
}

type concurrentHandler struct {
	mu sync.Mutex

	currentHandling int

	maxHandling int
}

func (c *concurrentHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {

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

func (c *concurrentHandler) MaxHandling() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.maxHandling
}

func TestBot_EditedMessage(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	handler := newHandlerMock()
	var capturedText string

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			capturedText = u.Text
			return u.Text == "edited text"
		},
	})

	update := tgbotapi.Update{
		EditedMessage: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "edited text",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, handler.called)
	assert.Equal(t, "edited text", capturedText)
}

func TestBot_CallbackQuery(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	handler := newHandlerMock()
	var capturedCallbackData string

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			capturedCallbackData = u.CallbackData
			return u.CallbackData == "action_confirm"
		},
	})

	update := tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{
					ID: 123,
				},
			},
			Data: "action_confirm",
		},
	}

	runBot(b, tg.updates, update)

	waitSignal(t, handler.called)
	assert.Equal(t, "action_confirm", capturedCallbackData)
}

func TestBot_ChatsCanBeHandledByOnlyOneHandlerAtATime(t *testing.T) {

	tg := newTelegramMock()

	session := &sessionMock{
		session: bot.NewChatSession(123),
	}

	b := bot.New(session, tg, slog.Default())

	handler := &concurrentHandler{}

	b.AddHandler(bot.HandlerFunc{
		HandleFunc: handler.Handle,
		MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
			return u.Text == "/start"
		},
	})

	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{
				ID: 123,
			},
			Text: "/start",
		},
	}

	runBot(b, tg.updates, update, update)

	assert.Equal(t, 1, handler.MaxHandling())
}
