package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ===== STATES =====

var StateGetAlias bot.ChatState = "ALIAS_GET"

var (
	StateWaitPath  = bot.ChatState("WAITING_PATH")
	StateWaitAlias = bot.ChatState("WAITING_ALIAS")
)

// ===== COMMANDS =====

var (
	CommandGetAliases = bot.Command("/alias")

	CommandAddAlias = bot.Command("/add-alias")

	prevPathCommand   = bot.Command("..")
	currentDirCommand = bot.Command("*")
)

// ===== GET ALIASES =====

type AliasesGetter interface {
	AliasPage(ctx context.Context, chatID int64, pageNum, pageSize int) (domain.Page[domain.Alias], error)
}

type GetAliasesHandler struct {
	getter AliasesGetter
}

func NewGetAliasesHandler(getter AliasesGetter) *GetAliasesHandler {
	return &GetAliasesHandler{
		getter: getter,
	}
}

func (h *GetAliasesHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	session := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)

	payload := pagePayload{}

	if session.State != bot.DefaultChatState {
		raw := session.Payload
		err := json.Unmarshal([]byte(raw), &payload)
		if err != nil {
			return bot.Response{}, err
		}
		if u.Text == NextPageCommand {
			payload.PageNum++
		} else {
			payload.PageNum = max(0, payload.PageNum-1)
		}
	}

	aliasesPage, err := h.getter.AliasPage(
		ctx,
		session.ChatID,
		payload.PageNum,
		domain.DefaultPageSize,
	)
	if err != nil {
		return bot.Response{}, err
	}

	textBuilder := []rune("Ваши алиасы\n")
	for _, alias := range aliasesPage.Values {
		textBuilder = append(
			textBuilder,
			[]rune(
				fmt.Sprintf(`
				%s -> %s\n
				`,
					alias.Path,
					alias.Alias,
				),
			)...)
	}

	if len(aliasesPage.Values) == 0 {
		textBuilder = append(textBuilder,
			[]rune(
				`
			У вас нет алиасов. Создайте свой первый алиас с помощью /add-alias
			`,
			)...)
	}

	buttons := tgbotapi.NewInlineKeyboardRow()
	if payload.PageNum != aliasesPage.TotalPages-1 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Next", NextPageCommand))
	}
	if payload.PageNum != 0 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Prev", PrevPageCommand))
	}

	replyMarkup := tgbotapi.NewInlineKeyboardMarkup(
		buttons,
	)

	var c tgbotapi.Chattable

	if session.State == bot.DefaultChatState || session.LastBotMessageID == 0 {
		msgCfg := tgbotapi.NewMessage(session.ChatID, string(textBuilder))
		msgCfg.ReplyMarkup = replyMarkup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageCaption(session.ChatID, session.LastBotMessageID, string(textBuilder))
		msgCfg.ReplyMarkup = &replyMarkup
		c = msgCfg
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}

	return bot.Response{
		Message:      c,
		NewChatState: &StateGetAlias,
		NewPayload:   string(bytes),
	}, nil
}

// ===== CREATE ALIAS =====

type pathChoosingPayload struct {
	CurPath string `json:"cur_path"`
	PageNum int    `json:"page"`
}

func (p pathChoosingPayload) cdPrev() pathChoosingPayload {
	last := strings.LastIndex(p.CurPath, "/")
	if last == -1 {
		return p
	}
	p.CurPath = p.CurPath[:last]
	return p
}

type UserVaultGetter interface {
	Vault(ctx context.Context, chatID int64) (domain.UserVault, error)
}

type AliasSaver interface {
	Save(context.Context, domain.Alias) error
}

type AddAliasHandler struct {
	vaultGetter  UserVaultGetter
	clientGetter ClientGetter
	saver        AliasSaver
}

func NewAddAliasHandler(vaultGetter UserVaultGetter, clientGetter ClientGetter, saver AliasSaver) *AddAliasHandler {
	return &AddAliasHandler{
		vaultGetter:  vaultGetter,
		clientGetter: clientGetter,
		saver:        saver,
	}
}

func (a *AddAliasHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	session := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)

	switch session.State {
	case bot.DefaultChatState, StateWaitPath:
		return a.handlePathSet(ctx, u, session)
	case StateWaitAlias:
		return a.handleAliasSet(ctx, u, session)
	}

	return bot.Response{}, domain.ErrCantHandle

}

type aliasBuilder struct {
	ID     string `json:"id"`
	ChatID int64  `json:"chat_id"`
	Path   string `json:"path"`
	Alias  string `json:"alias"`
	Type   string `json:"type"`
}

