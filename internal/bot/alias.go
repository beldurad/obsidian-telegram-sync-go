package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
)

// ===== STATES =====

const StateGetAlias = "ALIAS_GET"

const (
	StateWaitPath  = "WAITING_PATH"
	StateWaitAlias = "WAITING_ALIAS"
)

// ===== COMMANDS =====

const (
	CommandGetAliases = "/alias"

	CommandAddAlias = "/add_alias"

	prevPathCallback   = ".."
	prevPathButtonText = ".."

	curDirCallback   = "*"
	curDirButtonText = "Выбрать текущую директорию как путь"
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
	if u.Text == CommandGetAliases {
		s.ToDefault()
		return true
	}
	return s.State() == StateGetAlias && u.CallbackData != ""
}

func (h *GetAliasesHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
	const op = "GetAliasesHandler.Handle"

	state := s.State()
	payload := pagePayload{}

	if state == StateGetAlias {
		raw := s.Payload()
		err := json.Unmarshal(raw, &payload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: payload unmarshaling: %w", op, err)
		}
		payload = payload.handlePageUpdate(u)
		log.Println(payload)
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
					alias.Alias,
					alias.Path,
				),
			)...)
	}

	if len(aliasesPage.Values) == 0 {
		textBuilder = append(textBuilder,
			[]rune(fmt.Sprintf(
				`
			У вас нет алиасов. Создайте свой первый алиас с помощью %s
			`,
				CommandAddAlias))...)
	}

	buttons := pageButtons(aliasesPage)

	replyMarkup := tgbotapi.NewInlineKeyboardMarkup(
		buttons,
	)

	var c tgbotapi.Chattable

	if state != StateGetAlias || s.LastBotMessageID() == 0 {
		msgCfg := tgbotapi.NewMessage(u.ChatID, string(textBuilder))
		msgCfg.ReplyMarkup = replyMarkup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), string(textBuilder))
		msgCfg.ReplyMarkup = &replyMarkup
		c = msgCfg
	}

	payload = payloadFromPage(aliasesPage)
	log.Println("after page", payload)
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
	CurPath     string `json:"cur_path"`
	pagePayload `json:"page"`
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
	if u.Text == CommandAddAlias {
		s.ToDefault()
		return true
	}
	return (s.State() == StateWaitPath && u.CallbackData != "") ||
		(s.State() == StateWaitAlias && u.Text != "")
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
		resp, err = a.handlePathSet(ctx, s, u)
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
		Type:   domain.PathTypeFile,
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
		Path: domain.Path{
			Value: a.Path,
			Type:  a.Type,
		},
		Alias: a.Alias,
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
	if s.State() == StateWaitPath {
		raw := s.Payload()
		if err := json.Unmarshal(raw, &payload); err != nil {
			return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
		}
	}
	payload.pagePayload = payload.handlePageUpdate(u)

	if u.CallbackData == curDirCallback {
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), "Введите алиас для пути")
		builder := newAliasBuilder(u.ChatID)
		builder.Path = payload.CurPath
		builder.Type = domain.PathTypeDir
		bytes, err := json.Marshal(builder)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: marshaling builder: %w", op, err)
		}

		s.SetState(StateWaitAlias)
		s.SetPayload(bytes)
		return bot.Response{
			Message: msgCfg,
		}, nil
	} else if s.State() == StateWaitPath && !isPageUpdate(u) {
		payload.CurPath = path.Join(payload.CurPath, u.CallbackData)
		payload.pagePayload = pagePayload{}
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

	textMsg := "Выберите путь\n" +
		"(Файл, если вы хотите использовать алиас для записи в существующий файл)\n" +
		"(Директорию, если вы хотите использовать алиас для создания новых файлов в этой директории)"

	var c tgbotapi.Chattable

	switch s.State() {
	case StateWaitPath:
		msgCfg := tgbotapi.NewEditMessageText(u.ChatID, s.LastBotMessageID(), textMsg)
		markup := pathButtons(dir, payload)
		msgCfg.ReplyMarkup = &markup
		c = msgCfg
	default:
		msgCfg := tgbotapi.NewMessage(u.ChatID, textMsg)
		msgCfg.ReplyMarkup = pathButtons(dir, payload)
		c = msgCfg
	}

	payload.pagePayload = payloadFromPage(dir)
	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
	}

	s.SetState(StateWaitPath)
	s.SetPayload(bytes)
	return bot.Response{
		Message: c,
	}, nil
}

func pathButtons(dirElems domain.Page[domain.File], payload pathChoosingPayload) tgbotapi.InlineKeyboardMarkup {
	buttons := make([][]tgbotapi.InlineKeyboardButton, 0)
	if !payload.isRootPath() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				prevPathButtonText,
				prevPathCallback,
			),
		})
	}

	for _, elem := range dirElems.Values {
		text := elem.Name
		if elem.Path.Type == domain.PathTypeDir {
			text += "/"
		}
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				text,
				fmt.Sprintf("/%s", elem.Name),
			),
		})
	}
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			curDirButtonText,
			curDirCallback,
		),
	})

	pageRow := pageButtons(dirElems)
	if len(pageRow) != 0 {
		buttons = append(buttons, pageRow)
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
	msgCfg := tgbotapi.NewMessage(u.ChatID, "Алиас успешно сохранен")
	return bot.Response{Message: msgCfg}, nil
}
