package app

type UserMessage struct {
	ChatID int
	From   string
	Text   string
}

type BotResponse struct {
	ChatID int    `json:"chat_id"`
	Text   string `json:"text"`
	Err    error
}

type MessageHandler interface {
	Handle(UserMessage) BotResponse
	Supports(UserMessage) bool
}

type Bot struct {
	messages  <-chan UserMessage
	handlers  []MessageHandler
	responses chan BotResponse
}

func NewBot(messages <-chan UserMessage) *Bot {
	return &Bot{
		messages:  messages,
		handlers:  make([]MessageHandler, 0),
		responses: make(chan BotResponse),
	}
}

func (b *Bot) RegiserHandler(h MessageHandler) {
	b.handlers = append(b.handlers, h)
}

func (b *Bot) ListenToMessages() {
	for m := range b.messages {
		go b.Handle(m)
	}
}

func (b *Bot) Responses() <-chan BotResponse {
	return b.responses
}

func (b *Bot) Handle(m UserMessage) {
	for _, h := range b.handlers {
		if h.Supports(m) {
			b.responses <- h.Handle(m)
			return
		}
	}
	b.responses <- BotResponse{
		ChatID: m.ChatID,
		Text:   "I can't understand you",
	}
}
