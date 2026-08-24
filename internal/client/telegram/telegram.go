package telegram

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Client struct {
	bot             *tgbotapi.BotAPI
	webhookEndpoint string
}

func New(token string, webhookURL string, webhookEndpoint string) (*Client, error) {
	const op = "telegram.New"
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("%v: creating bot api client: %w", op, err)
	}

	webhook, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return nil, fmt.Errorf("%v: creating webhook: %w", op, err)
	}

	_, err = bot.Request(webhook)
	if err != nil {
		return nil, fmt.Errorf("%v: setting webhook to bot: %w", op, err)
	}

	return &Client{
		bot:             bot,
		webhookEndpoint: webhookEndpoint,
	}, nil
}

func (c *Client) GetUpdatesChan() <-chan tgbotapi.Update {

	return c.bot.ListenForWebhook(c.webhookEndpoint)
}

func (c *Client) Send(ch tgbotapi.Chattable) (tgbotapi.Message, error) {
	return c.bot.Send(ch)
}
