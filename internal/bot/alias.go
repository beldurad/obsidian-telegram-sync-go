package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

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

var _ bot.Handler = &GetAliasesHandler{}

func (h *GetAliasesHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	return u.Text == string(CommandGetAliases) ||
		s.State() == StateGetAlias
}

func (h *GetAliasesHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "GetAliasesHandler.Handle"

	state := s.State()
	payload := pagePayload{}

	if state != bot.DefaultChatState {
		raw := s.Payload()
		err := json.Unmarshal(raw, &payload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: payload unmarshaling: %w", op, err)
		}
		switch u.Text {
		case NextPageCommand:
			payload.PageNum++
		case PrevPageCommand:
			payload.PageNum = max(0, payload.PageNum-1)
		}
	}

	aliasesPage, err := h.getter.AliasPage(
		ctx,
		u.ChatID,
		payload.PageNum,
		domain.DefaultPageSize,
	)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: error while getting alias page %d: %w", op, payload.PageNum, err)
	}

	textBuilder := []rune("Ваши алиасы\n")
	for _, alias := range aliasesPage.Values {
		textBuilder = append(
			textBuilder,
			[]rune(
				fmt.Sprintf(`
				%s -> %s
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
	if aliasesPage.HasNext() {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Next", NextPageCommand))
	}
	if aliasesPage.HasPrev() {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Prev", PrevPageCommand))
	}

	replyMarkup := tgbotapi.NewInlineKeyboardMarkup(
		buttons,
	)

	var c tgbotapi.Chattable

	if state == bot.DefaultChatState || s.LastBotMessageID() == 0 {
		msgCfg := tgbotapi.NewMessage(u.ChatID, string(textBuilder))
		msgCfg.ReplyMarkup = replyMarkup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), string(textBuilder))
		msgCfg.ReplyMarkup = &replyMarkup
		c = msgCfg
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling response payload: %w", op, err)
	}

	s.SetState(StateGetAlias)
	s.SetPayload(bytes)
	return bot.Response{
		Message: c,
	}, nil
}

// ===== CREATE ALIAS =====

type pathChoosingPayload struct {
	CurPath string `json:"cur_path"`
	PageNum int    `json:"page"`
}

func newPathChoosingPayload() pathChoosingPayload {
	return pathChoosingPayload{
		CurPath: "/",
	}
}

func (p pathChoosingPayload) isRootPath() bool {
	return p.CurPath == "" || p.CurPath == "/"
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

var _ bot.Handler = &AddAliasHandler{}

func (a *AddAliasHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	return u.Text == string(CommandAddAlias) ||
		s.State() == StateWaitPath ||
		s.State() == StateWaitAlias
}

func (a *AddAliasHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (resp bot.Response, err error) {
	const op = "AddAliasHandler.Handle"

	defer func() {
		if err != nil {
			s.ToDefault()
		}
	}()

	switch s.State() {
	case bot.DefaultChatState, StateWaitPath:
		resp, err = a.handlePathSet(ctx, s, u)
	case StateWaitAlias:
		resp, err = a.handleAliasSet(ctx, s, u)
	default:
		resp, err = bot.Response{}, ErrCantHandle
	}

	if err != nil {
		err = fmt.Errorf("%v: %w", op, err)
	}
	return
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

func (a *AddAliasHandler) handlePathSet(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handlePathSet"
	client, err := a.clientGetter.Client(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting client: %w", op, err)
	}

	vault, err := a.vaultGetter.Vault(ctx, u.ChatID)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting vault: %w", op, err)
	}
	payload := newPathChoosingPayload()
	if s.State() != bot.DefaultChatState {
		raw := s.Payload()
		if err := json.Unmarshal(raw, &payload); err != nil {
			return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
		}
	}
	if u.Text == string(currentDirCommand) {
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Введите алиас для пути")
		builder := newAliasBuilder(u.ChatID)
		builder.Path = payload.CurPath
		builder.Type = domain.TypeDir
		bytes, err := json.Marshal(builder)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling builder: %w", op, err)
		}

		s.SetState(StateWaitAlias)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil
	} else if u.Text != NextPageCommand && u.Text != PrevPageCommand {
		payload.CurPath, err = url.JoinPath(payload.CurPath, u.Text)
		payload.PageNum = 0
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: joining path: %w", op, err)
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
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Введите алиас для пути")
		builder := newAliasBuilder(u.ChatID)
		builder.Path = payload.CurPath
		bytes, err := json.Marshal(builder)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling builder: %w", op, err)
		}

		s.SetState(StateWaitAlias)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil

	} else if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting directory contents: %w", op, err)
	}

	msgCfg := tgbotapi.NewMessage(
		u.ChatID,
		`
		Выберите путь
		(Файл, если вы хотите использовать алиас для записи в существующий файл)
		(Директорию, если вы хотите использовать алиас для создания новых файлов в этой директории)
		`,
	)

	msgCfg.ReplyMarkup = pathButtons(dir, payload)
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
	}

	s.SetState(StateWaitPath)
	s.SetPayload(bytes)
	return bot.Response{
		Message: msgCfg,
	}, nil
}

func pathButtons(dirElems domain.Page[domain.DirElem], payload pathChoosingPayload) tgbotapi.InlineKeyboardMarkup {
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	if !payload.isRootPath() {
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
				fmt.Sprintf("/%s", elem.Name),
			),
		})
	}
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			"Выбрать текущую директорию как путь",
			string(currentDirCommand),
		),
	})
	if dirElems.HasNext() {
		buttons = append(buttons,
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					"Next->",
					NextPageCommand,
				),
			},
		)
	}
	if dirElems.HasPrev() {
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

func (a *AddAliasHandler) handleAliasSet(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "handleAliasSet"
	alias := u.Text

	payload := aliasBuilder{}
	raw := s.Payload()
	if err := json.Unmarshal(raw, &payload); err != nil {
		return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
	}
	payload.Alias = alias
	aliasToSave, err := payload.toAlias()
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: can't convert payload to alias: %w", op, err)
	}

	if err := a.saver.Save(ctx, aliasToSave); err != nil {
		return bot.Response{}, fmt.Errorf("%v: error while saving alias: %w", op, err)
	}

	s.ToDefault()
	msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Алиас успешно сохранен")
	return bot.Response{Message: msgCfg}, nil
}
