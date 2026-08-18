package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const defaultWorkerPoolSize = 20

const ChatSessionKey = "chat_session_key"

var ErrInternalServer = fmt.Errorf("Internal Server Error")
var ErrUnknown = fmt.Errorf("Unknown error while handling message")

type Update struct {
	// [ChatID], [Text], [CallbackData] - common fields for trivial updates
	ChatID       int64
	Text         string
	CallbackData string

	// [Raw] - for more complex updates
	Raw tgbotapi.Update
}

func extractUpdate(u tgbotapi.Update) Update {
	var update Update
	update.Raw = u
	chat := u.FromChat()
	if chat == nil {
		return Update{}
	}
	update.ChatID = chat.ID
	update.CallbackData = u.CallbackData()
	switch {
	case u.Message != nil:
		update.Text = u.Message.Text
	case u.EditedMessage != nil:
		update.Text = u.EditedMessage.Text
	}
	return update
}

type Response struct {
	Message tgbotapi.Chattable
}

type Command string

type ChatState string

var DefaultChatState = ChatState("")

type ChatSession struct {
	chatID           int64
	state            ChatState
	lastBotMessageID int
	payload          json.RawMessage
}

func NewChatSession(chatID int64) *ChatSession {
	return &ChatSession{
		chatID: chatID,
	}
}

func (s *ChatSession) ChatID() int64 {
	return s.chatID
}

func (s *ChatSession) SetState(state ChatState) {
	s.state = state
}

func (s *ChatSession) State() ChatState {
	return s.state
}

func (s *ChatSession) ToDefault() {
	s.state = DefaultChatState
	s.payload = nil
}

func (s *ChatSession) SetPayload(p json.RawMessage) {
	s.payload = p
}

func (s *ChatSession) Payload() json.RawMessage {
	return s.payload
}
func (s *ChatSession) LastBotMessageID() int {
	return s.lastBotMessageID
}

func (s *ChatSession) EditMsgAvailable() bool {
	return s.lastBotMessageID > 0
}

type ChatSessionService interface {
	SessionByChatID(chatID int64) (*ChatSession, error)
	UpdateSession(new *ChatSession) error
}

type Handler interface {
	Handle(context.Context, *ChatSession, Update) (Response, error)
	Match(context.Context, *ChatSession, Update) bool
}

type HandlerFunc struct {
	HandleFunc func(context.Context, *ChatSession, Update) (Response, error)
	MatchFunc  func(context.Context, *ChatSession, Update) bool
}

var _ Handler = HandlerFunc{}

func (f HandlerFunc) Handle(ctx context.Context, s *ChatSession, u Update) (Response, error) {
	return f.HandleFunc(ctx, s, u)
}

func (f HandlerFunc) Match(ctx context.Context, s *ChatSession, u Update) bool {
	return f.MatchFunc(ctx, s, u)
}

type Middleware func(next Handler) Handler

type group struct {
	handlers    []Handler
	middlewares []Middleware
}

func NewGroup(handlers ...Handler) *group {
	return &group{
		handlers: handlers,
	}
}

func (g *group) Use(middlewares ...Middleware) {
	g.middlewares = append(g.middlewares, middlewares...)
}

type ErrorHandler interface {
	Match(err error) bool
	Handle(context.Context, *ChatSession, Update, error) Response
}

func defaultErrorHandle(chatID int64, err error) Response {
	return Response{
		Message: tgbotapi.NewMessage(chatID, ErrUnknown.Error()),
	}
}

func (b *Bot) errHandle(ctx context.Context, s *ChatSession, u Update, err error) Response {
	for _, h := range b.errorHandlers {
		if h.Match(err) {
			return h.Handle(ctx, s, u, err)
		}
	}
	return defaultErrorHandle(u.ChatID, err)
}

type TelegramBotClient interface {
	GetUpdatesChan() <-chan tgbotapi.Update
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}

// [Bot] is a structure responsible for
// dispatching updates and errors
// to the appropriate handlers.
// Updates are dispatched based on
// commands or the chat state, which
// is set by the bot's client.
type Bot struct {
	workerPoolSize int
	tgBot          TelegramBotClient

	sessionService ChatSessionService

	groups []*group

	errorHandlers []ErrorHandler

	globalMiddlewares []Middleware

	log *slog.Logger
}

func New(sessionService ChatSessionService, botClient TelegramBotClient, log *slog.Logger, opts ...option) *Bot {
	bot := &Bot{
		workerPoolSize:    defaultWorkerPoolSize,
		tgBot:             botClient,
		sessionService:    sessionService,
		errorHandlers:     make([]ErrorHandler, 0),
		groups:            make([]*group, 0),
		globalMiddlewares: make([]Middleware, 0),
		log:               log,
	}
	for _, opt := range opts {
		opt(bot)
	}
	return bot
}

