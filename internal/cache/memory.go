package cache

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	value     string
	expiresAt time.Time
}

// Memory is an in-process Cache implementation.
type Memory struct {
	mux   sync.Mutex
	store map[string]memEntry
	now   func() time.Time
}

var _ Cache = (*Memory)(nil)

// NewMemory creates an empty in-memory cache.
func NewMemory() *Memory {
	return &Memory{
		store: make(map[string]memEntry),
		now:   time.Now,
	}
}

// Get implements Cache.
func (m *Memory) Get(ctx context.Context, key string) (string, bool, error) {
	m.mux.Lock()
	defer m.mux.Unlock()

	e, ok := m.store[key]
	if !ok {
		return "", false, nil
	}
	if !e.expiresAt.IsZero() && m.now().After(e.expiresAt) {
		delete(m.store, key)
		return "", false, nil
	}
	return e.value, true, nil
}

// Set implements Cache.
func (m *Memory) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	exp := time.Time{}
	if ttl > 0 {
		exp = m.now().Add(ttl)
	}
	m.store[key] = memEntry{value: value, expiresAt: exp}
	return nil
}

// Del implements Cache.
func (m *Memory) Del(ctx context.Context, key string) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	delete(m.store, key)
	return nil
}
