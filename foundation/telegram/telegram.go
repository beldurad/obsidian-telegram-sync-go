package telegram

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Client struct {
	bot *tgbotapi.BotAPI
}

func New(token string) (*Client, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Client{
		bot: bot,
	}, nil
}

func (c *Client) GetUpdatesChan() <-chan tgbotapi.Update {
	return c.bot.GetUpdatesChan(tgbotapi.NewUpdate(0))
}

func (c *Client) Send(ch tgbotapi.Chattable) (tgbotapi.Message, error) {
	return c.bot.Send(ch)
}
