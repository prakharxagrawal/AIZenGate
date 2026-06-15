// Package store implements the StateStore interface using Redis.
package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements StateStore using a Redis backend.
type RedisStore struct {
	client *redis.Client
	logger *slog.Logger
}

// NewRedisStore creates a new instance of RedisStore.
func NewRedisStore(client *redis.Client, logger *slog.Logger) *RedisStore {
	return &RedisStore{
		client: client,
		logger: logger,
	}
}

// SaveState persists agent state to Redis.
func (r *RedisStore) SaveState(ctx context.Context, agentID string, state []byte) error {
	err := r.client.Set(ctx, fmt.Sprintf("agent:state:%s", agentID), state, 0).Err()
	if err != nil {
		r.logger.Error("failed to save state", "agentID", agentID, "error", err)
		return fmt.Errorf("redis save state: %w", err)
	}
	return nil
}

// GetState retrieves agent state from Redis.
func (r *RedisStore) GetState(ctx context.Context, agentID string) ([]byte, error) {
	val, err := r.client.Get(ctx, fmt.Sprintf("agent:state:%s", agentID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		r.logger.Error("failed to get state", "agentID", agentID, "error", err)
		return nil, fmt.Errorf("redis get state: %w", err)
	}
	return val, nil
}