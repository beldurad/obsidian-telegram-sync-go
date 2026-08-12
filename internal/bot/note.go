package bot

import (
	"context"
	"encoding/json"
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

func NewAddNoteHandler(aliasService AliasService, templateService TemplateService, client ClientGetter, vault UserVaultGetter) *AddNoteHandler {
	return &AddNoteHandler{
		aliasService:    aliasService,
		templateService: templateService,
		client:          client,
		vault:           vault,
	}
}

func (a *AddNoteHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	session := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)

	switch session.State {
	case bot.DefaultChatState, StateNoteWaitAlias:
		return a.handleWaitAlias(ctx, session, u)
	case StateNoteWaitTemplate:
		return a.handleWaitTemplate(ctx, session, u)
	case StateNoteWaitText:
		return a.handleWaitText(ctx, session, u)
	case StateNoteWaitFilename:
		return a.handleWaitFilename(ctx, session, u)
	default:
		return a.handleWaitAlias(ctx, session, u)
	}
}

func (a *AddNoteHandler) handleWaitAlias(ctx context.Context, s bot.ChatSession, u bot.Update) (bot.Response, error) {
	payload := pagePayload{}
	if s.State != bot.DefaultChatState {
		raw := s.Payload
		err := json.Unmarshal([]byte(raw), &payload)
		if err != nil {
			return bot.Response{}, err
		}
	}
	if u.Raw.CallbackData() == NextPageCommand {
		payload.PageNum++
	} else if u.Raw.CallbackData() == PrevPageCommand {
		payload.PageNum--
	} else if s.State != bot.DefaultChatState {
		alias, err := a.aliasService.Alias(ctx, u.Raw.CallbackData(), u.ChatID)
		if err != nil {
			return bot.Response{}, err
		}
		notePayload := noteAddPayload{
			Path: alias.Path,
		}
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, err
		}

		buttons, err := a.templateButtons(ctx, s, 0)
		msgCfg := tgbotapi.NewEditMessageText(s.ChatID, s.LastBotMessageID, "Выберите шаблон")
		msgCfg.ReplyMarkup = &buttons
		if err != nil {
			return bot.Response{}, err
		}
		return bot.Response{
			Message:      msgCfg,
			NewChatState: &StateNoteWaitTemplate,
			NewPayload:   bytes,
		}, nil
	}

	buttons, err := a.aliasButtons(ctx, s, payload.PageNum)
	if err != nil {
		return bot.Response{}, err
	}

	var c tgbotapi.Chattable

	if s.State == bot.DefaultChatState {
		msgCfg := tgbotapi.NewMessage(s.ChatID, "Выберите путь")
		msgCfg.ReplyMarkup = buttons
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageTextAndMarkup(s.ChatID, s.LastBotMessageID, "Выберите путь", buttons)
		c = msgCfg
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message:    c,
		NewPayload: bytes,
	}, nil

}

func (a *AddNoteHandler) aliasButtons(ctx context.Context, s bot.ChatSession, pageNum int) (tgbotapi.InlineKeyboardMarkup, error) {
	aliases, err := a.aliasService.AliasPage(ctx, s.ChatID, pageNum, domain.DefaultPageSize)
	if err != nil {
		return tgbotapi.InlineKeyboardMarkup{}, err
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

func (a *AddNoteHandler) handleWaitTemplate(ctx context.Context, s bot.ChatSession, u bot.Update) (bot.Response, error) {
	notePayload := noteAddPayload{}
	raw := s.Payload
	err := json.Unmarshal([]byte(raw), &notePayload)
	if err != nil {
		return bot.Response{}, err
	}
	payload := &notePayload.TemplatePage

	if u.Raw.CallbackData() == NextPageCommand {
		payload.PageNum++
	} else if u.Raw.CallbackData() == PrevPageCommand {
		payload.PageNum--
	} else if s.State != bot.DefaultChatState {
		template, err := a.templateService.Template(ctx, u.Raw.CallbackData(), u.ChatID)
		if err != nil {
			return bot.Response{}, err
		}
		notePayload.Template = template.Value
		notePayload.TemplatePage = pagePayload{}
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, err
		}

		msgCfg := tgbotapi.NewEditMessageText(s.ChatID, s.LastBotMessageID, "Введите текст заметки")
		msgCfg.ReplyMarkup = nil
		return bot.Response{
			Message:      msgCfg,
			NewChatState: &StateNoteWaitText,
			NewPayload:   bytes,
		}, nil
	}

	buttons, err := a.templateButtons(ctx, s, payload.PageNum)
	if err != nil {
		return bot.Response{}, err
	}

	msgCfg := tgbotapi.NewEditMessageTextAndMarkup(s.ChatID, s.LastBotMessageID, "Выберите путь", buttons)
	bytes, err := json.Marshal(notePayload)
	if err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message:    msgCfg,
		NewPayload: bytes,
	}, nil
}

