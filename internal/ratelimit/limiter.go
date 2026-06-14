package ratelimit

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter defines the core interface for rate limiting.
type Limiter interface {
	// Allow checks if the clientID is allowed to execute a request under the given limit and window.
	Allow(ctx context.Context, clientID string, limit int, window time.Duration) (bool, error)
}

// --- Token Bucket Limiter (In-Memory Fallback) ---

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

// TokenBucketLimiter implements a thread-safe local in-memory rate limiter.
type TokenBucketLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	capacity float64
	rate     float64 // tokens per second
}

// NewTokenBucketLimiter creates an in-memory rate limiter.
// rate specifies how many tokens are added per second, capacity is the maximum burst size.
func NewTokenBucketLimiter(rate, capacity float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: capacity,
		rate:     rate,
	}
}

// Allow implements Limiter. Local token bucket enforcement.
func (l *TokenBucketLimiter) Allow(ctx context.Context, clientID string, limit int, window time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, exists := l.buckets[clientID]
	now := time.Now()
	if !exists {
		l.buckets[clientID] = &tokenBucket{
			tokens:     l.capacity,
			lastRefill: now,
		}
		bucket = l.buckets[clientID]
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens = bucket.tokens + (elapsed * l.rate)
	if bucket.tokens > l.capacity {
		bucket.tokens = l.capacity
	}
	bucket.lastRefill = now

	// Check if token can be consumed
	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, nil
	}

	return false, nil
}

// --- Redis Sliding Window Limiter (Distributed) ---

// Go embed to load the Lua script at compile time
//go:embed scripts/sliding_window.lua
var slidingWindowLua string

// RedisSlidingWindowLimiter implements distributed sliding window rate limiting.
type RedisSlidingWindowLimiter struct {
	rdb       *redis.Client
	scriptSHA string
}

// NewRedisSlidingWindowLimiter creates a new Redis rate limiter and loads the Lua script.
func NewRedisSlidingWindowLimiter(rdb *redis.Client) (*RedisSlidingWindowLimiter, error) {
	sha, err := rdb.ScriptLoad(context.Background(), slidingWindowLua).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to load rate limit Lua script: %w", err)
	}

	return &RedisSlidingWindowLimiter{
		rdb:       rdb,
		scriptSHA: sha,
	}, nil
}

// Allow implements Limiter. Checks limit by executing atomic sliding window Lua script.
func (l *RedisSlidingWindowLimiter) Allow(ctx context.Context, clientID string, limit int, window time.Duration) (bool, error) {
	key := fmt.Sprintf("zg:rl:%s", clientID)
	now := time.Now().UnixNano() / int64(time.Millisecond) // current timestamp in ms
	windowMs := window.Milliseconds()

	// KEYS[1]: key
	// ARGV[1]: now, ARGV[2]: limit, ARGV[3]: windowMs
	res, err := l.rdb.EvalSha(ctx, l.scriptSHA, []string{key}, now, limit, windowMs).Result()
	if err != nil {
		// If the script was flushed from cache, reload and retry once
		if redis.HasErrorPrefix(err, "NOSCRIPT") {
			var reloadErr error
			l.scriptSHA, reloadErr = l.rdb.ScriptLoad(ctx, slidingWindowLua).Result()
			if reloadErr != nil {
				return false, fmt.Errorf("failed to reload rate limit Lua script: %w", reloadErr)
			}
			res, err = l.rdb.EvalSha(ctx, l.scriptSHA, []string{key}, now, limit, windowMs).Result()
			if err != nil {
				return false, fmt.Errorf("failed to execute rate limit Lua script after reload: %w", err)
			}
		} else {
			return false, fmt.Errorf("failed to execute rate limit Lua script: %w", err)
		}
	}

	allowed, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected script response type: %T", res)
	}

	return allowed == 1, nil
}
