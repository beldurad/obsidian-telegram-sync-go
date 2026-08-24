package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type OAuthContext struct {
	ChatID   int64
	State    string
	Verifier string
}

func NewOAuthContext(chatID int64) OAuthContext {

	return OAuthContext{
		ChatID:   chatID,
		State:    uuid.NewString(),
		Verifier: oauth2.GenerateVerifier(),
	}
}

func toRemoteConnectCtx(ctx OAuthContext) domain.RemoteConnectCtx {
	return domain.RemoteConnectCtx{
		ID:       ctx.ChatID,
		State:    ctx.State,
		Verifier: ctx.Verifier,
	}
}

func toRemoteToken(token *oauth2.Token) domain.RemoteToken {
	return domain.RemoteToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}
}

func toOAuthToken(token domain.RemoteToken) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.ExpiresAt,
	}
}

type RemoteConnectCtxStorage interface {
	Save(context.Context, domain.RemoteConnectCtx) error
	ContextByState(ctx context.Context, state string) (domain.RemoteConnectCtx, error)
}

type RemoteTokenStorage interface {
	Save(ctx context.Context, chatID int64, token domain.RemoteToken) error
	Token(ctx context.Context, chatID int64) (domain.RemoteToken, error)
}

type OAuthService struct {
	conf           *oauth2.Config
	contextStorage RemoteConnectCtxStorage
	tokenStorage   RemoteTokenStorage

	remoteContentCache RemoteContentCache
}

func NewOAuthService(
	cfg config.GithubConfig,
	contextStorage RemoteConnectCtxStorage,
	tokenStorage RemoteTokenStorage,
	remoteContentCache RemoteContentCache) *OAuthService {
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     github.Endpoint,
		Scopes:       strings.Split(cfg.Scopes, " "),
	}
	return &OAuthService{
		conf:               oauthCfg,
		contextStorage:     contextStorage,
		tokenStorage:       tokenStorage,
		remoteContentCache: remoteContentCache,
	}
}

func (s *OAuthService) GenerateAuthURL(ctx context.Context, chatID int64) (string, error) {
	const op = "OAuthService.GenerateAuthURL"
	oauthCtx := NewOAuthContext(chatID)

	err := s.contextStorage.Save(ctx, toRemoteConnectCtx(oauthCtx))
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

	if err := s.tokenStorage.Save(ctx, oauthCtx.ID, toRemoteToken(token)); err != nil {
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
		client: s.conf.Client(ctx, toOAuthToken(token)),
		cache:  s.remoteContentCache,
	}, nil
}
