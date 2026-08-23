package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ===== STATES =====

const (
	StateNoteWaitAlias    = "NOTE_WAITING_ALIAS"
	StateNoteWaitTemplate = "NOTE_WAITING_TEMPLATE"
	StateNoteWaitText     = "NOTE_WAITING_TEXT"
	StateNoteWaitFilename = "NOTE_WAITING_FILENAME"
)

// ===== COMMANDS =====

const (
	CommandAddNote = "/add_note"
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
	Path         string      `json:"path"`
	PathType     string      `json:"path_type"`
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
	if u.Text == CommandAddNote {
		s.ToDefault()
		return true
	}
	return (state == StateNoteWaitAlias && u.CallbackData != "") ||
		(state == StateNoteWaitFilename && u.CallbackData != "") ||
		(state == StateNoteWaitTemplate && u.CallbackData != "") ||
		(state == StateNoteWaitText && u.Text != "")
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
	if state == StateNoteWaitAlias {
		raw := s.Payload()
		err := json.Unmarshal(raw, &payload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
		}
	}
	if u.CallbackData == NextPageCallback {
		payload.PageNum++
	} else if u.CallbackData == PrevPageCallback {
		payload.PageNum--
	} else if state == StateNoteWaitAlias {
		alias, err := a.aliasService.Alias(ctx, u.CallbackData, u.ChatID)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: getting alias: %w", op, err)
		}

		notePayload := noteAddPayload{
			Path:     alias.Path.Value,
			PathType: alias.Path.Type,
		}
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling note payload: %w", op, err)
		}

		templates, err := a.templateService.TemplatesPage(ctx, u.ChatID, 0, domain.DefaultPageSize)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: getting templates: %w", op, err)
		}
		if templates.TotalPages == 0 {
			s.ToDefault()
			return bot.Response{
				Message: tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(),
					fmt.Sprintf("Шаблонов нет, добавьте через %s", CommandAddTemplate),
				),
			}, nil
		}

		buttons := a.templateButtons(templates)
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Выберите шаблон")
		msgCfg.ReplyMarkup = &buttons

		s.SetState(StateNoteWaitTemplate)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil
	}
	aliases, err := a.aliasService.AliasPage(ctx, s.ChatID(), payload.PageNum, domain.DefaultPageSize)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting aliases page: %w", op, err)
	}
	if aliases.TotalPages == 0 {
		s.ToDefault()
		return bot.Response{
			Message: tgbotapi.NewMessage(
				u.ChatID,
				fmt.Sprintf("У вас нет алиасов, добавьте с помощью %s", CommandAddAlias),
			),
		}, nil
	}
	buttons := a.aliasButtons(aliases)

	var c tgbotapi.Chattable

	if state != StateNoteWaitAlias {
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

	s.SetState(StateNoteWaitAlias)
	s.SetPayload(bytes)
	return bot.Response{
		Message: c,
	}, nil

}

func (a *AddNoteHandler) aliasButtons(page domain.Page[domain.Alias]) tgbotapi.InlineKeyboardMarkup {
	const op = "aliasButtons"
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	for _, alias := range page.Values {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				alias.Alias,
				alias.ID.String(),
			),
		})
	}
	if page.HasNext() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"Next",
				NextPageCallback,
			),
		})
	}
	if page.HasPrev() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"Prev",
				PrevPageCallback,
			),
		})
	}
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}
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

	if u.Raw.CallbackData() == NextPageCallback {
		payload.PageNum++
	} else if u.Raw.CallbackData() == PrevPageCallback {
		payload.PageNum--
	} else if s.State() == StateNoteWaitTemplate {
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

	templates, err := a.templateService.TemplatesPage(ctx, u.ChatID, payload.PageNum, domain.DefaultPageSize)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: error getting template page: %w", op, err)
	}
	if templates.TotalPages == 0 {
		s.ToDefault()
		return bot.Response{
			Message: tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(),
				fmt.Sprintf("Шаблонов нет, добавьте через %s", CommandAddTemplate),
			),
		}, nil
	}

	buttons := a.templateButtons(templates)

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

func (a *AddNoteHandler) templateButtons(page domain.Page[domain.Template]) tgbotapi.InlineKeyboardMarkup {
	const op = "templateButtons"
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	for _, template := range page.Values {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				template.Name,
				template.ID.String(),
			),
		})
	}
	paginationRow := tgbotapi.NewInlineKeyboardRow()
	if page.HasPrev() {
		paginationRow = append(paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(
				PrevPageButtonText,
				PrevPageCallback,
			),
		)
	}
	if page.HasNext() {
		paginationRow = append(paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(
				NextPageButtonText,
				NextPageCallback,
			),
		)
	}
	if len(paginationRow) != 0 {
		buttons = append(buttons, paginationRow)
	}

	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}
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

	if notePayload.PathType == domain.PathTypeDir {
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

	note := notePayload.toNote()

	client, err := a.client.Client(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting client: %w", op, err)
	}
	vault, err := a.vault.Vault(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting vault: %w", op, err)
	}

	if err := client.UpdateNote(ctx, vault.Owner, vault.Repo, note); err != nil {
		return bot.Response{}, fmt.Errorf("%v: updating note: %w", op, err)
	}
	msgCfg := tgbotapi.NewMessage(u.ChatID, "Заметка успешно обновлена")

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

	fullPath, err := url.JoinPath(notePayload.Path, u.Text)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: adding filename to path: %w", op, err)
	}
	notePayload.Path = fullPath

	note := notePayload.toNote()
	client, err := a.client.Client(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting client: %w", op, err)
	}
	vault, err := a.vault.Vault(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting vault: %w", op, err)
	}

	if err := client.CreateNote(ctx, vault.Owner, vault.Repo, note); err != nil {
		return bot.Response{}, fmt.Errorf("%v: creating note: %w", op, err)
	}

	msgCfg := tgbotapi.NewMessage(u.ChatID, "Заметка успешно сохранена")

	s.ToDefault()
	return bot.Response{
		Message: msgCfg,
	}, nil
}
