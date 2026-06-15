# Technical Specification: Redis Distributed Rate Limiter

## Overview
The Redis Rate Limiter is a high-performance, distributed component designed to protect ZenGate AI services from abuse, cascading failures, and API quota exhaustion. It implements a **Sliding Window Log** algorithm using Redis Sorted Sets to ensure precision and prevent the "burst" issue common in Fixed Window counters.

The system will act as a middleware layer that intercepts incoming requests, identifies the client (via API key, IP, or User ID), and determines if the request should be permitted based on predefined limits.

## Interface Contracts

The rate limiter is defined by a Go interface to allow for easy mocking during testing and potential migration to different backends (e.g., Memcached or an in-memory store for local dev).

```go
package ratelimit

import (
	"context"
	"time"
)

// Result contains the outcome of the rate limit check
type Result struct {
	Allowed   bool          // True if the request is permitted
	Remaining int           // Number of requests left in the current window
	Reset     time.Duration // Time until the window resets
}

// RateLimiter defines the contract for the rate limiting service
type RateLimiter interface {
	// Allow checks if a request with the given key is permitted.
	// limit: maximum requests allowed per window
	// window: the duration of the sliding window
	Allow(ctx context.Context, key string, limit int, window time.Duration) (*Result, error)
	
	// GetStatus retrieves the current state of a key without incrementing the counter
	GetStatus(ctx context.Context, key string, limit int, window time.Duration) (*Result, error)
}
```

## Data Flow

### 1. Request Lifecycle
1. **Request Arrival**: An incoming HTTP/gRPC request hits the ZenGate Gateway.
2. **Identity Extraction**: The middleware extracts a unique identifier (e.g., `user_id:123` or `ip:1.1.1.1`).
3. **Limiter Call**: The middleware calls `RateLimiter.Allow(ctx, key, limit, window)`.
4. **Redis Execution**: 
    - The Go client sends a **Lua Script** to Redis.
    - Redis executes the script atomically.
5. **Decision**:
    - If `Allowed == true`: Request proceeds to the backend service.
    - If `Allowed == false`: Middleware returns `HTTP 429 Too Many Requests` with a `Retry-After` header.

### 2. Redis Internal Logic (Lua Script)
To avoid race conditions (Check-then-Set), the following logic is executed inside a single Redis Lua script:
1. **Cleanup**: Remove all elements in the Sorted Set (`ZREMRANGEBYSCORE`) with timestamps older than `CurrentTime - Window`.
2. **Count**: Count the remaining elements in the set (`ZCARD`).
3. **Evaluate**: 
    - If `Count < Limit`: Add the current timestamp to the set (`ZADD`) and return `Allowed = true`.
    - If `Count >= Limit`: Return `Allowed = false`.
4. **TTL**: Set an expiration on the key (`EXPIRE`) equal to the window duration to ensure memory is reclaimed for inactive users.

## Design Decisions & Trade-offs

### 1. Algorithm: Sliding Window Log vs. Token Bucket
- **Decision**: Sliding Window Log (via Redis ZSET).
- **Trade-off**: While Token Bucket is more memory-efficient, Sliding Window Log provides absolute precision. It prevents the "edge-of-window" burst where a user could double their quota by sending requests at the very end of one window and the start of the next.
- **Complexity**: Time complexity is $O(\log N)$ for additions and $O(M)$ for removals, where $M$ is the number of expired elements. Given typical API limits (e.g., 100 req/min), this is negligible.

### 2. Atomicity via Lua
- **Decision**: Use Lua scripts instead of multiple Redis commands.
- **Trade-off**: Lua scripts block other Redis operations while running. However, since our script is extremely lightweight (ZREM $\rightarrow$ ZCARD $\rightarrow$ ZADD), the blocking time is minimal compared to the network overhead of multiple round-trips.

### 3. Fail-Open Strategy
- **Decision**: The system will **Fail-Open**.
- **Trade-off**: If the Redis cluster is unreachable, the `Allow` method will log an error and return `Allowed = true`. 
- **Reasoning**: In a production environment, it is generally better to risk a temporary overload of the backend than to cause a total system outage because the rate limiter is down.

### 4. Key Namespacing
- **Decision**: Keys will follow the pattern `rl:{service_name}:{identifier}`.
- **Reasoning**: This allows for granular control (e.g., different limits for `/auth` vs `/data` endpoints) and easier debugging via Redis CLI.

## Dependencies

| Dependency | Version | Purpose |
| :--- | :--- | :--- |
| **Redis** | $\ge 6.2$ | Distributed state store (requires ZSET support) |
| **go-redis** | $\ge v9$ | Type-safe Redis client for Go |
| **Context** | Standard Lib | For timeout and cancellation propagation |

## Summary of Complexity
- **Time Complexity**: $O(\log N)$ per request.
- **Space Complexity**: $O(K \times L)$ where $K$ is the number of active users and $L$ is the limit per window.