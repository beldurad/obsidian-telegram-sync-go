package bot

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const goroutinesPoolSize = 20

type Update struct {
	ChatID        int64
	Text          string
	ButtonPressed bool
}

type Button string

type Response struct {
	Text         string
	Buttons      [][]Button
	EditPrevMsg  bool
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
	lastBotMessageID int
}

type ChatSessionService interface {
	SessionByChatID(chatID int64) ChatSession
	UpdateSession(chatID int64, new ChatSession)
}

type Handler interface {
	Handle(context.Context, Update) (error, Response)
}

type Middleware func(next Handler) Handler

func merge(h Handler, middlewares ...Middleware) Handler {
	cur := h
	for _, m := range middlewares {
		cur = m(cur)
	}
	return cur
}

type ErrorHandlerFunc func(err error) Response

func defaultErrorHandle() ErrorHandlerFunc {
	return func(err error) Response {
		return Response{
			Text: "Unknown error while handling update",
		}
	}
}

type Bot struct {
	tgBot *tgbotapi.BotAPI

	ChatSessionService

	errorHandlers map[error]ErrorHandlerFunc

	// Maps for resolving update [Handler]
	byState   map[ChatState]Handler
	byCommand map[Command]Handler
	byBoth    map[commonKey]Handler
}

func New(token string) *Bot {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		panic(err)
	}
	return &Bot{
		tgBot: bot,
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
	b.ChatSessionService = s
}

func (b *Bot) AddErrorHandler(err error, h ErrorHandlerFunc) {
	b.errorHandlers[err] = h
}

func (b *Bot) handle(u tgbotapi.Update) {
	chat := u.FromChat()
	if chat == nil {
		return
	}
	var update Update
	update.ChatID = chat.ID
	if u.CallbackQuery != nil {
		update.ButtonPressed = true
		update.Text = u.CallbackData()
	} else if u.Message != nil {
		update.Text = u.Message.Text
	} else {
		return
	}
	session := b.ChatSessionService.SessionByChatID(chat.ID)
	state := session.State

	handler := b.resolveHandler(Command(update.Text), state)
	if handler == nil {
		return
	}
	ctx := context.Background()
	err, resp := handler.Handle(ctx, update)
	if err != nil {
		if errorHandler, ok := b.errorHandlers[err]; ok {
			resp = errorHandler(err)
		} else {
			defHandler := defaultErrorHandle()
			resp = defHandler(err)
		}
	}
	var msgCfg tgbotapi.Chattable
	if resp.EditPrevMsg && session.lastBotMessageID > 0 {
		msgCfg = tgbotapi.NewEditMessageText(chat.ID, int(session.lastBotMessageID), resp.Text)
	} else {
		msgCfg = tgbotapi.NewMessage(chat.ID, resp.Text)
	}

	msg, err := b.tgBot.Send(msgCfg)
	if err != nil {
		session.State = DefaultChatState
	} else {
		session.lastBotMessageID = msg.MessageID
		session.State = resp.NewChatState
	}
	b.ChatSessionService.UpdateSession(chat.ID, session)

}

func (b *Bot) StartListening() {
	goroutinesPool := make(chan int, goroutinesPoolSize)
	for u := range b.tgBot.GetUpdatesChan(tgbotapi.NewUpdate(0)) {
		goroutinesPool <- 1
		go func() {
			b.handle(u)
			<-goroutinesPool
		}()
	}
}
