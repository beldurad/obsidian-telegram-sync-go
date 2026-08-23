package telegram

import (
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Client struct {
	bot             *tgbotapi.BotAPI
	webhookEndpoint string
}

func New(cfg config.Config) (*Client, error) {
	const op = "telegram.New"
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("%v: creating bot api client: %w", op, err)
	}

	webhook, err := tgbotapi.NewWebhook(cfg.WebhookURL)
	if err != nil {
		return nil, fmt.Errorf("%v: creating webhook: %w", op, err)
	}

	_, err = bot.Request(webhook)
	if err != nil {
		return nil, fmt.Errorf("%v: setting webhook to bot: %w", op, err)
	}

	return &Client{
		bot:             bot,
		webhookEndpoint: cfg.WebhookEndpoint,
	}, nil
}

func (c *Client) GetUpdatesChan() <-chan tgbotapi.Update {

	return c.bot.ListenForWebhook(c.webhookEndpoint)
}

func (c *Client) Send(ch tgbotapi.Chattable) (tgbotapi.Message, error) {
	return c.bot.Send(ch)
}
