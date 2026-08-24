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

const StateGetTemplate = "TEMPLATE_GET"

const (
	StateWaitTemplateValue = "WAITING_TEMPLATE_VALUE"
	StateWaitTemplateName  = "WAITING_TEMPLATE_NAME"
)

// ===== COMMANDS =====

const (
	CommandGetTemplates = "/template"
	CommandAddTemplate  = "/add_template"
)

// ===== GET TEMPLATE =====

type TemplateGetter interface {
	TemplatesPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Template], error)
}

type GetTemplateHandler struct {
	getter TemplateGetter
}

var _ bot.Handler = &GetTemplateHandler{}

func NewGetTemplateHandler(getter TemplateGetter) *GetTemplateHandler {
	return &GetTemplateHandler{
		getter: getter,
	}
}

func (h *GetTemplateHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	if u.Text == CommandGetTemplates {
		s.ToDefault()
		return true
	}
	return s.State() == StateGetTemplate && u.CallbackData != ""
}

func (h *GetTemplateHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (resp bot.Response, err error) {
	const op = "GetTemplateHandler.Handle"

	defer func() {
		if err != nil {
			s.ToDefault()
		}
	}()

	chatID := s.ChatID()
	state := s.State()

	var payload pagePayload

	if state == StateGetTemplate {
		raw := s.Payload()
		err := json.Unmarshal(raw, &payload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
		}
		payload = payload.handlePageUpdate(u)
	}

	templatesPage, err := h.getter.TemplatesPage(
		ctx,
		chatID,
		payload.PageNum,
		domain.DefaultPageSize,
	)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting templates page: %w", op, err)
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

	buttons := pageButtons(templatesPage)

	replyMarkup := tgbotapi.NewInlineKeyboardMarkup(
		buttons,
	)

	var c tgbotapi.Chattable

	if state != StateGetTemplate || !s.EditMsgAvailable() {
		msgCfg := tgbotapi.NewMessage(chatID, string(textBuilder))
		msgCfg.ReplyMarkup = replyMarkup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageText(chatID, s.LastBotMessageID(), string(textBuilder))
		msgCfg.ReplyMarkup = &replyMarkup
		c = msgCfg
	}

	payload = payloadFromPage(templatesPage)
	bytes, err := json.Marshal(payload)
	if err != nil {
		resp, err = bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
		return
	}

	s.SetState(StateGetTemplate)
	s.SetPayload(bytes)
	return bot.Response{Message: c}, nil
}

// ===== CREATE TEMPLATE

type TemplateSaver interface {
	Save(ctx context.Context, t domain.Template) error
}

type TemplateAddHandler struct {
	saver TemplateSaver
}

var _ bot.Handler = &TemplateAddHandler{}

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
	const op = "toTemplate"
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return domain.Template{}, fmt.Errorf("%v: parsing id: %w", op, err)
	}
	createdAt, err := time.Parse(time.DateOnly, p.CreatedAt)
	if err != nil {
		return domain.Template{}, fmt.Errorf("%v: parsing created_at: %w", op, err)
	}
	return domain.Template{
		ID:        id,
		ChatID:    p.ChatID,
		Name:      p.Name,
		Value:     p.Value,
		CreatedAt: createdAt,
	}, nil
}

func (a *TemplateAddHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	if u.Text == CommandAddTemplate {
		s.ToDefault()
		return true
	}
	return (s.State() == StateWaitTemplateValue && u.Text != "") ||
		(s.State() == StateWaitTemplateName && u.Text != "")
}

func (a *TemplateAddHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (resp bot.Response, err error) {
	const op = "TemplateAddHandler.Handle"

	defer func() {
		if err != nil {
			s.ToDefault()
		}
	}()

	chatID := u.ChatID
	state := s.State()

	switch state {
	case StateWaitTemplateValue:
		resp, err = a.handleValue(chatID, u.Text, s)
	case StateWaitTemplateName:
		resp, err = a.handleName(ctx, chatID, u.Text, s)
	default:
		resp, err = a.handleDefault(s, u)
	}

	if err != nil {
		err = fmt.Errorf("%v: %w", op, err)
	}
	return
}

func (a *TemplateAddHandler) handleDefault(s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handleDefault"

	payload := newTemplateBuilder(u.ChatID)
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling builder: %w", op, err)
	}
	s.SetState(StateWaitTemplateValue)
	s.SetPayload(bytes)
	return bot.Response{
		Message: tgbotapi.NewMessage(
			u.ChatID,
			"Введите шаблон, используя {} как плейсхолдер",
		),
	}, nil
}

func (a *TemplateAddHandler) handleValue(chatID int64, value string, s *bot.ChatSession) (bot.Response, error) {
	const op = "handleValue"
	var payload templateBuilder
	err := json.Unmarshal([]byte(s.Payload()), &payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
	}
	payload.Value = value
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling builder: %w", op, err)
	}

	s.SetState(StateWaitTemplateName)
	s.SetPayload(bytes)
	return bot.Response{
		Message: tgbotapi.NewMessage(
			chatID,
			`
			Введите название шаблона
			`,
		),
	}, nil
}

func (a *TemplateAddHandler) handleName(ctx context.Context, chatID int64, name string, s *bot.ChatSession) (bot.Response, error) {
	const op = "handleName"
	var payload templateBuilder
	err := json.Unmarshal([]byte(s.Payload()), &payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
	}
	payload.Name = name
	template, err := payload.toTemplate()
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: converting builder to template: %w", op, err)
	}
	err = a.saver.Save(ctx, template)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: saving template: %w", op, err)
	}

	s.ToDefault()
	return bot.Response{
		Message: tgbotapi.NewMessage(
			chatID,
			`
			Шаблон сохранен
			`,
		),
	}, nil
}
