# Architecture Specification: Redis Distributed Rate Limiter

## Overview

ZenGate AI requires a highly performant, distributed, and accurate rate limiting component capable of enforcing throughput limits across multi-tenant API gateways and microservices. 

This specification defines a **Sliding Window Log** rate limiting engine using **Redis** and **Lua scripting**. This combination ensures atomicity, avoids race conditions under high concurrency, minimizes network round-trips, and maintains precise rate-limiting state across distributed gateway instances.

```
                  +------------------------+
                  |  ZenGate API Gateway   |
                  +-----------+------------+
                              |
                     1. Limit(ctx, key)
                              v
                  +-----------+------------+
                  |    Go Rate Limiter     |
                  +-----------+------------+
                              |
                 2. EVALSHA (Lua Script)
                              v
                  +-----------+------------+
                  |      Redis Cluster     |
                  +------------------------+
```

---

## Interface Contracts (Go)

The rate limiter interface is designed to be pluggable, allowing ZenGate services to swap backends (e.g., in-memory for local testing, Redis for production).

```go
package ratelimit

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrBackendConnection = errors.New("rate limiter backend connection failure")
)

// LimitConfig defines the window and allowed capacity for a specific rate limit.
type LimitConfig struct {
	// Unique identifier for the rate limit policy (e.g., "tier_gold", "sms_verification").
	PolicyID string
	// The sliding window duration (e.g., 1 * time.Minute).
	Window time.Duration
	// Maximum number of requests allowed within the window.
	Limit int64
}

// Result represents the state of a rate limit check.
type Result struct {
	// Allowed indicates whether the request should proceed.
	Allowed bool
	// Limit is the total configuration capacity.
	Limit int64
	// Remaining is the number of tokens/slots left in the current window.
	Remaining int64
	// ResetAfter indicates when the rate limit window will fully reset/refresh.
	ResetAfter time.Duration
}

// RateLimiter defines the core contract for rate-limiting operations.
type RateLimiter interface {
	// Limit evaluates the request against the configured limit for the given key.
	// Returns a Result containing metadata, or an error if the backend is unreachable.
	Limit(ctx context.Context, key string, config LimitConfig) (Result, error)

	// Reset manually clears the rate limit state for a key (useful for admin / manual overrides).
	Reset(ctx context.Context, key string) error
}
```

---

## Data Flow

### 1. Request Processing Lifecycle

The diagram below details the step-by-step transaction execution when an edge request hits the ZenGate API gateway.

```
  Client             Gateway          Go RateLimiter        Redis Cluster
    |                   |                   |                     |
    |--- 1. API Req --->|                   |                     |
    |                   |--- 2. Limit() --->|                     |
    |                   |                   |--- 3. EVALSHA ----->| (Runs Lua Script)
    |                   |                   |<-- 4. Return Arr ---| (Atomic Exec)
    |                   |<-- 5. Result -----|                     |
    |                   | (Allowed: true)   |                     |
    |<-- 6. Forward ----|                   |                     |
    |    (With Headers) |                   |                     |
```

### 2. Step-by-Step Sequence

1. **Request Interception**: The Gateway interceptor extracts the identifier (e.g., API key, User ID, or client IP) and constructs the rate limit key: `rate_limit:{tenant_id}:{policy_id}:{identifier}`.
2. **Evaluation**: The Gateway invokes `RateLimiter.Limit(ctx, key, config)`.
3. **Lua Script Execution**: The `RateLimiter` issues an `EVALSHA` command to Redis with the rate-limiting parameters:
   - `KEYS[1]`: Rate Limit Key (`rate_limit:...`)
   - `ARGV[1]`: Current epoch timestamp (microseconds)
   - `ARGV[2]`: Sliding window duration (microseconds)
   - `ARGV[3]`: Maximum limit capacity
4. **Redis Evaluation**: Redis runs the atomic Lua script:
   - Prunes expired timestamps older than `now - window_duration` from the Sorted Set (`ZSET`).
   - Counts the remaining elements in the set.
   - If count is below limit, adds the current timestamp to the set and sets key TTL.
   - Computes remaining capacity and reset time.
5. **Gateway Response**: The Rate Limiter returns a `Result`. The Gateway injects RFC-compliant standard headers:
   - `X-RateLimit-Limit`: Maximum allowance configured.
   - `X-RateLimit-Remaining`: Remaining capacity.
   - `X-RateLimit-Reset`: Unix timestamp when the limit fully resets.

---

## Lua Script Implementation

To guarantee strict accuracy under highly concurrent environments, the **Sliding Window Log** algorithm is executed inside a Redis Lua script.

```lua
-- KEYS[1]: Rate limit key (ZSET)
-- ARGV[1]: Current timestamp (microseconds)
-- ARGV[2]: Window size (microseconds)
-- ARGV[3]: Maximum capacity (limit)

local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

local clear_before = now - window

-- 1. Remove timestamps older than the sliding window threshold
redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

-- 2. Count current elements in the sliding window
local current_requests = redis.call('ZCARD', key)

local allowed = 0
local remaining = limit - current_requests

if current_requests < limit then
    -- 3. If below limit, register the current request timestamp
    redis.call('ZADD', key, now, now)
    allowed = 1
    remaining = remaining - 1
end

-- 4. Set dynamic TTL to clean up idle keys (window size in seconds)
local ttl_seconds = math.ceil(window / 1000000)
redis.call('EXPIRE', key, ttl_seconds)

return {allowed, remaining}
```

---

## Design Decisions & Trade-offs

### 1. Sliding Window Log vs. Token Bucket / Fixed Window
* **Decision**: Sliding Window Log (implemented via Redis ZSET).
* **Trade-off**: Memory overhead is higher compared to Fixed Window (which uses simple strings and increments) because every request timestamp is saved as a member in a ZSET.
* **Justification**: Eliminates "bursts" at window boundaries, which is a major drawback of Fixed Window algorithms. Highly accurate and ideal for commercial API monetization models where rate precision is critical.

### 2. Evaluation via Lua Scripts (`EVALSHA`)
* **Decision**: We utilize pre-loaded Lua scripts via SHA hashes (`EVALSHA`).
* **Justification**:
  * **Atomicity**: Redis executes Lua scripts sequentially. No parallel requests can execute middle operations, avoiding race conditions.
  * **Network Efficiency**: Reduces multi-roundtrip network operations (Read-Compute-Write) to a single Redis transaction.
  * **Performance**: Pre-compiled SHA execution avoids the overhead of parsing Lua source code on every request.

### 3. Fail-Open vs. Fail-Closed on Redis Outage
* **Decision**: Configure at the Gateway level, defaulting to **Fail-Open**.
* **Trade-off**: If Redis is entirely down, failing open lets users bypass limits (potentially exposing downstream services to load), whereas failing closed denies legitimate traffic.
* **Justification**: Availability takes precedence over rate-limit strictness for public edge gateways. The Go implementation should emit logs and metrics on system degradation.

---

## Dependencies

* **Go Runtime**: `v1.21` or higher.
* **Redis Client Library**: `github.com/redis/go-redis/v9` (Supports connection pooling, Redis Sentinel, and Cluster topologies).
* **Redis**: `v7.0` or higher (required for optimized `ZREMRANGEBYSCORE` and native script execution features).