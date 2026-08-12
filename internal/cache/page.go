package cache

import (
	"context"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/cache"
)

type pageCountKey struct {
	chatID   int64
	pageSize int
}

type PageCountCache struct {
	lru *cache.LRUCache[pageCountKey, int]
}

func NewPageCountCache() *PageCountCache {
	return &PageCountCache{
		lru: cache.NewLRU[pageCountKey, int](defCap),
	}
}

func (c *PageCountCache) Put(ctx context.Context, chatID int64, pageSize, count int) error {
	c.lru.Put(pageCountKey{
		chatID:   chatID,
		pageSize: pageSize,
	},
		count,
	)
	return nil
}

func (c *PageCountCache) Get(ctx context.Context, chatID int64, pageSize int) (count int, ok bool) {
	count, ok = c.lru.Get(pageCountKey{
		chatID:   chatID,
		pageSize: pageSize,
	})
	return
}

func (c *PageCountCache) Delete(ctx context.Context, chatID int64, pageSize int) error {
	c.lru.Delete(pageCountKey{
		chatID:   chatID,
		pageSize: pageSize,
	})
	return nil
}
