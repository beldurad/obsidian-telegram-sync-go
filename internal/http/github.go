package http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	app "github.com/beldurad/obsidian-telegram-sync-go/internal"
)

const githubBaseURL = "https://api.github.com/repos"
const newFileMessage = "Added by Telegram Bot: Github Sync"

type GithubClient struct {
	baseURL string
	token   string
}

func NewGithubClient(token, username, repo string) *GithubClient {
	return &GithubClient{
		baseURL: fmt.Sprintf("%s/%s/%s/contents", githubBaseURL, username, repo),
		token:   token,
	}
}

type githubRequestBody struct {
	Message       string         `json:"message"`
	Commiter      githubCommiter `json:"commiter"`
	ContentBase64 string         `json:"content"`
}
type githubCommiter struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (c *GithubClient) SaveFile(filepath, text string) error {
	contentBase64 := base64.StdEncoding.EncodeToString([]byte(text))
	body := githubRequestBody{
		Message:       newFileMessage,
		ContentBase64: contentBase64,
		Commiter:      githubCommiter{},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/%s", c.baseURL, filepath),
		bytes.NewReader(bodyBytes),
	)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return app.ErrClient
	}
	return nil
}
