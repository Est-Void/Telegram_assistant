// Package cache provides a small key/value cache abstraction used to memoize
// userbot results (chat history, user profiles).
package cache

import (
	"context"
	"time"
)

// Cache is a simple string value cache with TTL.
type Cache interface {
	// Get returns the value for key, found=true if present and not expired.
	Get(ctx context.Context, key string) (value string, found bool, err error)
	// Set stores value for key with the given TTL.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	// Del removes the key.
	Del(ctx context.Context, key string) error
}
