package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const goroutinesPoolSize = 20

type Update struct {
	ChatID    int64
	MessageID int64
	From      string
	Text      string
}

type HandleSupporter interface {
	Handler
	Supports(context.Context, Update) bool
}

type Handler interface {
	Handle(context.Context, Update) (tgbotapi.MessageConfig, error)
}

type HandleSupporterFunc struct {
	HandlerFunc
	SupportsFunc func(context.Context, Update) bool
}

func (h HandleSupporterFunc) Handle(ctx context.Context, u Update) (tgbotapi.MessageConfig, error) {
	return h.HandlerFunc.Handle(ctx, u)
}
func (h HandleSupporterFunc) Supports(ctx context.Context, u Update) bool {
	return h.SupportsFunc(ctx, u)
}

type HandlerFunc func(context.Context, Update) (tgbotapi.MessageConfig, error)

func (h HandlerFunc) Handle(ctx context.Context, u Update) (tgbotapi.MessageConfig, error) {
	return h(ctx, u)
}

type Middleware func(next Handler) Handler

type Bot struct {
	*tgbotapi.BotAPI
	handlers []HandleSupporter
}

func New(token string) *Bot {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		panic(err)
	}
	return &Bot{
		BotAPI: bot,
	}
}

func (d *Bot) RegisterHandler(h HandleSupporter, middlewares ...Middleware) {
	handler := func(ctx context.Context, u Update) (tgbotapi.MessageConfig, error) {
		var curHandler Handler = HandlerFunc(h.Handle)
		for i := len(middlewares) - 1; i >= 0; i-- {
			curHandler = middlewares[i](curHandler)
		}
		return curHandler.Handle(ctx, u)
	}
	supporter := func(ctx context.Context, u Update) bool {
		return h.Supports(ctx, u)
	}

	d.handlers = append(d.handlers, HandleSupporterFunc{
		HandlerFunc:  handler,
		SupportsFunc: supporter,
	})
}

func (d *Bot) StartListening() {
	goroutinesPool := make(chan int, goroutinesPoolSize)
	log.Println("BOT STARTS LISTENING")
	for u := range d.GetUpdatesChan(tgbotapi.NewUpdate(0)) {
		goroutinesPool <- 1
		go func() {
			d.Handle(u)
			<-goroutinesPool
		}()
	}
}

func (d *Bot) Handle(u tgbotapi.Update) {

	ctx := context.Background()
	appUpdate := Update{}
	switch {
	case u.Message != nil:
		appUpdate.ChatID = u.Message.Chat.ID
		appUpdate.From = u.Message.From.UserName
		appUpdate.Text = u.Message.Text
		appUpdate.MessageID = int64(u.Message.MessageID)
	case u.CallbackQuery != nil:
		appUpdate.ChatID = u.CallbackQuery.Message.Chat.ID
		appUpdate.From = u.CallbackQuery.Message.From.UserName
		appUpdate.Text = u.CallbackQuery.Data
		appUpdate.MessageID = int64(u.CallbackQuery.Message.MessageID)
	default:
		return
	}
	log.Printf("HANDLING MESSAGE: id=%d, text=%s", appUpdate.MessageID, appUpdate.Text)
	var h HandleSupporter
	for _, handler := range d.handlers {
		if handler.Supports(ctx, appUpdate) {
			h = handler
			break
		}
	}
	if h == nil {
		// TODO: обработку ошибки при отсутствии нужного обработчика
	}
	msgCfg, err := h.Handle(ctx, appUpdate)
	if err != nil {
		// TODO: обработку ошибки, возникшей при обработке запроса обработчиком
	}
	d.BotAPI.Send(msgCfg)
}
