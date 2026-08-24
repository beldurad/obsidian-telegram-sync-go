package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/client"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

type GithubUser struct {
	Username string `json:"login"`
}

type GithubRepo struct {
	Name string `json:"name"`
}

type GithubFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	PathType string `json:"type"`
	Content  string `json:"content"`
	Sha      string `json:"sha"`
}

func (f GithubFile) toDomainFile() domain.File {
	return domain.File{
		Name: f.Name,
		Path: domain.Path{
			Value: f.Path,
			Type:  f.PathType,
		},
		Content: f.Content,
	}
}

const baseURL = "https://api.github.com/"

func base64Encode(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func base64Decode(raw string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type CreateRequest struct {
	Owner   string `json:"-"`
	Repo    string `json:"-"`
	Path    string `json:"-"`
	Message string `json:"message"`
	Content string `json:"content"`
	Sha     string `json:"sha,omitempty"`
}

type GetRequest struct {
	Owner string
	Repo  string
	Path  string
}

type RemoteContentCache interface {
	Put(owner, repoName, path string, content []domain.File)
	Get(owner, repoName, path string) ([]domain.File, bool)
}

// Client is created by [OAuthService]
type GithubClient struct {
	client *http.Client
	cache  RemoteContentCache
}

func update(ctx context.Context, client *http.Client, r CreateRequest) (*http.Response, error) {
	const op = "update"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	r.Content = base64Encode(r.Content)

	fullURL, err := url.JoinPath(baseURL, "repos", r.Owner, r.Repo, "contents", r.Path)
	if err != nil {
		return nil, fmt.Errorf("%v: building url: %w", op, err)
	}

	byteSlice, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("%v: marshaling request: %w", op, err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fullURL,
		bytes.NewReader(byteSlice),
	)
	if err != nil {
		return nil, fmt.Errorf("%v: creating request: %w", op, err)
	}

	return client.Do(req)
}

func (c *GithubClient) CreateNote(ctx context.Context, owner, repo string, n domain.Note) error {
	const op = "createFile"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.file(ctx, owner, repo, n.Path)
	if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%v: getting file: %w", op, domain.ErrAlreadyExists)
	}

	resp, err := update(ctx, c.client, CreateRequest{
		Owner:   owner,
		Repo:    repo,
		Path:    n.Path,
		Message: "updating file",
		Content: renderNoteContent(n.Template, n.Text),
	})
	if err != nil {
		return fmt.Errorf("%v: updating file: %w: %w", op, domain.ErrClient, err)
	}

	if err := client.ConvertStatusCodeToError(resp.StatusCode); err != nil {
		return fmt.Errorf("%v: updating file: %w: status %v", op, err, resp.StatusCode)
	}
	return nil
}

func (c *GithubClient) UpdateNote(ctx context.Context, owner, repo string, n domain.Note) error {
	const op = "updateFile"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	f, err := c.file(ctx, owner, repo, n.Path)
	if err != nil {
		return fmt.Errorf("%v: getting file: %w", op, err)
	}

	content, err := base64Decode(f.Content)
	if err != nil {
		return fmt.Errorf("%v: decoding file content: %w", op, err)
	}

	resp, err := update(ctx, c.client, CreateRequest{
		Owner:   owner,
		Repo:    repo,
		Path:    n.Path,
		Message: "updating file",
		Content: content + "\n" + renderNoteContent(n.Template, n.Text),
		Sha:     f.Sha,
	})
	if err != nil {
		return fmt.Errorf("%v: updating file: %w: %w", op, domain.ErrClient, err)
	}

	if err := client.ConvertStatusCodeToError(resp.StatusCode); err != nil {
		return fmt.Errorf("%v: updating file: %w: status %v", op, err, resp.StatusCode)
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
	const op = "GithubClient.UserInfo"

	fullURL, err := url.JoinPath(baseURL, "user")
	if err != nil {
		return domain.RemoteUser{}, fmt.Errorf("%v: building url: %w", op, err)
	}

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return domain.RemoteUser{}, fmt.Errorf("%v: requesting user: %w", op, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.RemoteUser{}, fmt.Errorf("%v: %w: status %v", op, domain.ErrClient, resp.StatusCode)
	}

	decoder := json.NewDecoder(resp.Body)
	var user GithubUser

	if err := decoder.Decode(&user); err != nil {
		return domain.RemoteUser{}, fmt.Errorf("%v: decoding user: %w", op, err)
	}
	return domain.RemoteUser{
		Username: user.Username,
	}, nil
}

func getTotalPages(resp *http.Response) (int, error) {
	link := resp.Header.Get("Link")
	if link == "" {
		return 1, nil
	}

	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)

		if !strings.Contains(part, `rel="last"`) {
			continue
		}

		start := strings.Index(part, "<")
		end := strings.Index(part, ">")

		if start == -1 || end == -1 || start >= end {
			return 0, fmt.Errorf("invalid Link header: %q", link)
		}

		lastURL, err := url.Parse(part[start+1 : end])
		if err != nil {
			return 0, fmt.Errorf("parsing last URL: %w", err)
		}

		pageStr := lastURL.Query().Get("page")
		if pageStr == "" {
			return 0, fmt.Errorf("page parameter not found in Link header")
		}

		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return 0, fmt.Errorf("invalid page number %q: %w", pageStr, err)
		}

		return page, nil
	}
	return 1, nil
}
func (c *GithubClient) UserRepos(username string, pageNum int, pageSize int) (domain.Page[domain.RemoteRepo], error) {
	const op = "GithubClient.UserRepos"

	fullURL, err := url.JoinPath(baseURL, "user", "repos")
	if err != nil {
		return domain.Page[domain.RemoteRepo]{}, fmt.Errorf("%v: building url: %w", op, err)
	}

	params := url.Values{}
	// Github pagination starts with 1, domain pagination - with 0
	params.Set("page", strconv.Itoa(pageNum+1))
	params.Set("per_page", strconv.Itoa(pageSize))
	fullURL += "?" + params.Encode()

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return domain.Page[domain.RemoteRepo]{}, fmt.Errorf("%v: requesting repos: %w", op, err)
	}
	defer resp.Body.Close()

	if err := client.ConvertStatusCodeToError(resp.StatusCode); err != nil {
		return domain.Page[domain.RemoteRepo]{}, fmt.Errorf("%v: %w", op, err)
	}

	var repos []GithubRepo

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&repos); err != nil {
		return domain.Page[domain.RemoteRepo]{}, fmt.Errorf("%v: decoding repos: %w", op, err)
	}

	totalPages, err := getTotalPages(resp)
	if err != nil {
		return domain.Page[domain.RemoteRepo]{}, fmt.Errorf("%v: while getting total pages: %w", op, err)
	}

	values := make([]domain.RemoteRepo, len(repos))
	for i := range values {
		values[i] = domain.RemoteRepo{
			Name: repos[i].Name,
		}
	}

	return domain.Page[domain.RemoteRepo]{
		Values:     values,
		CurPage:    pageNum,
		TotalPages: totalPages,
	}, nil
}

