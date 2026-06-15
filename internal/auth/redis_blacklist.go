// Package auth provides authentication primitives for the ZenGate AI Gateway.
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Blacklist defines the interface for checking revoked tokens.
type Blacklist interface {
	IsRevoked(ctx context.Context, token string) (bool, error)
	Revoke(ctx context.Context, token string, expiration time.Duration) error
}

// RedisBlacklist implements Blacklist using a Redis store.
type RedisBlacklist struct {
	client *redis.Client
}

// NewRedisBlacklist creates a new Redis-backed blacklist.
func NewRedisBlacklist(client *redis.Client) *RedisBlacklist {
	return &RedisBlacklist{
		client: client,
	}
}

// IsRevoked checks if the token exists in the Redis blacklist.
func (rb *RedisBlacklist) IsRevoked(ctx context.Context, token string) (bool, error) {
	key := fmt.Sprintf("blacklist:%s", token)
	exists, err := rb.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists check failed: %w", err)
	}
	return exists > 0, nil
}

// Revoke adds a token to the blacklist with a specific TTL.
func (rb *RedisBlacklist) Revoke(ctx context.Context, token string, expiration time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", token)
	err := rb.client.Set(ctx, key, "1", expiration).Err()
	if err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}
	return nil
}