System Architect agent for ZenGate AI.
Build a Redis rate limiter.

        *   Detailed technical specifications.
        *   Interface contracts (Go interfaces).
        *   Data flow.
        *   Design decisions/trade-offs.
        *   Dependencies.
Markdown.

    *   *Purpose:* Prevent API abuse, ensure fairness, and protect backend services from spikes.
    *   *Key Requirements:*
        *   Low latency (must not slow down requests).
        *   Distributed (multiple API gateway instances sharing the same state).
        *   Configurable limits (requests per window).
        *   Support for different strategies (Fixed Window, Sliding Window, Token Bucket). *Decision: I'll implement the "Sliding Window Log" or "Sliding Window Counter" via Redis Sorted Sets/Lua scripts for accuracy.*

    *   *Overview:* Describe what the component does and the chosen strategy.
    *   *Interface Contracts:* Need a Go interface for the `RateLimiter` and potentially a `Store` interface.
    *   *Data Flow:* Step-by-step process from request arrival to allow/deny decision.
    *   *Design Decisions:* Why Redis? Why Lua? Why Sliding Window?
    *   *Dependencies:* Redis client, Go language.

    *   *Strategy:* Sliding Window Counter is usually the best balance between accuracy and performance. Use a Lua script to ensure atomicity.
    *   *Redis Key Structure:* `rl:{resource}:{identifier}`.
    *   *Lua Logic:*
        1. Remove old entries outside the window.
        2. Count current entries.
        3. If count < limit, add current timestamp and allow.
        4. Else, deny.

    *   *Overview:* Focus on high-throughput, distributed rate limiting for ZenGate AI.
    *   *Interfaces:*
        *   `RateLimiter` interface with `Allow(ctx, key, limit, window)` method.
        *   `Result` struct to provide metadata (Remaining, Reset).
    *   *Data Flow:* User $\rightarrow$ Middleware $\rightarrow$ RateLimiter $\rightarrow$ Redis $\rightarrow$ Result $\rightarrow$ User.
    *   *Trade-offs:* Lua scripts (CPU on Redis) vs. multiple round trips. Sorted sets (Memory) vs. simple counters (Less accurate).# Technical Specification: Redis Distributed Rate Limiter

## Overview
The Redis Rate Limiter is a high-performance, distributed component designed for ZenGate AI to protect downstream services from cascading failures and API abuse. It implements a **Sliding Window Log** algorithm using Redis Sorted Sets and Lua scripting to ensure atomicity and prevent race conditions in a multi-node environment.

The system provides a mechanism to track request counts per unique identifier (e.g., API Key, IP Address) over a rolling time window, ensuring that users cannot bypass limits by timing requests at the edge of a fixed window.

---

## Interface Contracts

The implementation will be written in Go. We define a clean separation between the business logic of rate limiting and the underlying storage mechanism.

### 1. RateLimiter Interface
This is the primary entry point for the ZenGate AI middleware.

```go
package ratelimit

import (
	"context"
	"time"
)

// Result carries the outcome of the rate limit check
type Result struct {
	Allowed   bool          // Whether the request is permitted
	Remaining int           // Number of requests remaining in the window
	Reset     time.Duration // Time until the window resets/clears
	Limit     int           // The configured limit
}

// RateLimiter defines the behavior for checking request quotas
type RateLimiter interface {
	// Allow checks if a request for a specific key is permitted 
	// given a limit and a rolling window duration.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (*Result, error)
}
```

### 2. Store Interface
To allow for potential migrations (e.g., to Valkey or an in-memory mock for testing), the storage layer is abstracted.

```go
package ratelimit

import "context"

type Store interface {
	// ExecuteLua runs a provided Lua script on the Redis cluster
	ExecuteLua(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
}
```

---

## Data Flow

### Request Lifecycle
1. **Interception**: The ZenGate AI Gateway intercepts an incoming request.
2. **Identification**: The Gateway extracts the `identifier` (e.g., `user_id` from JWT) and determines the `limit` and `window` based on the user's subscription tier.
3. **Evaluation**:
    - The `RateLimiter` calls the `Store.ExecuteLua` method.
    - The Lua script is sent to Redis along with the `key` (e.g., `rl:user_123`).
4. **Redis Execution (Atomic)**:
    - **Step A**: Remove all elements in the Sorted Set older than `now - window`.
    - **Step B**: Count the remaining elements in the set.
    - **Step C**: If `count < limit`, add the current timestamp (microsecond precision) to the set.
    - **Step D**: Return `[allowed_boolean, remaining_count, reset_time]`.
5. **Response**: 
    - If `Allowed == true`, the request proceeds to the backend.
    - If `Allowed == false`, the Gateway returns a `429 Too Many Requests` response with the `Retry-After` header.

### Diagram
`Request` $\rightarrow$ `Middleware` $\rightarrow$ `RateLimiter` $\rightarrow$ `Lua Script` $\rightarrow$ `Redis (Sorted Set)` $\rightarrow$ `Result` $\rightarrow$ `Response`

---

## Design Decisions & Trade-offs

### 1. Sliding Window vs. Fixed Window
- **Decision**: Sliding Window Log.
- **Trade-off**: Fixed windows are computationally cheaper (simple `INCR` and `EXPIRE`) but suffer from "bursting" at window boundaries (a user could double their quota by attacking the end of window N and the start of window N+1). Sliding windows provide strict accuracy at the cost of higher Redis memory usage (storing timestamps).

### 2. Lua Scripting for Atomicity
- **Decision**: Encapsulate the "Check-and-Set" logic in a Lua script.
- **Trade-off**: Without Lua, the application would need to perform multiple round-trips (`ZREMRANGEBYSCORE`, `ZCARD`, `ZADD`), which creates race conditions where two concurrent requests both see `count < limit` and both increment, exceeding the limit. Lua scripts are executed atomically in Redis.

### 3. Data Structure: Sorted Sets (`ZSET`)
- **Decision**: Use Redis `ZSET` where the member and the score are both the timestamp.
- **Trade-off**: Memory consumption grows linearly with the number of requests per window. For extremely high-volume endpoints (millions of req/sec), we would transition to a **Sliding Window Counter** (using multiple buckets) to save memory, though this reduces precision.

### 4. Complexity Analysis
- **Time Complexity**: $O(\log(N) + M)$ where $N$ is the number of elements in the set and $M$ is the number of expired elements removed.
- **Space Complexity**: $O(N)$ per user key.

---

## Dependencies

| Dependency | Version | Purpose |
| :--- | :--- | :--- |
| **Redis** | $\ge 6.2$ | Distributed state store |
| **go-redis/redis** | $\text{Latest}$ | Type-safe Redis client for Go |
| **Context** | Standard Lib | Deadline and cancellation propagation |

## Lua Script Specification
The internal script to be loaded into Redis:
```lua
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

local clear_before = now - window

-- Remove old entries
redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

-- Count current entries
local current_count = redis.call('ZCARD', key)

if current_count < limit then
    redis.call('ZADD', key, now, now)
    redis.call('PEXPIRE', key, window)
    return {1, limit - current_count - 1}
else
    return {0, 0}
end
```