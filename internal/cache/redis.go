package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis is a redis-backed Cache implementation.
type Redis struct {
	client *redis.Client
}

var _ Cache = (*Redis)(nil)

// NewRedis creates a Redis cache connected to addr (host:port).
// It verifies the connection with a PING.
func NewRedis(ctx context.Context, addr string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Redis{client: client}, nil
}

// Get implements Cache.
func (r *Redis) Get(ctx context.Context, key string) (string, bool, error) {
	value, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// Set implements Cache.
func (r *Redis) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

// Del implements Cache.
func (r *Redis) Del(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}
