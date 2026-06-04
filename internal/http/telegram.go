package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	app "github.com/beldurad/obsidian-telegram-sync-go/internal"
)

const baseURL = "https://api.telegram.org"
const timeout = 30

type response struct {
	Ok     string   `json:"ok"`
	Result []update `json:"result"`
}

type update struct {
	UpdateID int `json:"update_id"`
	message  `json:"message"`
}

func mapToUserMessage(u update) app.UserMessage {
	return app.UserMessage{
		From: u.From.Username,
		Text: u.Text,
	}
}

type message struct {
	ChatID int    `json:"chat_id"`
	Text   string `json:"text"`
	From   user   `json:"from"`
}

type user struct {
	Username string `json:"username"`
}

type TelegramClient struct {
	baseURL      string
	lastUpdateID int
	messages     chan<- app.UserMessage
	responses    <-chan app.BotResponse
}

func NewTelegramClient(token string, messages chan<- app.UserMessage, responses <-chan app.BotResponse) *TelegramClient {
	return &TelegramClient{
		baseURL:      fmt.Sprintf("%s/bot%s", baseURL, token),
		lastUpdateID: -1,
		messages:     messages,
		responses:    responses,
	}
}

func (c *TelegramClient) StartUpdateReader() {
	for {
		messages, err := c.GetUpdates()
		if err != nil {
			continue
		}
		for _, m := range messages {
			c.messages <- m
		}
	}
}

func (c *TelegramClient) StartResponseWriter() {
	for r := range c.responses {
		c.SendBotResponse(r)
	}

}

func (c *TelegramClient) GetUpdates() ([]app.UserMessage, error) {
	httpResp, err := http.Get(
		fmt.Sprintf(
			"%s/getUpdates?offset=%d;timeout=%d",
			c.baseURL,
			c.lastUpdateID,
			timeout,
		))
	if err != nil {
		return nil, err
	}
	var resp response
	json.NewDecoder(httpResp.Body).Decode(&resp)

	c.lastUpdateID = resp.Result[len(resp.Result)-1].UpdateID + 1

	result := make([]app.UserMessage, len(resp.Result))

	for i := range len(resp.Result) {
		result[i] = mapToUserMessage(resp.Result[i])
	}
	return result, err
}

func (c *TelegramClient) SendBotResponse(r app.BotResponse) error {
	byteResp, err := json.Marshal(r)
	if err != nil {
		return err
	}
	httpResp, err := http.Post(fmt.Sprintf("%s/sendMessage", c.baseURL), "application/json", bytes.NewReader(byteResp))
	if err != nil {
		return err
	}
	if httpResp.StatusCode != 200 {
		return app.ErrClient
	}
	return nil
}
