// Package chat provides a memoized read access to chat history and user
// profiles obtained from the userbot.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"telegram-assistant/internal/cache"
	"telegram-assistant/internal/userbot"
)

const (
	historyKeyPrefix = "chat_history:"
	profileKeyPrefix = "user_profile:"
	historyTTL       = 30 * time.Second
	profileTTL       = time.Hour
)

// Source is the userbot-facing interface used by the service.
type Source interface {
	History(ctx context.Context, chatID int64, limit int) ([]userbot.Message, error)
	Profile(ctx context.Context, userID int64) (*userbot.UserProfile, error)
}

// Service reads data through the userbot and memoizes results in a cache.
// Concurrent requests for the same key are deduplicated with singleflight.
type Service struct {
	ub    Source
	cache cache.Cache
	group singleflight.Group
}

// New creates a chat service.
func New(ub Source, c cache.Cache) *Service {
	return &Service{ub: ub, cache: c}
}

// History returns the most recent messages of a chat, newest first,
// memoized for historyTTL.
func (s *Service) History(ctx context.Context, chatID int64, limit int) ([]userbot.Message, error) {
	key := fmt.Sprintf("%s%d:%d", historyKeyPrefix, chatID, limit)

	if msgs, ok := s.getHistoryCached(ctx, key); ok {
		return msgs, nil
	}

	result, err, _ := s.group.Do(key, func() (interface{}, error) {
		if msgs, ok := s.getHistoryCached(ctx, key); ok {
			return msgs, nil
		}

		msgs, err := s.ub.History(ctx, chatID, limit)
		if err != nil {
			return nil, err
		}
		if data, err := json.Marshal(msgs); err == nil {
			if err := s.cache.Set(ctx, key, string(data), historyTTL); err != nil {
				return nil, err
			}
		}
		return msgs, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]userbot.Message), nil
}

func (s *Service) getHistoryCached(ctx context.Context, key string) ([]userbot.Message, bool) {
	raw, ok, err := s.cache.Get(ctx, key)
	if err != nil || !ok {
		return nil, false
	}
	var msgs []userbot.Message
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return nil, false
	}
	return msgs, true
}

// Profile returns a user profile, memoized for profileTTL.
func (s *Service) Profile(ctx context.Context, userID int64) (*userbot.UserProfile, error) {
	key := fmt.Sprintf("%s%d", profileKeyPrefix, userID)

	if p, ok := s.getProfileCached(ctx, key); ok {
		return p, nil
	}

	result, err, _ := s.group.Do(key, func() (interface{}, error) {
		if p, ok := s.getProfileCached(ctx, key); ok {
			return p, nil
		}

		p, err := s.ub.Profile(ctx, userID)
		if err != nil {
			return nil, err
		}
		if data, err := json.Marshal(p); err == nil {
			if err := s.cache.Set(ctx, key, string(data), profileTTL); err != nil {
				return nil, err
			}
		}
		return p, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*userbot.UserProfile), nil
}

func (s *Service) getProfileCached(ctx context.Context, key string) (*userbot.UserProfile, bool) {
	raw, ok, err := s.cache.Get(ctx, key)
	if err != nil || !ok {
		return nil, false
	}
	var p userbot.UserProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, false
	}
	return &p, true
}

// Frame renders the recent messages of a chat as a short readable text block,
// newest message last.
func (s *Service) Frame(ctx context.Context, chatID int64, n int) (string, error) {
	msgs, err := s.History(ctx, chatID, n)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, m := range msgs {
		who := m.SenderName
		if who == "" {
			who = fmt.Sprintf("user-%d", m.SenderID)
		}
		if m.Out {
			who = "я"
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			text = "(медиа)"
		}
		fmt.Fprintf(&b, "%s %s: %s\n", time.Unix(m.Date, 0).Format("02.01 15:04"), who, text)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
