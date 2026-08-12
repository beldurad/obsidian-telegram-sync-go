package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ===== STATES =====

var StateGetTemplate bot.ChatState = "TEMPLATE_GET"

var (
	StateWaitTemplateValue = bot.ChatState("WAITING_TEMPLATE_VALUE")
	StateWaitTemplateName  = bot.ChatState("WAITING_TEMPLATE_NAME")
)

// ===== COMMANDS =====

const (
	CommandGetTemplates = "/template"
	CommandAddTemplate  = "/add-template"
)

// ===== GET TEMPLATE =====

type TemplateGetter interface {
	TemplatesPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Template], error)
}

type GetTemplateHandler struct {
	getter TemplateGetter
}

func NewGetTemplateHandler(getter TemplateGetter) *GetTemplateHandler {
	return &GetTemplateHandler{
		getter: getter,
	}
}

func (h *GetTemplateHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	chatSession := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)

	var payload pagePayload

	if chatSession.State != bot.DefaultChatState {
		raw := chatSession.Payload
		err := json.Unmarshal([]byte(raw), &payload)
		if err != nil {
			return bot.Response{}, err
		}
		switch u.Raw.CallbackData() {
		case NextPageCommand:
			payload.PageNum++
		case PrevPageCommand:
			payload.PageNum--
		}
	}

	templatesPage, err := h.getter.TemplatesPage(
		ctx,
		chatSession.ChatID,
		payload.PageNum,
		domain.DefaultPageSize,
	)
	if err != nil {
		return bot.Response{}, err
	}

	textBuilder := []rune("Ваши текстовые шаблоны\n")
	for _, template := range templatesPage.Values {
		textBuilder = append(
			textBuilder,
			[]rune(
				fmt.Sprintf(`
				%s: %s\n
				`,
					template.Name,
					template.Value,
				),
			)...)
	}

	if len(templatesPage.Values) == 0 {
		textBuilder = append(textBuilder,
			[]rune(`
			У вас нет шаблонов. Создайте свой первый шаблон с помощью /add-template
			`,
			)...)
	}

	buttons := tgbotapi.NewInlineKeyboardRow()
	if templatesPage.HasNext() {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Next", NextPageCommand))
	}
	if templatesPage.HasPrev() {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Prev", PrevPageCommand))
	}

	replyMarkup := tgbotapi.NewInlineKeyboardMarkup(
		buttons,
	)

	var c tgbotapi.Chattable

	if chatSession.State == bot.DefaultChatState || chatSession.LastBotMessageID == 0 {
		msgCfg := tgbotapi.NewMessage(chatSession.ChatID, string(textBuilder))
		msgCfg.ReplyMarkup = replyMarkup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageCaption(chatSession.ChatID, chatSession.LastBotMessageID, string(textBuilder))
		msgCfg.ReplyMarkup = &replyMarkup
		c = msgCfg
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}

	return bot.Response{
		Message:      c,
		NewChatState: &StateGetTemplate,
		NewPayload:   bytes,
	}, nil
}

// ===== CREATE TEMPLATE

type TemplateSaver interface {
	Save(ctx context.Context, t domain.Template) error
}

type TemplateAddHandler struct {
	saver TemplateSaver
}

func NewTemplateAddHandler(saver TemplateSaver) *TemplateAddHandler {
	return &TemplateAddHandler{
		saver: saver,
	}
}

type templateBuilder struct {
	ID        string `json:"id"`
	ChatID    int64  `json:"chat_id"`
	Value     string `json:"value"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func newTemplateBuilder(chatID int64) templateBuilder {
	return templateBuilder{
		ID:        uuid.NewString(),
		ChatID:    chatID,
		CreatedAt: time.Now().Format(time.DateOnly),
	}
}

func (p templateBuilder) toTemplate() (domain.Template, error) {
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return domain.Template{}, err
	}
	createdAt, err := time.Parse(time.DateOnly, p.CreatedAt)
	if err != nil {
		return domain.Template{}, err
	}
	return domain.Template{
		ID:        id,
		ChatID:    p.ChatID,
		Name:      p.Name,
		Value:     p.Value,
		CreatedAt: createdAt,
	}, nil
}

func (a *TemplateAddHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	chatSession := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)
	chatID := chatSession.ChatID
	state := chatSession.State
	payload := chatSession.Payload

	if state == StateWaitTemplateValue {
		return a.handleValue(chatID, u.Text, payload)
	}
	if state == StateWaitTemplateName {
		return a.handleName(ctx, chatID, u.Text, payload)
	}
	return a.handleDefault(chatID)
}

func (a *TemplateAddHandler) handleDefault(chatID int64) (bot.Response, error) {
	payload := newTemplateBuilder(chatID)
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message: tgbotapi.NewMessage(
			chatID,
			"Введите шаблон, используя {} как плейсхолдер",
		),
		NewChatState: &StateWaitTemplateValue,
		NewPayload:   bytes,
	}, nil
}

func (a *TemplateAddHandler) handleValue(chatID int64, value string, rawPayload json.RawMessage) (bot.Response, error) {
	var payload templateBuilder
	err := json.Unmarshal([]byte(rawPayload), &payload)
	if err != nil {
		return bot.Response{}, err
	}
	payload.Value = value
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message: tgbotapi.NewMessage(
			chatID,
			`
			Введите название шаблона
			`,
		),
		NewChatState: &StateWaitTemplateName,
		NewPayload:   bytes,
	}, nil
}

func (a *TemplateAddHandler) handleName(ctx context.Context, chatID int64, name string, rawPayload json.RawMessage) (bot.Response, error) {
	var payload templateBuilder
	err := json.Unmarshal([]byte(rawPayload), &payload)
	if err != nil {
		return bot.Response{}, err
	}
	payload.Name = name
	template, err := payload.toTemplate()
	if err != nil {
		return bot.Response{}, err
	}
	err = a.saver.Save(ctx, template)
	if err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message: tgbotapi.NewMessage(
			chatID,
			`
			Шаблон сохранен
			`,
		),
	}, nil
}
