Build a Redis rate limiter.
System Architect for ZenGate AI.
Detailed technical specifications, interface contracts, data flow, design decisions, and dependencies.

    *   What does a rate limiter do? It controls the rate of requests a user/client can make to a service.
    *   What are the common algorithms? Fixed Window, Sliding Window Log, Sliding Window Counter, Token Bucket, Leaky Bucket.
    *   *Selection:* For a distributed system using Redis, the **Sliding Window Counter** is generally the best balance between precision and performance. It avoids the "burst" problem of Fixed Window and is more memory-efficient than Sliding Window Log.

    *   *Overview:* High-level goal, target behavior.
    *   *Interface Contracts:* Go-style interfaces.
    *   *Data Flow:* How a request moves through the system.
    *   *Design Decisions:* Why Redis? Why this algorithm?
    *   *Dependencies:* Redis, specific Go libraries.

    *   *Input:* Client ID (IP or API Key), Limit (max requests), Window (timeframe).
    *   *Output:* Allowed/Denied, Remaining, Reset time.
    *   *Redis implementation:* Use a Sorted Set (`ZSET`).
        *   Key: `rate_limit:{clientID}`.
        *   Score: Timestamp.
        *   Value: Unique identifier (UUID or timestamp).
        *   Process:
            1. Remove old entries (outside the current window).
            2. Count existing entries.
            3. If count < limit, add current entry.
            4. Set expiry on the key to clean up idle users.

    *   *Interface:* Needs a `Allow(ctx, key, limit, window)` method.
    *   *Data Flow:* Request $\rightarrow$ RateLimiter $\rightarrow$ Redis Lua Script $\rightarrow$ Decision $\rightarrow$ Response.
    *   *Trade-offs:* Lua scripts ensure atomicity. ZSETs use more memory than simple counters but provide precision.

    *   *Correction:* If I use ZSETs for every single request, memory might spike for high-traffic sites.
    *   *Optimization:* Use a Lua script to minimize round-trips.
    *   *Decision:* Let's stick to the Sliding Window via ZSET for the "Gold Standard" implementation, but mention Fixed Window as an alternative for ultra-high scale.# Technical Specification: Distributed Redis Rate Limiter

## Overview
The Redis Rate Limiter is a middleware component designed to protect ZenGate AI services from abuse, brute-force attacks, and cascading failures by limiting the number of requests a client can make within a specific time window. 

To ensure precision and prevent "bursting" at window boundaries (a common flaw in Fixed Window algorithms), this implementation utilizes a **Sliding Window Log** implemented via Redis Sorted Sets (`ZSET`).

### Core Objectives
- **Distributed State:** Ensure rate limits are synchronized across multiple API gateway instances.
- **Atomicity:** Use Lua scripting to ensure that the "check-and-set" operation is atomic.
- **Precision:** Implement a sliding window to provide a smooth request rate.
- **Low Latency:** Minimize round-trips between the application and Redis.

---

## Interface Contracts

The rate limiter will be implemented as a Go interface to allow for easy mocking during tests or future migration to different backends (e.g., Memcached).

```go
package ratelimit

import (
	"context"
	"time"
)

// Result contains the outcome of the rate limit check
type Result struct {
	Allowed   bool          // Whether the request is permitted
	Remaining int           // Number of requests left in the current window
	Reset     time.Duration // Time until the window resets fully
	Limit     int           // The maximum configured limit
}

// RateLimiter defines the contract for the rate limiting service
type RateLimiter interface {
	// Allow checks if a request from the given key should be permitted.
	// key: Unique identifier (e.g., IP address or API Key)
	// limit: Max number of requests allowed in the window
	// window: The duration of the sliding window
	Allow(ctx context.Context, key string, limit int, window time.Duration) (*Result, error)
}

// RedisRateLimiter is the concrete implementation of the RateLimiter interface
type RedisRateLimiter struct {
	client  RedisClient // Interface wrapping redis.Client (go-redis)
	luaScript string    // Pre-loaded Lua script for atomicity
}
```

---

## Data Flow

### Request Lifecycle
1. **Request Arrival:** A request hits the ZenGate AI Gateway.
2. **Key Extraction:** The gateway extracts the identity key (e.g., `Authorization` header or `X-Forwarded-For` IP).
3. **Limiter Call:** The gateway calls `RateLimiter.Allow(ctx, key, 100, 1*time.Minute)`.
4. **Redis Lua Execution:**
   - **Step A:** Delete all elements in the ZSET with scores older than `CurrentTime - Window`.
   - **Step B:** Count the remaining elements in the ZSET.
   - **Step C:** If `Count < Limit`, add the current timestamp to the ZSET.
   - **Step D:** Set a TTL on the ZSET key equal to the window size to ensure memory cleanup.
5. **Decision Return:** Redis returns the current count and the boolean "Allowed" status.
6. **Enforcement:** If `Allowed == false`, the gateway returns a `429 Too Many Requests` HTTP response.

### Sequence Diagram
`Client` $\rightarrow$ `Gateway` $\rightarrow$ `RateLimiter` $\rightarrow$ `Redis (Lua Script)` $\rightarrow$ `Gateway` $\rightarrow$ `Client`

---

## Design Decisions & Trade-offs

### 1. Algorithm: Sliding Window Log vs. Fixed Window
- **Decision:** Sliding Window Log (via `ZSET`).
- **Trade-off:** 
    - *Fixed Window* is faster and uses less memory (single `INCR` and `EXPIRE`), but it allows 2x the limit if requests cluster around the window boundary.
    - *Sliding Window* is more memory-intensive (storing every timestamp), but provides a strict, fair limit. Given ZenGate's need for precision in AI credit consumption, memory is traded for accuracy.

### 2. Atomicity via Lua Scripting
- **Decision:** Implement the logic inside a Redis Lua script.
- **Reasoning:** To prevent race conditions between the `ZCARD` (count) and `ZADD` (add) operations. By running the logic on the Redis server, we eliminate multiple network round-trips and ensure that the check-and-update operation is atomic.

### 3. Key Namespace
- **Decision:** Keys will follow the pattern `ratelimit:{version}:{identifier}`.
- **Reasoning:** Allows for easy flushing of specific versions of limits without affecting others and prevents key collisions with other Redis-backed services.

### 4. Complexity Analysis
- **Time Complexity:** $O(\log(N) + M)$ where $N$ is the number of elements in the ZSET and $M$ is the number of expired elements removed.
- **Space Complexity:** $O(N)$ per user. For high-traffic users, this can grow; however, the `ZREMRANGEBYSCORE` call keeps the set size capped at the `Limit` value.

---

## Dependencies

### Infrastructure
- **Redis 6.2+**: Required for optimized ZSET operations and Lua scripting.
- **Redis Cluster/Sentinel**: Recommended for high availability to prevent the rate limiter from becoming a single point of failure.

### Software Libraries
- **go-redis/v9**: The primary Redis client for Go.
- **context**: For handling request cancellation and timeouts.

### Configuration Parameters
| Parameter | Default | Description |
| :--- | :--- | :--- |
| `REDIS_TIMEOUT` | `50ms` | Strict timeout to ensure rate limiting doesn't bottleneck the API. |
| `LUA_SCRIPT_LOADED` | `true` | Use `SCRIPT LOAD` on startup to refer to scripts by SHA1 hash. |