func newAliasBuilder(chatID int64) aliasBuilder {
	return aliasBuilder{
		ID:     uuid.NewString(),
		ChatID: chatID,
		Type:   domain.TypeFile,
	}
}

func (a aliasBuilder) toAlias() (domain.Alias, error) {
	id, err := uuid.Parse(a.ID)
	if err != nil {
		return domain.Alias{}, err
	}
	return domain.Alias{
		ID:     id,
		ChatID: a.ChatID,
		Path:   a.Path,
		Alias:  a.Alias,
	}, nil
}

func (a *AddAliasHandler) handlePathSet(ctx context.Context, u bot.Update, s bot.ChatSession) (bot.Response, error) {
	client, err := a.clientGetter.Client(ctx, s.ChatID)
	if err != nil {
		return bot.Response{}, err
	}

	vault, err := a.vaultGetter.Vault(ctx, s.ChatID)
	if err != nil {
		return bot.Response{}, err
	}
	payload := pathChoosingPayload{}
	if s.State != bot.DefaultChatState {
		raw := s.Payload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return bot.Response{}, err
		}
	}
	if u.Text == string(prevPathCommand) {
		payload = payload.cdPrev()
	} else if u.Text == string(currentDirCommand) {
		msgCfg := tgbotapi.NewEditMessageText(s.ChatID, s.LastBotMessageID, "Введите алиас для пути")
		builder := newAliasBuilder(s.ChatID)
		builder.Path = payload.CurPath
		builder.Type = domain.TypeDir
		bytes, err := json.Marshal(builder)
		if err != nil {
			return bot.Response{}, err
		}
		return bot.Response{
			Message:      msgCfg,
			NewChatState: &StateWaitAlias,
			NewPayload:   string(bytes),
		}, nil
	} else if u.Text != NextPageCommand && u.Text != PrevPageCommand {
		payload.CurPath, err = url.JoinPath(payload.CurPath, u.Text)
		if err != nil {
			return bot.Response{}, err
		}
	}

	dir, err := client.Directory(
		vault.Owner,
		vault.Repo,
		payload.CurPath,
		payload.PageNum,
		domain.DefaultPageSize,
	)
	if err != nil && errors.Is(err, domain.ErrNotDirectory) {
		msgCfg := tgbotapi.NewEditMessageText(s.ChatID, s.LastBotMessageID, "Введите алиас для пути")
		builder := newAliasBuilder(s.ChatID)
		builder.Path = payload.CurPath
		bytes, err := json.Marshal(builder)
		if err != nil {
			return bot.Response{}, err
		}
		return bot.Response{
			Message:      msgCfg,
			NewChatState: &StateWaitAlias,
			NewPayload:   string(bytes),
		}, nil

	} else if err != nil {
		return bot.Response{}, err
	}

	msgCfg := tgbotapi.NewMessage(
		s.ChatID,
		`
		Выберите путь
		(Файл, если вы хотите использовать алиас для записи в существующий файл)
		(Директорию, если вы хотите использовать алиас для создания новых файлов в этой директории)
		`,
	)

	msgCfg.ReplyMarkup = a.pathButtons(dir, payload)
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message:    msgCfg,
		NewPayload: string(bytes),
	}, nil
}

func (a *AddAliasHandler) pathButtons(dirElems domain.Page[domain.DirElem], payload pathChoosingPayload) tgbotapi.InlineKeyboardMarkup {
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	if payload.CurPath != "" {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				"..",
				string(prevPathCommand),
			),
		})
	}

	for _, elem := range dirElems.Values {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%s: %s", elem.Type, elem.Name),
				elem.Name,
			),
		})
	}
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			"Выбрать текущую директорию как путь",
			string(currentDirCommand),
		),
	})
	if payload.PageNum < dirElems.TotalPages-1 {
		buttons = append(buttons,
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					"Next->",
					NextPageCommand,
				),
			},
		)
	}
	if payload.PageNum > 0 {
		buttons = append(buttons,
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					"<-Prev",
					PrevPageCommand,
				),
			},
		)
	}
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}
}

func (a *AddAliasHandler) handleAliasSet(ctx context.Context, u bot.Update, s bot.ChatSession) (bot.Response, error) {
	alias := u.Text

	payload := aliasBuilder{}
	raw := s.Payload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return bot.Response{}, err
	}
	payload.Alias = alias
	aliasToSave, err := payload.toAlias()
	if err != nil {
		return bot.Response{}, err
	}
	if err := a.saver.Save(ctx, aliasToSave); err != nil {
		return bot.Response{}, err
	}
	return bot.Response{
		Message: tgbotapi.NewMessage(
			s.ChatID,
			"Алиас успешно сохранен",
		),
		NewChatState: &bot.DefaultChatState,
	}, nil
}
