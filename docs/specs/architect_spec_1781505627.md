
*   Goal: Build a Redis rate limiter.
*   Persona: System Architect for ZenGate AI.
*   Responsibilities: Detailed technical specs, interface contracts, data flow, architecture decisions, dependencies.
*   Output Format: Markdown (Overview, Interface Contracts, Data Flow, Design Decisions, Dependencies).

    *   *Why Redis?* Distributed state, fast, atomic operations (Lua scripts), TTL for automatic expiration.
    *   *What algorithm?*
        *   Fixed Window: Simplest, but has "edge" bursts.
        *   Sliding Window Log: Accurate, but memory-intensive.
        *   Sliding Window Counter: Good balance of accuracy and memory.
        *   Token Bucket/Leaky Bucket: Smooths out requests.
    *   *Selection:* For a general-purpose "ZenGate AI" rate limiter, a **Sliding Window Counter** (implemented via Redis Sorted Sets or Lua scripts) is usually the gold standard for accuracy without excessive memory usage. However, for extreme performance, a **Fixed Window with Lua** is common. Let's go with a **Sliding Window using Redis Sorted Sets (ZSET)** because it provides precise rate limiting and handles bursts better than fixed windows.

    *   Need a generic interface so we can swap the backend (e.g., to an in-memory limiter for testing).
    *   Method: `Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)`.
    *   Wait, maybe a `RateLimitResult` struct to provide metadata (Remaining, ResetTime).

    *   Request $\rightarrow$ Middleware $\rightarrow$ RateLimiter $\rightarrow$ Redis (ZADD, ZREMRANGEBYSCORE, ZCARD, EXPIRE).

    *   *Atomic Operations:* Use Lua scripts to avoid race conditions between checking the count and incrementing.
    *   *Key Strategy:* `ratelimit:{identifier}:{window_id}`.
    *   *Complexity:* O(log N) for ZSET operations.

    *   Overview: High-level purpose.
    *   Interface: Go code.
    *   Data Flow: Steps.
    *   Decisions: Why ZSET? Why Lua?
    *   Dependencies: `go-redis`.# Technical Specification: Redis Distributed Rate Limiter

## Overview
The Redis Rate Limiter is a distributed middleware component designed to protect ZenGate AI services from abuse and ensure fair resource allocation. It implements a **Sliding Window Log** algorithm using Redis Sorted Sets (ZSETs) to provide precise rate limiting without the "edge burst" problem associated with Fixed Window algorithms.

The system will allow the definition of limits per user, per API key, or per IP address across a distributed cluster of application servers.

---

## Interface Contracts

To ensure the rate limiter can be swapped or mocked for testing, we define a generic `RateLimiter` interface.

```go
package limiter

import (
	"context"
	"time"
)

// RateLimitResult carries the status of the rate limit check
type RateLimitResult struct {
	Allowed   bool          // True if the request is allowed
	Remaining int           // Number of requests remaining in the current window
	Reset     time.Duration // Time until the window resets/clears
	Limit     int           // The configured limit for this window
}

// RateLimiter defines the contract for distributed rate limiting
type RateLimiter interface {
	// Allow checks if a request for a specific key is permitted
	// key: The identifier (e.g., "user_123", "ip_1.1.1.1")
	// limit: Maximum requests allowed per window
	// window: The duration of the sliding window
	Allow(ctx context.Context, key string, limit int, window time.Duration) (*RateLimitResult, error)
}

// RedisLimiter is the concrete implementation using Redis
type RedisLimiter struct {
	client *redis.Client
	// luaScript is the pre-loaded SHA for the atomic sliding window logic
	luaScriptSHA string 
}
```

---

## Data Flow

### Sequence of Operations
1. **Request Arrival**: The API Gateway/Middleware extracts the identifier (e.g., `apiKey`).
2. **Key Generation**: A Redis key is constructed: `rl:{identifier}`.
3. **Atomic Execution (Lua)**: The application sends a Lua script to Redis to ensure atomicity:
    - **Step A**: Remove all elements in the ZSET with a score older than `currentTime - window`.
    - **Step B**: Count the remaining elements in the ZSET (`ZCARD`).
    - **Step C**: If `count < limit`, add the current timestamp as a new element (`ZADD`).
    - **Step D**: Set a TTL on the key to ensure cleanup of inactive users.
4. **Response**: Redis returns the current count and the decision.
5. **Middleware Action**: 
    - If `Allowed == true`: Request proceeds to the handler.
    - If `Allowed == false`: Return `HTTP 429 Too Many Requests` with `X-RateLimit` headers.

### Data Flow Diagram
`Request` $\rightarrow$ `Middleware` $\rightarrow$ `RedisLimiter.Allow()` $\rightarrow$ `Redis (Lua Script)` $\rightarrow$ `Boolean Result` $\rightarrow$ `HTTP Response`

---

## Design Decisions & Trade-offs

### 1. Algorithm: Sliding Window Log vs. Token Bucket
- **Decision**: Sliding Window Log (via ZSET).
- **Reasoning**: Token Bucket is more memory efficient but harder to synchronize perfectly across distributed nodes without frequent updates. Sliding Window Log provides 100% accuracy for the defined window, ensuring a user cannot double their quota by bursting at the end of one window and the start of another.
- **Trade-off**: Memory usage is higher because every request timestamp is stored until it expires. For extreme scale, we would migrate to a *Sliding Window Counter* (approximate).

### 2. Atomicity: Lua Scripting
- **Decision**: Use Redis Lua scripts.
- **Reasoning**: A standard "Get-then-Set" flow in Go would create a race condition where multiple concurrent requests see the same count and all proceed, exceeding the limit. Lua scripts execute atomically in Redis.

### 3. Time Synchronization
- **Decision**: Use `TIME` command from Redis rather than the application server's local clock.
- **Reasoning**: In a distributed environment, clock drift between application nodes can lead to inconsistent rate limiting. Using the Redis server as the single source of truth for time ensures consistency.

### 4. Complexity Analysis
- **Time Complexity**: $O(\log N)$ for `ZREMRANGEBYSCORE` and `ZADD`, where $N$ is the number of requests in the window.
- **Space Complexity**: $O(N)$ per active key.

---

## Dependencies

| Dependency | Version | Purpose |
| :--- | :--- | :--- |
| `go-redis/v9` | Latest | Redis client for Go |
| `Redis` | $\ge$ 6.2 | Backend store (supporting Lua and ZSETs) |
| `Context` | Standard Lib | Deadline and cancellation propagation |

## Lua Implementation Logic (Pseudocode)
The following logic will be embedded in the `RedisLimiter` implementation:

```lua
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- 1. Remove old entries
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

-- 2. Count current entries
local current_count = redis.call('ZCARD', key)

if current_count < limit then
    -- 3. Add current request
    redis.call('ZADD', key, now, now)
    redis.call('EXPIRE', key, window)
    return {1, current_count + 1} -- Allowed
else
    return {0, current_count} -- Rejected
end
```