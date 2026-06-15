// Package ratelimiter provides distributed rate limiting using Redis.
package ratelimiter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lua script for atomic fixed-window rate limiting
const rateLimitScript = `
local current = redis.call("INCR", KEYS[1])
if tonumber(current) == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
if tonumber(current) > tonumber(ARGV[2]) then
    return 0
end
return 1
`

// RedisLimiter implements the Limiter interface.
type RedisLimiter struct {
	client *redis.Client
	logger *slog.Logger
}

func NewRedisLimiter(client *redis.Client, logger *slog.Logger) *RedisLimiter {
	return &RedisLimiter{client: client, logger: logger}
}

// Allow checks if the request is permitted. Fails open on Redis errors.
func (r *RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	allowed, err := r.client.Eval(ctx, rateLimitScript, []string{key}, int(window.Milliseconds()), limit).Int()
	if err != nil {
		r.logger.Error("rate limiter redis error, failing open", "error", err, "key", key)
		return true, fmt.Errorf("redis eval failed: %w", err)
	}

	return allowed == 1, nil
}