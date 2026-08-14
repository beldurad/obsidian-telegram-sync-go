package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const defaultWorkerPoolSize = 20

const ChatSessionKey = "chat_session_key"

var ErrInternalServer = fmt.Errorf("Internal Server Error")
var ErrUnknown = fmt.Errorf("Unknown error while handling message")

type Update struct {
	// [ChatID], [Text], [ButtonPressed] - common fields for trivial updates
	ChatID        int64
	Text          string
	ButtonPressed bool

	// [Raw] - for more complex updates
	Raw tgbotapi.Update
}

func extractUpdate(u tgbotapi.Update) Update {
	var update Update
	update.Raw = u
	chat := u.FromChat()
	if chat == nil {
		return update
	}
	update.ChatID = chat.ID
	if u.CallbackQuery != nil {
		update.ButtonPressed = true
		update.Text = u.CallbackData()
	} else if u.Message != nil {
		update.Text = u.Message.Text
	}
	return update
}

type Response struct {
	Message tgbotapi.Chattable

	// New chat state resulting from the update handling
	NewChatState *ChatState

	NewPayload json.RawMessage
}

type Command string

type ChatState string

var DefaultChatState = ChatState("")

type ChatSession struct {
	ChatID           int64
	State            ChatState
	LastBotMessageID int
	Payload          json.RawMessage
}

func NewChatSession(chatID int64) *ChatSession {
	return &ChatSession{
		ChatID: chatID,
	}
}

type ChatSessionService interface {
	SessionByChatID(chatID int64) (*ChatSession, error)
	UpdateSession(chatID int64, new *ChatSession) error
}

type Handler interface {
	Handle(context.Context, Update) (Response, error)
}

type HandlerFunc func(context.Context, Update) (Response, error)

func (f HandlerFunc) Handle(ctx context.Context, u Update) (Response, error) {
	return f(ctx, u)
}

type Middleware func(next Handler) Handler

func merge(h Handler, middlewares ...Middleware) Handler {
	cur := h
	for i := len(middlewares) - 1; i >= 0; i-- {
		cur = middlewares[i](cur)
	}
	return cur
}

type ErrorHandler interface {
	Match(err error) bool
	Handle(context.Context, Update, error) Response
}

func defaultErrorHandle(chatID int64) Response {
	return Response{
		Message: tgbotapi.NewMessage(chatID, ErrUnknown.Error()),
	}
}

func (b *Bot) errHandle(ctx context.Context, u Update, err error) Response {
	for _, h := range b.errorHandlers {
		if h.Match(err) {
			return h.Handle(ctx, u, err)
		}
	}
	return defaultErrorHandle(u.ChatID)
}

type TelegramBotClient interface {
	GetUpdatesChan() <-chan tgbotapi.Update
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}

type handlerResolveKey struct {
	ChatState
	Command
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

	errorHandlers []ErrorHandler

	// Maps for resolving update [Handler]
	byState   map[ChatState]Handler
	byCommand map[Command]Handler
	byBoth    map[handlerResolveKey]Handler

	globalMiddlewares []Middleware
}

func New(sessionService ChatSessionService, botClient TelegramBotClient, opts ...option) *Bot {
	bot := &Bot{
		workerPoolSize:    defaultWorkerPoolSize,
		tgBot:             botClient,
		sessionService:    sessionService,
		errorHandlers:     make([]ErrorHandler, 0),
		byState:           make(map[ChatState]Handler),
		byCommand:         make(map[Command]Handler),
		byBoth:            make(map[handlerResolveKey]Handler),
		globalMiddlewares: make([]Middleware, 0),
	}
	for _, opt := range opts {
		opt(bot)
	}
	return bot
}

func (b *Bot) AddHandlerForCommand(c Command, h Handler, m ...Middleware) {
	b.byCommand[c] = merge(h, m...)
}

func (b *Bot) AddHandlerForState(s ChatState, h Handler, m ...Middleware) {
	b.byState[s] = merge(h, m...)
}

func (b *Bot) AddHandler(c Command, s ChatState, h Handler, m ...Middleware) {
	k := handlerResolveKey{
		ChatState: s,
		Command:   c,
	}
	b.byBoth[k] = merge(h, m...)
}

func (b *Bot) Use(m Middleware) {
	b.globalMiddlewares = append(b.globalMiddlewares, m)
}

func (b *Bot) resolveHandler(c Command, s ChatState) Handler {
	key := handlerResolveKey{
		Command:   c,
		ChatState: s,
	}
	if h, ok := b.byBoth[key]; ok {
		return h
	} else if h, ok = b.byState[s]; ok {
		return h
	} else if h, ok := b.byCommand[c]; ok {
		return h
	}
	return nil
}

func (b *Bot) SetChatSessionService(s ChatSessionService) {
	b.sessionService = s
}

func (b *Bot) AddErrorHandler(h ErrorHandler) {
	b.errorHandlers = append(b.errorHandlers, h)
}

func (b *Bot) handle(ctx context.Context, u tgbotapi.Update) {

	var resp Response

	chat := u.FromChat()
	if chat == nil {
		return
	}
	update := extractUpdate(u)
	if update.Text == "" {
		return
	}
	session, err := b.sessionService.SessionByChatID(chat.ID)

	if err != nil {
		resp = Response{
			Message: tgbotapi.NewMessage(chat.ID, ErrInternalServer.Error()),
		}
		b.tgBot.Send(resp.Message)
		return
	}

	handler := b.resolveHandler(Command(update.Text), session.State)
	if handler == nil {
		return
	}
	for i := len(b.globalMiddlewares) - 1; i >= 0; i-- {
		handler = b.globalMiddlewares[i](handler)
	}

	ctx = context.WithValue(ctx, ChatSessionKey, session)
	resp, err = handler.Handle(ctx, update)
	if err != nil {
		resp = b.errHandle(ctx, update, err)
	}

	msg, err := b.tgBot.Send(resp.Message)

	// If an error occurs while sending a response, the program does not save the new session state.
	if err != nil {
		return
	}

	session.LastBotMessageID = msg.MessageID
	if resp.NewChatState != nil {
		session.State = *resp.NewChatState
	}
	session.Payload = resp.NewPayload
	b.sessionService.UpdateSession(chat.ID, session)
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
					if !ok {
						return
					}
					chat := u.FromChat()
					if chat == nil {
						continue
					}
					unlock := locks.lock(chat.ID)
					b.handle(ctx, u)
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
