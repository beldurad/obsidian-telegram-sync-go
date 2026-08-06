package cache

import (
	"context"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
	"golang.org/x/oauth2"
)

const defCap = 10_000

type OauthContextStorage struct {
	lru *cache.LRUCache[string, *domain.OAuthContext]
}

func NewOauthContextStorage() *OauthContextStorage {
	return &OauthContextStorage{
		lru: cache.NewLRU[string, *domain.OAuthContext](defCap),
	}
}

func (s *OauthContextStorage) Save(ctx context.Context, c *domain.OAuthContext) error {
	s.lru.Put(c.State, c)
	return nil
}

func (s *OauthContextStorage) ContextByState(ctx context.Context, state string) (*domain.OAuthContext, error) {
	oauthCtx, ok := s.lru.Get(state)
	if !ok {
		return nil, domain.ErrNotFound
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
	token, ok := s.lru.Get(chatID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	return token, nil
}
