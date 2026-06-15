// Package brain implements StateStore using Redis.
package brain

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
)

// RedisStateStore implements the StateStore interface.
type RedisStateStore struct {
	client *redis.Client
}

func NewRedisStateStore(client *redis.Client) *RedisStateStore {
	return &RedisStateStore{client: client}
}

func (r *RedisStateStore) SaveState(ctx context.Context, agentID string, state []byte) error {
	err := r.client.Set(ctx, fmt.Sprintf("agent:%s", agentID), state, 0).Err()
	if err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}
	return nil
}

func (r *RedisStateStore) GetState(ctx context.Context, agentID string) ([]byte, error) {
	val, err := r.client.Get(ctx, fmt.Sprintf("agent:%s", agentID)).Bytes()
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}
	return val, nil
}