package bot

import (
	"context"
	"log/slog"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
)

type LogMiddleware struct {
	log *slog.Logger
}

func NewLogMiddleware(log *slog.Logger) *LogMiddleware {
	return &LogMiddleware{
		log: log,
	}
}

func (l *LogMiddleware) Middleware() bot.Middleware {
	log := l.log
	return func(next bot.Handler) bot.Handler {
		return bot.HandlerFunc(func(ctx context.Context, s bot.ChatSession, u bot.Update) (bot.Response, error) {
			log.Info("get update", "chat_id", u.ChatID, "update_id", u.Raw.UpdateID, "update", u)
			resp, err := next.Handle(ctx, s, u)
			if err != nil {
				log.Error("error response", "chat_id", u.ChatID, "update_id", u.Raw.UpdateID, "error", err)
				return resp, err
			}
			log.Info("successful handle", "chat_id", u.ChatID, "update_id", u.Raw.UpdateID, "response", resp)
			return resp, err
		})
	}
}
