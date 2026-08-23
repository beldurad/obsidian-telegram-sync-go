package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ===== STATES =====

var RepoSetState = bot.ChatState("SETTING_REPO")

// ===== COMMANDS =====

const CommandSetRepo = "/set_repo"

var RepoSetCommands = []string{
	CommandSetRepo,
	NextPageCallback,
	PrevPageCallback,
}

// ===== SET REPO =====

type ClientGetter interface {
	Client(ctx context.Context, chatID int64) (domain.RemoteStorage, error)
}

type UserVaultService interface {
	Save(context.Context, domain.UserVault) error
	ExistsByChatID(ctx context.Context, chatID int64) (bool, error)
}

type RepoSetHandler struct {
	clientGetter ClientGetter
	vaultService UserVaultService
}

var _ bot.Handler = &RepoSetHandler{}

func NewRepoSetHandler(clientGetter ClientGetter, vaultService UserVaultService) *RepoSetHandler {
	return &RepoSetHandler{
		clientGetter: clientGetter,
		vaultService: vaultService,
	}
}

func (h *RepoSetHandler) Match(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
	if u.Text == CommandSetRepo {
		s.ToDefault()
		return true
	}
	return s.State() == RepoSetState && u.CallbackData != ""
}

func (h *RepoSetHandler) Handle(ctx context.Context, s *bot.ChatSession, u bot.Update) (resp bot.Response, err error) {
	const op = "RepoSetHandler.Handle"

	defer func() {
		if err != nil {
			s.ToDefault()
		}
	}()

	chatID := s.ChatID()
	state := s.State()

	client, err := h.clientGetter.Client(ctx, chatID)
	if err != nil {
		resp, err = bot.Response{}, fmt.Errorf("%v: getting client: %w", op, err)
		return
	}
	user, err := client.UserInfo()
	if err != nil {
		resp, err = bot.Response{}, fmt.Errorf("%v: getting user info: %w", op, err)
		return
	}

	if state == RepoSetState &&
		!slices.Contains(RepoSetCommands, u.Raw.CallbackData()) {
		resp, err = h.handleChosenRepo(ctx, s, client, u.CallbackData, user.Username)
		return
	}

	var payload pagePayload
	if state != bot.DefaultChatState {
		raw := s.Payload()
		err := json.Unmarshal(raw, &payload)
		if err != nil {
			return bot.Response{}, fmt.Errorf("%v: unmarshaling payload: %w", op, err)
		}
		switch u.Raw.CallbackData() {
		case NextPageCallback:
			payload.PageNum++
		case PrevPageCallback:
			payload.PageNum = max(0, payload.PageNum-1)
		}
	}

	repoPage, err := client.UserRepos(user.Username, payload.PageNum, domain.DefaultPageSize)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: getting user repos: %w", op, err)
	}

	buttons := make([][]tgbotapi.InlineKeyboardButton, len(repoPage.Values))
	for i, repo := range repoPage.Values {
		buttons[i] = []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(repo.Name, repo.Name),
		}
	}
	paginationRow := make([]tgbotapi.InlineKeyboardButton, 0)
	if repoPage.HasPrev() {
		paginationRow = append(
			paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(PrevPageButtonText, PrevPageCallback),
		)
	}
	if repoPage.HasNext() {
		paginationRow = append(
			paginationRow,
			tgbotapi.NewInlineKeyboardButtonData(NextPageButtonText, NextPageCallback),
		)
	}
	if len(paginationRow) != 0 {
		buttons = append(buttons, paginationRow)
	}
	markup := tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}

	var c tgbotapi.Chattable

	if state == bot.DefaultChatState || s.LastBotMessageID() == 0 {
		msgCfg := tgbotapi.NewMessage(chatID, "Выберите репозиторий хранилища")
		msgCfg.ReplyMarkup = markup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageText(chatID, s.LastBotMessageID(), "Выберите репозиторий хранилища")
		msgCfg.ReplyMarkup = &markup
		c = msgCfg
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: marshaling payload: %w", op, err)
	}

	s.SetState(RepoSetState)
	s.SetPayload(bytes)

	resp, err = bot.Response{Message: c}, nil
	return
}

func (h *RepoSetHandler) handleChosenRepo(ctx context.Context, session *bot.ChatSession, client domain.RemoteStorage, repo, owner string) (bot.Response, error) {
	const op = "handleChosenRepo"
	chatID := session.ChatID()
	lastMessageID := session.LastBotMessageID()

	exists, err := client.RepoExists(owner, repo)
	if err != nil {
		return bot.Response{}, fmt.Errorf("%v: checking repo exists: %w", op, err)
	}
	if !exists {
		msgCfg := tgbotapi.NewEditMessageText(
			chatID,
			lastMessageID,
			"Такого репозитория не существует попробуйте еще раз командой /set-repo",
		)
		return bot.Response{Message: msgCfg}, nil
	}

	vault := domain.UserVault{
		ChatID: chatID,
		Owner:  owner,
		Repo:   repo,
	}
	if err := h.vaultService.Save(ctx, vault); err != nil {
		return bot.Response{}, fmt.Errorf("%v: saving vault: %w", op, err)
	}

	session.ToDefault()
	msgCfg := tgbotapi.NewEditMessageText(
		chatID,
		lastMessageID,
		"Репозиторий успешно установлен",
	)
	return bot.Response{Message: msgCfg}, nil
}

func (h *RepoSetHandler) RepoSetMiddleware() bot.Middleware {
	return bot.Middleware(func(next bot.Handler) bot.Handler {
		return bot.HandlerFunc{
			HandleFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) (bot.Response, error) {
				const op = "RepoSetMiddleware"
				exists, err := h.vaultService.ExistsByChatID(ctx, u.ChatID)
				if err != nil {
					return bot.Response{}, fmt.Errorf("%v: checking vault exists: %w", op, err)
				}
				if !exists {
					return bot.Response{
						Message: tgbotapi.NewMessage(u.ChatID, "Установите репозиторий в котором находится ваще хранилище через /set-repo"),
					}, nil
				}
				return next.Handle(ctx, s, u)
			},
			MatchFunc: func(ctx context.Context, s *bot.ChatSession, u bot.Update) bool {
				return next.Match(ctx, s, u)
			},
		}
	})
}
