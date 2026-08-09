package domain

// ======== GITHUB AUTH ==========
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

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type GithubUser struct {
	Username string `json:"login"`
}

type GithubRepo struct {
	Name string `json:"name"`
}

type OAuthContext struct {
	ChatID   int64
	State    string
	Verifier string
}

func NewOAuthContext(chatID int64) *OAuthContext {

	return &OAuthContext{
		ChatID:   chatID,
		State:    uuid.NewString(),
		Verifier: oauth2.GenerateVerifier(),
	}
}

type OAuthContextStorage interface {
	Save(context.Context, *OAuthContext) error
	ContextByState(ctx context.Context, state string) (*OAuthContext, error)
}

type OAuthTokenStorage interface {
	Save(ctx context.Context, chatID int64, token *oauth2.Token) error
	Token(ctx context.Context, chatID int64) (*oauth2.Token, error)
}

type OAuthService struct {
	conf           *oauth2.Config
	contextStorage OAuthContextStorage
	tokenStorage   OAuthTokenStorage
}

func NewOAuthService(oauthConfig *oauth2.Config, contextStorage OAuthContextStorage, tokenStorage OAuthTokenStorage) *OAuthService {
	return &OAuthService{
		conf:           oauthConfig,
		contextStorage: contextStorage,
		tokenStorage:   tokenStorage,
	}
}

func (s *OAuthService) GenerateAuthURL(ctx context.Context, chatID int64) (string, error) {
	oauthCtx := NewOAuthContext(chatID)

	err := s.contextStorage.Save(ctx, oauthCtx)
	if err != nil {
		return "", err
	}

	return s.conf.AuthCodeURL(
			oauthCtx.State,
			oauth2.AccessTypeOffline,
			oauth2.S256ChallengeOption(oauthCtx.Verifier)),
		nil
}

func (s *OAuthService) CompleteAuth(ctx context.Context, code string, state string) error {
	oauthCtx, err := s.contextStorage.ContextByState(ctx, state)
	if err != nil {
		return err
	}

	token, err := s.conf.Exchange(ctx, code, oauth2.VerifierOption(oauthCtx.Verifier))
	if err != nil {
		return err
	}

	if err := s.tokenStorage.Save(ctx, oauthCtx.ChatID, token); err != nil {
		return err
	}
	return nil
}

func (s *OAuthService) Client(ctx context.Context, chatID int64) (*GithubClient, error) {
	token, err := s.tokenStorage.Token(ctx, chatID)
	if err != nil {
		return nil, err
	}

	return &GithubClient{
		client: s.conf.Client(ctx, token),
	}, nil
}

// ======== CLIENT =========
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

func (c *GithubClient) CreateFile(ctx context.Context, r CreateRequest) error {
	r.Sha = ""
	resp, err := update(ctx, c.client, r)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrReqNotSend, err)
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("%w: %v", ErrNotSuccessfulResp, resp.StatusCode)
	}
	return nil
}

func (c *GithubClient) UpdateFile(ctx context.Context, r CreateRequest) error {
	resp, err := update(ctx, c.client, r)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrReqNotSend, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %v", ErrNotSuccessfulResp, resp.StatusCode)
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

type DirElem struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Sha     string `json:"sha"`
}

const (
	TypeFile = "file"
	TypeDir  = "dir"
)

func (c *GithubClient) UserInfo() (GithubUser, error) {
	fullURL, err := url.JoinPath(baseURL, "user")
	if err != nil {
		return GithubUser{}, err
	}
	resp, err := c.client.Get(fullURL)
	if err != nil {
		return GithubUser{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GithubUser{}, ErrNotSuccessfulResp
	}
	decoder := json.NewDecoder(resp.Body)
	var user GithubUser

	if err := decoder.Decode(&user); err != nil {
		return GithubUser{}, err
	}
	return user, nil
}
func (c *GithubClient) UserRepos(username string, pageNum int, pageSize int) (Page[GithubRepo], error) {
	fullURL, err := url.JoinPath(baseURL, "user", "repos")
	if err != nil {
		return Page[GithubRepo]{}, err
	}
	resp, err := c.client.Get(fullURL)
	if err != nil {
		return Page[GithubRepo]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page[GithubRepo]{}, ErrNotSuccessfulResp
	}
	decoder := json.NewDecoder(resp.Body)
	var repos []GithubRepo
	if err := decoder.Decode(&repos); err != nil {
		return Page[GithubRepo]{}, err
	}
	if len(repos) == 0 {
		return Page[GithubRepo]{}, nil
	}
	leftBound := min(pageNum*pageSize, len(repos))
	rightBound := min(pageNum*pageSize+pageSize, len(repos))
	totalPages := int(math.Ceil(float64(len(repos)) / float64(pageSize)))
	if leftBound >= rightBound {
		return Page[GithubRepo]{TotalPages: totalPages}, nil
	}

	return Page[GithubRepo]{
		Values:     repos[leftBound:rightBound],
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

func (c *GithubClient) Directory(owner, repo, path string, pageNum int, pageSize int) (Page[DirElem], error) {
	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo, "contents", path)
	if err != nil {
		return Page[DirElem]{}, err
	}

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return Page[DirElem]{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page[DirElem]{}, ErrNotSuccessfulResp
	}
	var dir []DirElem

	decoder := json.NewDecoder(resp.Body)

	if err := decoder.Decode(&dir); err != nil {
		return Page[DirElem]{}, err
	}
	leftBound := min(pageNum*pageSize, len(dir)-1)
	rightBound := min(pageNum*pageSize+pageSize, len(dir))
	totalPages := int(math.Ceil(float64(len(dir)) / float64(pageSize)))

	return Page[DirElem]{
		TotalPages: totalPages,
		Values:     dir[leftBound:rightBound],
	}, nil

}

func (c *GithubClient) File(owner, repo, path string) (DirElem, error) {
	fullURL, err := url.JoinPath(baseURL, "repos", owner, repo, "contents", path)
	if err != nil {
		return DirElem{}, err
	}

	resp, err := c.client.Get(fullURL)
	if err != nil {
		return DirElem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DirElem{}, ErrNotSuccessfulResp
	}
	var file DirElem

	decoder := json.NewDecoder(resp.Body)

	if err := decoder.Decode(&file); err != nil {
		return DirElem{}, err
	}

	return file, nil
}

func (c *GithubClient) SaveNote(ctx context.Context, owner, repo string, note Note) error {
	file, err := c.File(owner, repo, note.Path)
	if err == nil && file.Type == TypeFile {
		raw, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return err
		}
		return c.UpdateFile(ctx, CreateRequest{
			Owner:   owner,
			Repo:    repo,
			Path:    note.Path,
			Message: "update note",
			Content: renderNoteContent(string(raw), note.Text),
			Sha:     file.Sha,
		})
	}

	return c.CreateFile(ctx, CreateRequest{
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
