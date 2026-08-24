package cache

import (
	"time"

	"github.com/beldurad/obsidian-telegram-sync-go/foundation/cache"
	"github.com/beldurad/obsidian-telegram-sync-go/internal/domain"
)

type remoteContentKey struct {
	owner    string
	repoName string
	path     string
}

type RemoteContentCache struct {
	lru *cache.LRUCache[remoteContentKey, []domain.File]
}

func NewRemoteContentCache() *RemoteContentCache {
	return &RemoteContentCache{
		lru: cache.NewLRU(
			cache.
				WithTTL[remoteContentKey, []domain.File](
				30 * time.Minute,
			),
		),
	}
}

func (c *RemoteContentCache) Put(owner, repoName, path string, content []domain.File) {
	c.lru.Put(remoteContentKey{
		owner:    owner,
		repoName: repoName,
		path:     path,
	}, content)
}

func (c *RemoteContentCache) Get(owner, repoName, path string) ([]domain.File, bool) {
	return c.lru.Get(remoteContentKey{
		owner:    owner,
		repoName: repoName,
		path:     path,
	})
}
