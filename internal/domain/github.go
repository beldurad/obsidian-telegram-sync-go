package domain

// ======== GITHUB AUTH ==========
import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type GithubUser struct {
	Username string
}

type GithubRepo struct {
	Name string
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
const baseURL = "https://api.github.com/repos/"

func toFullURL(owner string, repo string, path string) string {
	res, err := url.JoinPath(baseURL, owner, repo, "contents", path)
	if err != nil {
		panic("Wrong URL")
	}
	return res
}

func base64Convert(raw string) string {
	data := []byte(raw)
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(dst, data)
	return string(data)
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

	url := toFullURL(r.Owner, r.Repo, r.Path)

	byteSlice, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		url,
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
	Type string `json:"type"`
	Name string `json:"name"`
}

const (
	TypeFile = "file"
	TypeDir  = "dir"
)

func (c *GithubClient) UserInfo() (GithubUser, error) {
	panic("")
}
func (c *GithubClient) UserRepos(username string, pageNum int) (Page[GithubRepo], error) {
	panic("")
}

func (c *GithubClient) RepoExists(owner, repo string) (bool, error) {
	panic("")
}

func (c *GithubClient) Directory(owner, repo, path string, pageNum int) (Page[DirElem], error) {
	panic("")
}

func (c *GithubClient) SaveNote(owner, repo string, note Note) error {
	panic("")
}