func (a *AddNoteHandler) templateButtons(ctx context.Context, s bot.ChatSession, pageNum int) (tgbotapi.InlineKeyboardMarkup, error) {
	templates, err := a.templateService.TemplatesPage(ctx, s.ChatID, pageNum, domain.DefaultPageSize)
	if err != nil {
		return tgbotapi.InlineKeyboardMarkup{}, err
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

func (a *AddNoteHandler) handleWaitText(ctx context.Context, s bot.ChatSession, u bot.Update) (bot.Response, error) {
	notePayload := noteAddPayload{}
	raw := s.Payload
	err := json.Unmarshal([]byte(raw), &notePayload)
	if err != nil {
		return bot.Response{}, err
	}
	notePayload.Text = u.Text
	note := notePayload.toNote()

	client, err := a.client.Client(ctx, s.ChatID)
	if err != nil {
		return bot.Response{}, err
	}
	vault, err := a.vault.Vault(ctx, s.ChatID)
	if err != nil {
		return bot.Response{}, err
	}

	file, err := client.File(vault.Owner, vault.Repo, note.Path)
	if err != nil || file.Type != domain.TypeFile {
		bytes, err := json.Marshal(notePayload)
		if err != nil {
			return bot.Response{}, err
		}
		msgCfg := tgbotapi.NewMessage(s.ChatID, "Введите название файла")
		return bot.Response{
			Message:      msgCfg,
			NewChatState: &StateNoteWaitFilename,
			NewPayload:   bytes,
		}, nil
	}

	if err := a.saveNote(ctx, s.ChatID, note); err != nil {
		return bot.Response{}, err
	}
	msgCfg := tgbotapi.NewMessage(s.ChatID, "Заметка успешно сохранена")
	return bot.Response{
		Message:      msgCfg,
		NewChatState: &bot.DefaultChatState,
	}, nil

}

func (a *AddNoteHandler) handleWaitFilename(ctx context.Context, s bot.ChatSession, u bot.Update) (bot.Response, error) {
	notePayload := noteAddPayload{}
	raw := s.Payload
	if err := json.Unmarshal([]byte(raw), &notePayload); err != nil {
		return bot.Response{}, err
	}

	notePayload.Path = strings.TrimSuffix(notePayload.Path, "/")
	if notePayload.Path != "" {
		notePayload.Path += "/"
	}
	notePayload.Path += u.Text

	if err := a.saveNote(ctx, s.ChatID, notePayload.toNote()); err != nil {
		return bot.Response{}, err
	}
	msgCfg := tgbotapi.NewMessage(s.ChatID, "Заметка успешно сохранена")
	return bot.Response{
		Message:      msgCfg,
		NewChatState: &bot.DefaultChatState,
	}, nil
}

func (a *AddNoteHandler) saveNote(ctx context.Context, chatID int64, note domain.Note) error {
	client, err := a.client.Client(ctx, chatID)
	if err != nil {
		return err
	}
	vault, err := a.vault.Vault(ctx, chatID)
	if err != nil {
		return err
	}
	return client.SaveNote(ctx, vault.Owner, vault.Repo, note)
}
