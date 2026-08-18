package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ===== STATES =====

var (
	StateNoteWaitAlias    = bot.ChatState("NOTE_WAITING_ALIAS")
	StateNoteWaitTemplate = bot.ChatState("NOTE_WAITING_TEMPLATE")
	StateNoteWaitText     = bot.ChatState("NOTE_WAITING_TEXT")
	StateNoteWaitFilename = bot.ChatState("NOTE_WAITING_FILENAME")
)

// ===== COMMANDS =====

const (
	CommandAddNote = "/add-note"
)

// ===== ADD NOTE =====

type AliasService interface {
	AliasPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Alias], error)
	Alias(ctx context.Context, id string, chatID int64) (domain.Alias, error)
}

type TemplateService interface {
	TemplatesPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Template], error)
	Template(ctx context.Context, id string, chatID int64) (domain.Template, error)
}

type noteAddPayload struct {
	Path         string      `json:"alias"`
	AliasPage    pagePayload `json:"alias_page"`
	Template     string      `json:"template"`
	TemplatePage pagePayload `json:"template_page"`
	Text         string      `json:"text"`
}

func (n noteAddPayload) toNote() domain.Note {
	return domain.Note{
		Path:     n.Path,
		Template: n.Template,
		Text:     n.Text,
	}
}

type AddNoteHandler struct {
	aliasService    AliasService
	templateService TemplateService
	client          ClientGetter
	vault           UserVaultGetter
}

var _ bot.Handler = &AddNoteHandler{}

func NewAddNoteHandler(aliasService AliasService, templateService TemplateService, client ClientGetter, vault UserVaultGetter) *AddNoteHandler {
	return &AddNoteHandler{
		aliasService:    aliasService,
		templateService: templateService,
		client:          client,
		vault:           vault,
	}
}

func (a *AddNoteHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	state := s.State()
	return u.Text == CommandAddNote ||
		state == StateNoteWaitAlias ||
		state == StateNoteWaitFilename ||
		state == StateNoteWaitTemplate ||
		state == StateNoteWaitText
}

func (a *AddNoteHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (resp bot.Response, err error) {
	const op = "AddNoteHandler.Handle"

	defer func() {
		if err != nil {
			s.ToDefault()
		}
	}()

	switch s.State() {
	case bot.DefaultChatState, StateNoteWaitAlias:
		resp, err = a.handleWaitAlias(ctx, s, u)
	case StateNoteWaitTemplate:
		resp, err = a.handleWaitTemplate(ctx, s, u)
	case StateNoteWaitText:
		resp, err = a.handleWaitText(ctx, s, u)
	case StateNoteWaitFilename:
		resp, err = a.handleWaitFilename(ctx, s, u)
	default:
		resp, err = a.handleWaitAlias(ctx, s, u)
	}

	if err != nil {
		err = fmt.Errorf("%v: %w", op, err)
	}
	return
}

func (a *AddNoteHandler) handleWaitAlias(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handleWaitAlias"

	state := s.State()
	payload := pagePayload{}
	if state != bot.DefaultChatState {
		raw := s.Payload()
		err := json.Unmarshal(raw, &payload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
		}
	}
	if u.CallbackData == NextPageCommand {
		payload.PageNum++
	} else if u.CallbackData == PrevPageCommand {
		payload.PageNum--
	} else if state != bot.DefaultChatState {
		alias, err := a.aliasService.Alias(ctx, u.CallbackData, u.ChatID)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: getting alias: %w", op, err)
		}

		notePayload := noteAddPayload{
			Path: alias.Path,
		}
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling note payload: %w", op, err)
		}

		buttons, err := a.templateButtons(ctx, s, 0)
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Выберите шаблон")
		msgCfg.ReplyMarkup = &buttons
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: getting template buttons: %w", op, err)
		}

		s.SetState(StateNoteWaitTemplate)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil
	}

	buttons, err := a.aliasButtons(ctx, s, payload.PageNum)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting alias buttons: %w", op, err)
	}

	var c tgbotapi.Chattable

	if state == bot.DefaultChatState {
		msgCfg := tgbotapi.NewMessage(u.ChatID, "Выберите путь")
		msgCfg.ReplyMarkup = buttons
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageTextAndMarkup(u.ChatID, s.LastBotMessageID(), "Выберите путь", buttons)
		c = msgCfg
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
	}

	s.SetPayload(bytes)
	return bot.Response{
		Message: c,
	}, nil

}