func (b *Bot) AddHandler(h Handler, m ...Middleware) {
	g := NewGroup(h)
	g.Use(m...)
	b.groups = append(b.groups, g)
}

func (b *Bot) AddGroup(g *group) {
	b.groups = append(b.groups, g)
}

func (b *Bot) Use(m Middleware) {
	b.globalMiddlewares = append(b.globalMiddlewares, m)
}

func (b *Bot) handler(ctx context.Context, s *ChatSession, u Update) Handler {
	h, g := func() (Handler, *group) {
		for _, g := range b.groups {
			for _, h := range g.handlers {
				if h.Match(ctx, s, u) {
					return h, g
				}
			}
		}
		return nil, nil
	}()
	if h == nil || g == nil {
		return nil
	}
	for i := len(g.middlewares) - 1; i >= 0; i-- {
		h = g.middlewares[i](h)
	}
	for i := len(b.globalMiddlewares) - 1; i >= 0; i-- {
		h = b.globalMiddlewares[i](h)
	}
	return h
}

func (b *Bot) SetChatSessionService(s ChatSessionService) {
	b.sessionService = s
}

func (b *Bot) AddErrorHandler(h ErrorHandler) {
	b.errorHandlers = append(b.errorHandlers, h)
}

func (b *Bot) handle(ctx context.Context, u tgbotapi.Update) {
	const op = "Bot.handle"
	log := b.log.With("chat_id", u.FromChat().ID, "msg_id", u.UpdateID)

	var resp Response

	update := extractUpdate(u)
	if update == (Update{}) {
		return
	}

	chatID := update.ChatID

	session, err := b.sessionService.SessionByChatID(chatID)
	if err != nil {
		err := fmt.Errorf("%v: %w: %w", op, ErrInternalServer, err)
		log.Error("error while getting session", "error", err)

		_, err = b.tgBot.Send(tgbotapi.NewMessage(chatID, ErrInternalServer.Error()))
		if err != nil {
			log.Error("error while sending error message", "error", err)
		}
		return
	}

	handler := b.handler(ctx, session, update)
	if handler == nil {
		log.Debug("no suitable handler found")
		return
	}

	ctx = context.WithValue(ctx, ChatSessionKey, session)
	resp, err = handler.Handle(ctx, session, update)
	if err != nil {
		log.Debug("handler error", "error", err)
		resp = b.errHandle(ctx, session, update, err)
	}

	_, err = b.tgBot.Send(resp.Message)

	// If an error occurs while sending a response, the program does not save the new session state.
	if err != nil {
		log.Error("error while sending message", "error", err)
		return
	}

	err = b.sessionService.UpdateSession(session)
	if err != nil {
		log.Error("error while updating session", "error", err)
	}
}

type chatLock struct {
	mu   sync.Mutex
	refs int
}

type chatLocks struct {
	mu    sync.Mutex
	locks map[int64]*chatLock
}

func (c *chatLocks) lock(chatID int64) (unlock func()) {
	c.mu.Lock()

	l, ok := c.locks[chatID]
	if !ok {
		l = &chatLock{}
		c.locks[chatID] = l
	}

	l.refs++
	c.mu.Unlock()

	l.mu.Lock()

	unlock = func() {
		l.mu.Unlock()

		c.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(c.locks, chatID)
		}
		c.mu.Unlock()
	}
	return
}

func (b *Bot) StartListening(ctx context.Context) {
	b.log.Info("bot starting listening")
	if b.sessionService == nil {
		panic("Bot needs ChatSessionService - a service to retrieve and save the chat session state")
	}

	locks := &chatLocks{
		mu:    sync.Mutex{},
		locks: make(map[int64]*chatLock),
	}

	workerChans := make(map[int]chan tgbotapi.Update)
	for i := range b.workerPoolSize {
		workerChans[i] = make(chan tgbotapi.Update)
		defer close(workerChans[i])
	}
	updates := b.tgBot.GetUpdatesChan()
	done := ctx.Done()
	for i := range b.workerPoolSize {
		go func() {
			for {
				select {
				case <-done:
					return
				case u, ok := <-workerChans[i]:
					b.log.Debug("update from chan get")
					if !ok {
						return
					}
					chat := u.FromChat()
					if chat == nil {
						continue
					}
					unlock := locks.lock(chat.ID)
					b.log.Debug("locked")
					b.handle(ctx, u)
					b.log.Debug("unlocked")
					unlock()

				}
			}
		}()
	}
	for {
		select {
		case <-done:
			return
		case u := <-updates:
			chat := u.FromChat()
			if chat == nil {
				continue
			}
			select {
			case workerChans[int(chat.ID)%b.workerPoolSize] <- u:
			case <-done:
				return
			}
		}
	}
}

type option func(*Bot)

func WithWorkers(n int) option {
	return option(func(b *Bot) {
		b.workerPoolSize = n
	})
}
