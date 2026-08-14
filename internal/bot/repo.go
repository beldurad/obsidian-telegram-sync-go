package bot

import (
	"context"
	"encoding/json"
	"log"
	"slices"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ===== STATES =====

var RepoSetState = bot.ChatState("SETTING_REPO")

// ===== COMMANDS =====

const RepoSetCommand = "/set-repo"

var RepoSetCommands = []string{
	RepoSetCommand,
	NextPageCommand,
	PrevPageCommand,
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

func NewRepoSetHandler(clientGetter ClientGetter, vaultService UserVaultService) *RepoSetHandler {
	return &RepoSetHandler{
		clientGetter: clientGetter,
		vaultService: vaultService,
	}
}

func (h *RepoSetHandler) Register(b *bot.Bot, m ...bot.Middleware) {
	b.AddHandlerForCommand(RepoSetCommand, h)
	b.AddHandlerForState(RepoSetState, h)
}

func (h *RepoSetHandler) Handle(ctx context.Context, u bot.Update) (bot.Response, error) {
	session := ctx.Value(bot.ChatSessionKey).(bot.ChatSession)
	chatID := session.ChatID
	state := session.State

	client, err := h.clientGetter.Client(ctx, chatID)
	if err != nil {
		return bot.Response{}, err
	}
	user, err := client.UserInfo()
	if err != nil {
		return bot.Response{}, err
	}

	if state == RepoSetState &&
		!slices.Contains(RepoSetCommands, u.Raw.CallbackData()) {
		log.Println("repo chosen")
		return h.handleChosenRepo(ctx, session, client, u.Raw.CallbackData(), user.Username)
	}

	var payload pagePayload
	if session.State != bot.DefaultChatState {
		raw := session.Payload
		err := json.Unmarshal([]byte(raw), &payload)
		if err != nil {
			log.Printf("error while unmarshalling: %v", err)
			return bot.Response{}, err
		}
		log.Printf("unmarshalled previous payload: %v", payload)
		switch u.Raw.CallbackData() {
		case NextPageCommand:
			payload.PageNum++
		case PrevPageCommand:
			payload.PageNum = max(0, payload.PageNum-1)
		}
	}

	log.Printf("payload: %v", payload)
	repoPage, err := client.UserRepos(user.Username, payload.PageNum, domain.DefaultPageSize)
	if err != nil {
		return bot.Response{}, err
	}
	buttons := make([][]tgbotapi.InlineKeyboardButton, len(repoPage.Values))
	for i, repo := range repoPage.Values {
		buttons[i] = []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(repo.Name, repo.Name),
		}
	}
	if repoPage.HasNext() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Next", NextPageCommand),
		})
	}
	if repoPage.HasPrev() {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Prev", PrevPageCommand),
		})
	}
	markup := tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: buttons,
	}

	var c tgbotapi.Chattable

	if session.State == bot.DefaultChatState || session.LastBotMessageID == 0 {
		msgCfg := tgbotapi.NewMessage(chatID, "Выберите репозиторий хранилища")
		msgCfg.ReplyMarkup = markup
		c = msgCfg
	} else {
		msgCfg := tgbotapi.NewEditMessageCaption(session.ChatID, session.LastBotMessageID, "Выберите репозиторий хранилища")
		msgCfg.ReplyMarkup = &markup
		c = msgCfg
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return bot.Response{}, err
	}

	return bot.Response{
		Message:      c,
		NewChatState: &RepoSetState,
		NewPayload:   bytes,
	}, nil

}

func (h *RepoSetHandler) handleChosenRepo(ctx context.Context, session bot.ChatSession, client domain.RemoteStorage, repo, owner string) (bot.Response, error) {
	chatID := session.ChatID
	lastMessageID := session.LastBotMessageID
	exists, err := client.RepoExists(owner, repo)
	if err != nil {
		return bot.Response{}, err
	}
	if !exists {
		msgCfg := tgbotapi.NewEditMessageText(
			chatID,
			lastMessageID,
			"Такого репозитория не существует попробуйте еще раз командой /set-repo",
		)
		return bot.Response{
			Message: msgCfg,
		}, nil
	}
	vault := domain.UserVault{
		ChatID: chatID,
		Owner:  owner,
		Repo:   repo,
	}
	if err := h.vaultService.Save(ctx, vault); err != nil {
		return bot.Response{}, err
	}
	msgCfg := tgbotapi.NewEditMessageText(
		chatID,
		lastMessageID,
		"Репозиторий успешно установлен",
	)
	return bot.Response{
		Message: msgCfg,
	}, nil
}

func (h *RepoSetHandler) RepoSetMiddleware() bot.Middleware {
	return bot.Middleware(func(next bot.Handler) bot.Handler {
		return bot.HandlerFunc(func(ctx context.Context, u bot.Update) (bot.Response, error) {
			exists, err := h.vaultService.ExistsByChatID(ctx, u.ChatID)
			if err != nil {
				return bot.Response{}, err
			}
			if !exists {
				return bot.Response{
					Message: tgbotapi.NewMessage(u.ChatID, "Установите репозиторий в котором находится ваще хранилище через /set-repo"),
				}, nil
			}
			return next.Handle(ctx, u)
		})
	})
}
