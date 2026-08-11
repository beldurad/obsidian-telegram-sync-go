package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

type GithubUser struct {
	Username string `json:"login"`
}

type GithubRepo struct {
	Name string `json:"name"`
}

const baseURL = "https://api.github.com/"

func base64Convert(raw string) string {
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

type CreateRequest struct {
	Owner   string `json:"-"`
	Repo    string `json:"-"`
	Path    string `json:"-"`
	Message string `json:"message"`
	Content string `json:"content"`
	Sha     string `json:"sha,omitempty"`
}

// Client is created by [OAuthService]
type GithubClient struct {
	client *http.Client
}

func update(ctx context.Context, client *http.Client, r CreateRequest) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	r.Content = base64Convert(r.Content)

	fullURL, err := url.JoinPath(baseURL, "repos", r.Owner, r.Repo, "contents", r.Path)
	if err != nil {
		return nil, err
	}

	byteSlice, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fullURL,
		bytes.NewReader(byteSlice),
	)
	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

func (c *GithubClient) createFile(ctx context.Context, r CreateRequest) error {
	r.Sha = ""
	resp, err := update(ctx, c.client, r)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrReqNotSend, err)
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%w: %v", domain.ErrNotSuccessfulResp, resp.StatusCode)
	}
	return nil
}

func (c *GithubClient) updateFile(ctx context.Context, r CreateRequest) error {
	resp, err := update(ctx, c.client, r)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrReqNotSend, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %v", domain.ErrNotSuccessfulResp, resp.StatusCode)
	}
	return nil
}

type GetDirRequest struct {
	Owner string
	Repo  string
	Path  string

	Offset int
	Limit  int
}

const (
	TypeFile = "file"
	TypeDir  = "dir"
)

func (c *GithubClient) UserInfo() (domain.RemoteUser, error) {
	fullURL, err := url.JoinPath(baseURL, "user")
	if err != nil {
		return domain.RemoteUser{}, err
	}
	resp, err := c.client.Get(fullURL)
	if err != nil {
		return domain.RemoteUser{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.RemoteUser{}, domain.ErrNotSuccessfulResp
	}
	decoder := json.NewDecoder(resp.Body)
	var user GithubUser

	if err := decoder.Decode(&user); err != nil {
		return domain.RemoteUser{}, err
	}
	return domain.RemoteUser{
		Username: user.Username,
	}, nil
}
func (c *GithubClient) UserRepos(username string, pageNum int, pageSize int) (domain.Page[domain.RemoteRepo], error) {
	fullURL, err := url.JoinPath(baseURL, "user", "repos")
	if err != nil {
		return domain.Page[domain.RemoteRepo]{}, err
	}
	resp, err := c.client.Get(fullURL)
	if err != nil {
		return domain.Page[domain.RemoteRepo]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.Page[domain.RemoteRepo]{}, domain.ErrNotSuccessfulResp
	}
	decoder := json.NewDecoder(resp.Body)
	var repos []GithubRepo
	if err := decoder.Decode(&repos); err != nil {
		return domain.Page[domain.RemoteRepo]{}, err
	}
	if len(repos) == 0 {
		return domain.Page[domain.RemoteRepo]{}, nil
	}
	leftBound := min(pageNum*pageSize, len(repos))
	rightBound := min(pageNum*pageSize+pageSize, len(repos))
	totalPages := int(math.Ceil(float64(len(repos)) / float64(pageSize)))
	if leftBound >= rightBound {
		return domain.Page[domain.RemoteRepo]{TotalPages: totalPages}, nil
	}
	values := make([]domain.RemoteRepo, rightBound-leftBound)
	for i := leftBound; i < rightBound; i++ {
		values[i-leftBound] = domain.RemoteRepo{
			Name: repos[i].Name,
		}
	}

	return domain.Page[domain.RemoteRepo]{
		Values:     values,
		TotalPages: totalPages,
	}, nil
}

func (c *GithubClient) RepoExists(owner, repo string) (bool, error) {
	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Get(fullURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func (c *GithubClient) Directory(owner, repo, path string, pageNum int, pageSize int) (domain.Page[domain.DirElem], error) {

	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo, "contents", path)
	if err != nil {
		return domain.Page[domain.DirElem]{}, err
	}

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return domain.Page[domain.DirElem]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.Page[domain.DirElem]{}, domain.ErrNotSuccessfulResp
	}
	var dir []domain.DirElem

	decoder := json.NewDecoder(resp.Body)

	if err := decoder.Decode(&dir); err != nil {
		return domain.Page[domain.DirElem]{}, err
	}
	leftBound := min(pageNum*pageSize, len(dir)-1)
	rightBound := min(pageNum*pageSize+pageSize, len(dir))
	totalPages := int(math.Ceil(float64(len(dir)) / float64(pageSize)))

	return domain.Page[domain.DirElem]{
		TotalPages: totalPages,
		Values:     dir[leftBound:rightBound],
	}, nil

}

func (c *GithubClient) File(owner, repo, path string) (domain.DirElem, error) {
	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo, "contents", path)
	if err != nil {
		return domain.DirElem{}, err
	}

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return domain.DirElem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.DirElem{}, domain.ErrNotSuccessfulResp
	}
	var file domain.DirElem

	decoder := json.NewDecoder(resp.Body)

	if err := decoder.Decode(&file); err != nil {
		return domain.DirElem{}, err
	}

	return file, nil
}

func (c *GithubClient) SaveNote(ctx context.Context, owner, repo string, note domain.Note) error {
	file, err := c.File(owner, repo, note.Path)
	if err == nil && file.Type == TypeFile {
		raw, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return err
		}
		return c.updateFile(ctx, CreateRequest{
			Owner:   owner,
			Repo:    repo,
			Path:    note.Path,
			Message: "update note",
			Content: renderNoteContent(string(raw), note.Text),
			Sha:     file.Sha,
		})
	}

	return c.createFile(ctx, CreateRequest{
		Owner:   owner,
		Repo:    repo,
		Path:    note.Path,
		Message: "create note",
		Content: renderNoteContent(note.Template, note.Text),
	})
}

func renderNoteContent(template, text string) string {
	if strings.Contains(template, "{}") {
		return strings.ReplaceAll(template, "{}", text)
	}
	return text
}