func (a *AddNoteHandler) aliasButtons(ctx context.Context, s *bot.ChatSession, pageNum int) (tgbotapi.InlineKeyboardMarkup, error) {
	const op = "aliasButtons"
	aliases, err := a.aliasService.AliasPage(ctx, s.ChatID(), pageNum, domain.DefaultPageSize)
	if err != nil {
		return tgbotapi.InlineKeyboardMarkup{}, fmt.Errorf("%v: getting aliases page: %w", op, err)
	}
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	for _, alias := range aliases.Values {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				alias.Alias,
				alias.ID.String(),
			),
		})
	}
	if aliases.HasNext() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"Next",
				NextPageCommand,
			),
		})
	}
	if aliases.HasPrev() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"Prev",
				PrevPageCommand,
			),
		})
	}
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}, nil
}

func (a *AddNoteHandler) handleWaitTemplate(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handleWaitTemplate"

	notePayload := noteAddPayload{}

	raw := s.Payload()
	err := json.Unmarshal(raw, &notePayload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
	}

	payload := &notePayload.TemplatePage

	if u.Raw.CallbackData() == NextPageCommand {
		payload.PageNum++
	} else if u.Raw.CallbackData() == PrevPageCommand {
		payload.PageNum--
	} else if s.State() != bot.DefaultChatState {
		template, err := a.templateService.Template(ctx, u.Raw.CallbackData(), u.ChatID)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: getting template: %w", op, err)
		}
		notePayload.Template = template.Value
		notePayload.TemplatePage = pagePayload{}
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
		}

		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Введите текст заметки")
		msgCfg.ReplyMarkup = nil

		s.SetState(StateNoteWaitText)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil
	}

	buttons, err := a.templateButtons(ctx, s, payload.PageNum)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting template buttons: %w", op, err)
	}

	msgCfg := tgbotapi.NewEditMessageTextAndMarkup(u.ChatID, s.LastBotMessageID(), "Выберите путь", buttons)
	bytes, err := json.Marshal(notePayload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
	}

	s.SetPayload(bytes)
	return bot.Response{
		Message: msgCfg,
	}, nil
}

func (a *AddNoteHandler) templateButtons(ctx context.Context, s *bot.ChatSession, pageNum int) (tgbotapi.InlineKeyboardMarkup, error) {
	const op = "templateButtons"
	templates, err := a.templateService.TemplatesPage(ctx, s.ChatID(), pageNum, domain.DefaultPageSize)
	if err != nil {
		return tgbotapi.InlineKeyboardMarkup{}, fmt.Errorf("%v: getting templates page: %w", op, err)
	}
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	for _, template := range templates.Values {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				template.Name,
				template.ID.String(),
			),
		})
	}
	if templates.HasNext() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"Next",
				NextPageCommand,
			),
		})
	}
	if templates.HasPrev() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"Prev",
				PrevPageCommand,
			),
		})
	}

	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}, nil
}

func (a *AddNoteHandler) handleWaitText(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handleWaitText"
	notePayload := noteAddPayload{}
	raw := s.Payload()
	err := json.Unmarshal(raw, &notePayload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
	}
	notePayload.Text = u.Text
	note := notePayload.toNote()

	client, err := a.client.Client(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting client: %w", op, err)
	}
	vault, err := a.vault.Vault(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting vault: %w", op, err)
	}

	file, err := client.File(vault.Owner, vault.Repo, note.Path)
	if err != nil || file.Type != domain.TypeFile {
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
		}
		msgCfg := tgbotapi.NewMessage(u.ChatID, "Введите название файла")

		s.SetState(StateNoteWaitFilename)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil
	}

	if err := a.saveNote(ctx, u.ChatID, note); err != nil {
		return bot.Response{}, fmt.Errorf("%v: saving note: %w", op, err)
	}
	msgCfg := tgbotapi.NewMessage(u.ChatID, "Заметка успешно сохранена")

	s.ToDefault()
	return bot.Response{
		Message: msgCfg,
	}, nil

}

func (a *AddNoteHandler) handleWaitFilename(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handleWaitFilename"
	notePayload := noteAddPayload{}
	raw := s.Payload()
	if err := json.Unmarshal(raw, &notePayload); err != nil {
		return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
	}

	notePayload.Path = strings.TrimSuffix(notePayload.Path, "/")
	if notePayload.Path != "" {
		notePayload.Path += "/"
	}
	notePayload.Path += u.Text

	msgCfg := tgbotapi.NewMessage(u.ChatID, "Заметка успешно сохранена")

	s.ToDefault()
	return bot.Response{
		Message: msgCfg,
	}, nil
}

func (a *AddNoteHandler) saveNote(ctx context.Context, chatID int64, note domain.Note) error {
	const op = "saveNote"
	client, err := a.client.Client(ctx, chatID)
	if err != nil {
		return fmt.Errorf("%v: getting client: %w", op, err)
	}
	vault, err := a.vault.Vault(ctx, chatID)
	if err != nil {
		return fmt.Errorf("%v: getting vault: %w", op, err)
	}
	if err := client.SaveNote(ctx, vault.Owner, vault.Repo, note); err != nil {
		return fmt.Errorf("%v: saving note: %w", op, err)
	}
	return nil
}