func (c *GithubClient) RepoExists(owner, repo string) (bool, error) {
	const op = "GithubClient.RepoExists"

	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo)
	if err != nil {
		return false, fmt.Errorf("%v: building url: %w", op, err)
	}

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return false, fmt.Errorf("%v: requesting repo: %w", op, err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func (c *GithubClient) Directory(owner, repo, path string, pageNum int, pageSize int) (domain.Page[domain.File], error) {
	const op = "GithubClient.Directory"

	if pageNum < 0 {
		return domain.Page[domain.File]{}, fmt.Errorf(
			"%v: invalid page number: %d", op, pageNum,
		)
	}
	if pageSize <= 0 {
		return domain.Page[domain.File]{}, fmt.Errorf(
			"%v: invalid page size: %d", op, pageSize,
		)
	}

	var dir []domain.File

	if content, ok := c.cache.Get(owner, repo, path); ok {
		dir = content
	} else {
		fullURL, err := url.JoinPath(baseURL, "repos", owner, repo, "contents", path)
		if err != nil {
			return domain.Page[domain.File]{}, fmt.Errorf("%v: building url: %w", op, err)
		}

		resp, err := c.client.Get(fullURL)
		if err != nil {
			return domain.Page[domain.File]{}, fmt.Errorf("%v: requesting directory: %w", op, err)
		}
		defer resp.Body.Close()

		if err := client.ConvertStatusCodeToError(resp.StatusCode); err != nil {
			return domain.Page[domain.File]{}, fmt.Errorf("%v: %w", op, err)
		}

		var githubDir []GithubFile

		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&githubDir); err != nil {
			return domain.Page[domain.File]{}, fmt.Errorf("%v: decoding directory: %w: %w", op, domain.ErrNotDirectory, err)
		}

		dir = make([]domain.File, len(githubDir))
		for i := range githubDir {
			dir[i] = githubDir[i].toDomainFile()
		}
		c.cache.Put(owner, repo, path, dir)
	}

	length := len(dir)
	leftBound := min(pageNum*pageSize, length)
	rightBound := min(pageNum*pageSize+pageSize, length)
	totalPages := length / pageSize
	if length%pageSize != 0 {
		totalPages++
	}

	return domain.Page[domain.File]{
		TotalPages: totalPages,
		CurPage:    pageNum,
		Values:     dir[leftBound:rightBound],
	}, nil

}

func (c *GithubClient) File(ctx context.Context, owner, repo, path string) (domain.File, error) {
	const op = "GithubClient.File"
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	f, err := c.file(ctx, owner, repo, path)
	if err != nil {
		return domain.File{}, fmt.Errorf("%v: getting file: %w", op, err)
	}
	return f.toDomainFile(), nil
}

func (c *GithubClient) file(ctx context.Context, owner, repo, path string) (GithubFile, error) {
	const op = "file"
	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo, "contents", path)
	if err != nil {
		return GithubFile{}, fmt.Errorf("%v: building url: %w", op, err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fullURL,
		nil,
	)
	if err != nil {

	}
	resp, err := c.client.Do(req)
	if err != nil {
		return GithubFile{}, fmt.Errorf("%v: requesting file: %w", op, err)
	}
	defer resp.Body.Close()

	if err := client.ConvertStatusCodeToError(resp.StatusCode); err != nil {
		return GithubFile{}, fmt.Errorf("%v: %w", op, err)
	}

	var file GithubFile

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&file); err != nil {
		return GithubFile{}, fmt.Errorf("%v: decoding file: %w", op, err)
	}

	return file, nil
}

func renderNoteContent(template, text string) string {
	if strings.Contains(template, "{}") {
		return strings.ReplaceAll(template, "{}", text)
	}
	return text
}
