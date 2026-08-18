package github

import (
	"context"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

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
	const op = "OAuthService.GenerateAuthURL"
	oauthCtx := NewOAuthContext(chatID)

	err := s.contextStorage.Save(ctx, oauthCtx)
	if err != nil {
		return "", fmt.Errorf("%v: saving oauth context: %w", op, err)
	}

	return s.conf.AuthCodeURL(
			oauthCtx.State,
			oauth2.AccessTypeOffline,
			oauth2.S256ChallengeOption(oauthCtx.Verifier)),
		nil
}

func (s *OAuthService) CompleteAuth(ctx context.Context, code string, state string) error {
	const op = "OAuthService.CompleteAuth"
	oauthCtx, err := s.contextStorage.ContextByState(ctx, state)
	if err != nil {
		return fmt.Errorf("%v: getting oauth context by state: %w", op, err)
	}

	token, err := s.conf.Exchange(ctx, code, oauth2.VerifierOption(oauthCtx.Verifier))
	if err != nil {
		return fmt.Errorf("%v: exchanging code for token: %w", op, err)
	}

	if err := s.tokenStorage.Save(ctx, oauthCtx.ChatID, token); err != nil {
		return fmt.Errorf("%v: saving token: %w", op, err)
	}
	return nil
}

func (s *OAuthService) Client(ctx context.Context, chatID int64) (domain.RemoteStorage, error) {
	const op = "OAuthService.Client"
	token, err := s.tokenStorage.Token(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("%v: getting token: %w", op, err)
	}

	return &GithubClient{
		client: s.conf.Client(ctx, token),
	}, nil
}
