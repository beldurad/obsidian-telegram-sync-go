package cache

import (
	"context"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

type RemoteConnectCtxStorage struct {
	lru *cache.LRUCache[string, domain.RemoteConnectCtx]
}

func NewRemoteConnectCtxStorage() *RemoteConnectCtxStorage {
	return &RemoteConnectCtxStorage{
		lru: cache.NewLRU[string, domain.RemoteConnectCtx](),
	}
}

func (s *RemoteConnectCtxStorage) Save(ctx context.Context, c domain.RemoteConnectCtx) error {
	s.lru.Put(c.State, c)
	return nil
}

func (s *RemoteConnectCtxStorage) ContextByState(ctx context.Context, state string) (domain.RemoteConnectCtx, error) {
	const op = "RemoteConnectCtxStorage.ContextByState"
	oauthCtx, ok := s.lru.Get(state)
	if !ok {
		return domain.RemoteConnectCtx{}, fmt.Errorf("%v: oauth context not found: %w", op, domain.ErrNotFound)
	}
	return oauthCtx, nil
}

type RemoteTokenStorage struct {
	lru *cache.LRUCache[int64, domain.RemoteToken]
}

func NewRemoteTokenStorage() *RemoteTokenStorage {
	return &RemoteTokenStorage{
		lru: cache.NewLRU[int64, domain.RemoteToken](),
	}
}

func (s *RemoteTokenStorage) Save(ctx context.Context, chatID int64, token domain.RemoteToken) error {
	s.lru.Put(chatID, token)
	return nil
}
func (s *RemoteTokenStorage) Token(ctx context.Context, chatID int64) (domain.RemoteToken, error) {
	const op = "RemoteTokenStorage.Token"
	token, ok := s.lru.Get(chatID)
	if !ok {
		return domain.RemoteToken{}, fmt.Errorf("%v: token not found: %w", op, domain.ErrNotFound)
	}
	return token, nil
}
