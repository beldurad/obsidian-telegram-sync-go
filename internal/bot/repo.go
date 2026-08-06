package bot

import (
	"context"
	"encoding/json"
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
	Client(ctx context.Context, chatID int64) (*domain.GithubClient, error)
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

	if state != bot.DefaultChatState &&
		!slices.Contains(RepoSetCommands, u.Raw.CallbackData()) {

		h.handleChosenRepo(ctx, session, client, u.Raw.CallbackData(), user.Username)
	}

	var payload pagePayload
	if state == bot.DefaultChatState {
		payload = pagePayload{
			PageNum: 0,
		}
	} else {
		raw := session.Payload
		var prev pagePayload
		err := json.Unmarshal([]byte(raw), &prev)
		if err != nil {
			return bot.Response{}, err
		}
		if u.Text == NextPageCommand {
			payload.PageNum++
		} else if u.Text == PrevPageCommand {
			payload.PageNum--
		}
	}
	repoPage, err := client.UserRepos(user.Username, payload.PageNum)
	if err != nil {
		return bot.Response{}, err
	}
	buttons := make([][]tgbotapi.InlineKeyboardButton, len(repoPage.Values))
	for i, repo := range repoPage.Values {
		buttons[i] = []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(repo.Name, repo.Name),
		}
	}
	if payload.PageNum < repoPage.TotalPages-1 {
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Next", NextPageCommand),
		})
	}
	if payload.PageNum > 0 {
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
		return bot.Response{}, nil
	}

	return bot.Response{
		Message:      c,
		NewChatState: &RepoSetState,
		NewPayload:   string(bytes),
	}, nil

}

func (h *RepoSetHandler) handleChosenRepo(ctx context.Context, session bot.ChatSession, client *domain.GithubClient, repo, owner string) (bot.Response, error) {
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
		return bot.Response{}, nil
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
				return bot.Response{}, nil
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
