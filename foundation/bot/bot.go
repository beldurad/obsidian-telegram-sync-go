package bot

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const goroutinesPoolSize = 20

const ChatSessionKey = "session"

type Update struct {
	// [ChatID], [Text], [ButtonPressed] - common fields for trivial updates
	ChatID        int64
	Text          string
	ButtonPressed bool

	// [Update] - for more complex updates
	Raw tgbotapi.Update
}

type Response struct {
	Message tgbotapi.Chattable

	// New chat state resulting from the update handling
	NewChatState ChatState
}

type Command string

type commonKey struct {
	ChatState
	Command
}

type ChatState string

const DefaultChatState = ""

type ChatSession struct {
	ChatID           int64
	State            ChatState
	LastBotMessageID int
}

type ChatSessionService interface {
	SessionByChatID(chatID int64) ChatSession
	UpdateSession(chatID int64, new ChatSession)
}

type Handler interface {
	Handle(context.Context, Update) (Response, error)
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
	Handle(chatID int64, err error) Response
}

func defaultErrorHandle(chatID int64) Response {
	return Response{
		Message: tgbotapi.NewMessage(chatID, "Unknown error while handling message"),
	}
}

func (b *Bot) errHandle(chatID int64, err error) Response {
	for _, h := range b.errorHandlers {
		if h.Match(err) {
			return h.Handle(chatID, err)
		}
	}
	return defaultErrorHandle(chatID)
}

type TelegramBotClient interface {
	GetUpdatesChan() chan tgbotapi.Update
	Send(tgbotapi.Chattable) (tgbotapi.Message, error)
}

// [Bot] is a structure responsible for
// dispatching updates and errors
// to the appropriate handlers.
// Updates are dispatched based on
// commands or the chat state, which
// is set by the bot's client.
type Bot struct {
	tgBot TelegramBotClient

	sessionService ChatSessionService

	errorHandlers []ErrorHandler

	// Maps for resolving update [Handler]
	byState   map[ChatState]Handler
	byCommand map[Command]Handler
	byBoth    map[commonKey]Handler
}

func New(token string, sessionService ChatSessionService, botClient TelegramBotClient) *Bot {
	return &Bot{
		tgBot:          botClient,
		sessionService: sessionService,
		errorHandlers:  make([]ErrorHandler, 0),
		byState:        make(map[ChatState]Handler),
		byCommand:      make(map[Command]Handler),
		byBoth:         make(map[commonKey]Handler),
	}
}

func (b *Bot) AddHandlerForCommand(c Command, h Handler, m ...Middleware) {
	b.byCommand[c] = merge(h, m...)
}

func (b *Bot) AddHandlerForState(s ChatState, h Handler, m ...Middleware) {
	b.byState[s] = merge(h, m...)
}

func (b *Bot) AddHandler(c Command, s ChatState, h Handler, m ...Middleware) {
	k := commonKey{
		ChatState: s,
		Command:   c,
	}
	b.byBoth[k] = merge(h, m...)
}

func (b *Bot) resolveHandler(c Command, s ChatState) Handler {
	key := commonKey{
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
	chat := u.FromChat()
	if chat == nil {
		return
	}
	var update Update
	update.Raw = u
	update.ChatID = chat.ID
	if u.CallbackQuery != nil {
		update.ButtonPressed = true
		update.Text = u.CallbackData()
	} else if u.Message != nil {
		update.Text = u.Message.Text
	} else {
		return
	}
	session := b.sessionService.SessionByChatID(chat.ID)
	state := session.State

	handler := b.resolveHandler(Command(update.Text), state)
	if handler == nil {
		return
	}
	ctx = context.WithValue(ctx, ChatSessionKey, session)
	resp, err := handler.Handle(ctx, update)
	if err != nil {
		resp = b.errHandle(chat.ID, err)
	}

	msg, err := b.tgBot.Send(resp.Message)

	var newState ChatState
	if err != nil {
		newState = DefaultChatState
	} else {
		session.LastBotMessageID = msg.MessageID
		newState = resp.NewChatState
	}

	if newState != session.State || err == nil {
		session.State = newState
		b.sessionService.UpdateSession(chat.ID, session)
	}
}

func (b *Bot) StartListening(ctx context.Context) {
	if b.sessionService == nil {
		panic("Bot needs ChatSessionService - a service to retrieve and save the chat session state")
	}

	updates := b.tgBot.GetUpdatesChan()
	done := ctx.Done()

	jobs := make(chan tgbotapi.Update, 100)

	for range goroutinesPoolSize {
		go func() {
			var u tgbotapi.Update
			for {
				select {
				case <-done:
					return
				case u = <-jobs:
					b.handle(ctx, u)

				}
			}
		}()
	}
	for {
		var u tgbotapi.Update
		select {
		case <-done:
			return
		case u = <-updates:
			jobs <- u
		}
	}
}
