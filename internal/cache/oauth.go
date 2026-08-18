package cache

import (
	"context"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/github"
	"golang.org/x/oauth2"
)

const defCap = 10_000

type OauthContextStorage struct {
	lru *cache.LRUCache[string, *github.OAuthContext]
}

func NewOauthContextStorage() *OauthContextStorage {
	return &OauthContextStorage{
		lru: cache.NewLRU[string, *github.OAuthContext](defCap),
	}
}

func (s *OauthContextStorage) Save(ctx context.Context, c *github.OAuthContext) error {
	s.lru.Put(c.State, c)
	return nil
}

func (s *OauthContextStorage) ContextByState(ctx context.Context, state string) (*github.OAuthContext, error) {
	const op = "OauthContextStorage.ContextByState"
	oauthCtx, ok := s.lru.Get(state)
	if !ok {
		return nil, fmt.Errorf("%v: oauth context not found: %w", op, domain.ErrNotFound)
	}
	return oauthCtx, nil
}

type OAuthTokenStorage struct {
	lru *cache.LRUCache[int64, *oauth2.Token]
}

func NewOAuthTokenStorage() *OAuthTokenStorage {
	return &OAuthTokenStorage{
		lru: cache.NewLRU[int64, *oauth2.Token](defCap),
	}
}

func (s *OAuthTokenStorage) Save(ctx context.Context, chatID int64, token *oauth2.Token) error {
	s.lru.Put(chatID, token)
	return nil
}
func (s *OAuthTokenStorage) Token(ctx context.Context, chatID int64) (*oauth2.Token, error) {
	const op = "OAuthTokenStorage.Token"
	token, ok := s.lru.Get(chatID)
	if !ok {
		return nil, fmt.Errorf("%v: token not found: %w", op, domain.ErrNotFound)
	}
	return token, nil
}
