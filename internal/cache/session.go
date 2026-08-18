package cache

import (
	"github.com/beldurad/obsidian-telegram-sync-go/foundation/bot"
	"github.com/beldurad/obsidian-telegram-sync-go/foundation/cache"
)

type ChatSessionService struct {
	lru *cache.LRUCache[int64, *bot.ChatSession]
}

var _ bot.ChatSessionService = &ChatSessionService{}

func NewChatSessionService() *ChatSessionService {
	return &ChatSessionService{
		lru: cache.NewLRU[int64, *bot.ChatSession](defCap),
	}
}

func (s *ChatSessionService) SessionByChatID(chatID int64) (*bot.ChatSession, error) {
	session, ok := s.lru.Get(chatID)
	if !ok {
		return bot.NewChatSession(chatID), nil
	}
	return session, nil

}

func (s *ChatSessionService) UpdateSession(new *bot.ChatSession) error {
	s.lru.Put(new.ChatID(), new)
	return nil
}
