
*   Goal: Build a Redis-based rate limiter.
    *   Role: System Architect.
    *   Output Format: Markdown with specific sections (Overview, Interface Contracts, Data Flow, Design Decisions, Dependencies).

    *   What is a rate limiter? A mechanism to control the rate of requests sent or received by a network interface or service.
    *   Why Redis? Shared state (distributed), high performance, atomic operations (Lua scripts).
    *   Common Algorithms:
        *   *Fixed Window:* Simple, but allows bursts at window edges.
        *   *Sliding Window Log:* Accurate, but memory-intensive.
        *   *Sliding Window Counter:* Good balance of accuracy and memory.
        *   *Token Bucket/Leaky Bucket:* Smooths out traffic, allows bursts.
    *   *Decision:* For a general-purpose Redis rate limiter, the **Generic Cell Rate Algorithm (GCRA)** or a **Sliding Window Counter** using Lua is the industry standard. I'll specify the Sliding Window Counter approach for clarity and flexibility.

    *   *Overview:* High-level goal—protect services from overload and prevent abuse using a distributed Redis store.

    *   *Interface Contracts (Go):*
        *   Need a `RateLimiter` interface.
        *   Methods: `Allow(ctx, key, limit, window)`, `GetRemaining(ctx, key)`.
        *   Need a `RedisClient` interface to decouple the specific Redis library (e.g., go-redis).

    *   *Data Flow:*
        1.  Request enters.
        2.  Middleware extracts identity key (IP, UserID).
        3.  Call to `RateLimiter.Allow`.
        4.  Execute Lua script in Redis.
        5.  Redis returns "allowed" or "blocked" + current count/reset time.
        6.  Application returns 200 OK or 429 Too Many Requests.

    *   *Design Decisions & Trade-offs:*
        *   *Lua Scripts:* Why? Atomicity. Prevents race conditions between `GET` and `INCR`.
        *   *Sliding Window vs. Fixed Window:* Sliding window avoids the "double limit" problem at boundary edges.
        *   *Key Strategy:* Prefixing keys (`ratelimit:user_123`) for organization.
        *   *Failure Mode:* Fail-open vs. Fail-closed. (Usually fail-open for UX, but configurable).

    *   *Dependencies:*
        *   Redis (v6.0+ for certain commands).
        *   Redis Go Client (`github.com/redis/go-redis`).

    *   Ensure the Markdown is clean.
    *   Make sure the Go interfaces are idiomatic.
    *   Double-check that the data flow is logical.# Technical Specification: Redis Distributed Rate Limiter

## Overview
The Redis Rate Limiter is a distributed middleware component designed to protect ZenGate AI services from abuse, prevent cascading failures, and enforce API quotas. By utilizing Redis as a centralized state store, the limiter ensures that rate limits are applied consistently across multiple horizontally scaled application instances.

The system will implement a **Sliding Window Counter** algorithm via Lua scripting to ensure atomicity and prevent the "edge-of-window" burst problem common in Fixed Window implementations.

## Interface Contracts

### Rate Limiter Interface
This is the primary entry point for the application logic.

```go
package ratelimiter

import (
	"context"
	"time"
)

// Result contains the outcome of the rate limit check
type Result struct {
	Allowed   bool          // True if the request is permitted
	Remaining int64         // Number of requests remaining in the current window
	Reset     time.Duration // Time until the window resets
}

// RateLimiter defines the contract for checking request quotas
type RateLimiter interface {
	// Allow checks if a request for a specific key is permitted
	// key: Unique identifier (e.g., IP address, API Key, User ID)
	// limit: Maximum number of requests allowed in the window
	// window: The time duration of the sliding window
	Allow(ctx context.Context, key string, limit int, window time.Duration) (*Result, error)
}
```

### Redis Store Interface
To ensure the system is testable, the Redis dependency is abstracted.

```go
package ratelimiter

import "context"

// Store defines the low-level Redis operations required by the limiter
type Store interface {
	// Eval executes a Lua script on the Redis server
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
}
```

## Data Flow

1.  **Request Entry**: A request hits the API Gateway or Service Middleware.
2.  **Identity Extraction**: The middleware extracts a unique identifier (e.g., `Authorization` header or `X-Forwarded-For` IP).
3.  **Limit Lookup**: The middleware calls `RateLimiter.Allow(ctx, "user_123", 100, 1 * time.Minute)`.
4.  **Atomic Execution (Redis Lua)**:
    *   The `RateLimiter` sends a Lua script to Redis.
    *   **Redis Step A**: Remove all entries in the sorted set (`ZREMRANGEBYSCORE`) older than `current_time - window`.
    *   **Redis Step B**: Count remaining entries (`ZCARD`).
    *   **Redis Step C**: If `count < limit`, add the current timestamp to the set (`ZADD`).
    *   **Redis Step D**: Set an expiration on the key to ensure cleanup of inactive users (`PEXPIRE`).
5.  **Response**: Redis returns the current count and whether the request was allowed.
6.  **Enforcement**: 
    *   If `Allowed == true`: Request proceeds to the handler.
    *   If `Allowed == false`: Return `HTTP 429 Too Many Requests` with `Retry-After` header.

## Design Decisions & Trade-offs

### 1. Algorithm: Sliding Window Log (via Sorted Sets)
*   **Decision**: Use Redis Sorted Sets (`ZSET`) where the score and value are both the timestamp.
*   **Trade-off**: This is more memory-intensive than a Fixed Window (which only needs a counter). However, it prevents the "burst" issue where a user can double their quota by sending requests at the very end of one window and the start of the next.
*   **Complexity**: Time complexity is $O(\log N + M)$ where $N$ is the number of elements in the set and $M$ is the number of expired elements removed.

### 2. Atomicity: Lua Scripting
*   **Decision**: All logic (Clean $\rightarrow$ Count $\rightarrow$ Add) is wrapped in a single Lua script.
*   **Trade-off**: Lua scripts block other Redis commands during execution. However, because the operations are lightweight and specific to a single key, the impact is negligible compared to the risk of race conditions in a multi-step `GET/SET` flow.

### 3. Failure Mode: Fail-Open
*   **Decision**: If Redis is unavailable or returns an error, the rate limiter will log the error and **Allow** the request.
*   **Trade-off**: This prioritizes availability over strict limit enforcement. In a critical security scenario, this could be toggled to "Fail-Closed," but for most AI services, UX is prioritized.

### 4. Key Namespacing
*   **Decision**: Keys will be formatted as `rl:{window_seconds}:{identifier}`.
*   **Trade-off**: This prevents collisions between different limit tiers (e.g., a per-second limit and a per-hour limit for the same user).

## Dependencies

| Dependency | Version | Purpose |
| :--- | :--- | :--- |
| **Redis** | $\ge$ 6.0 | Centralized state store and Lua engine. |
| **go-redis** | $\text{v9.x}$ | Type-safe Go client for Redis communication. |
| **Context** | Stdlib | For cancellation and timeout propagation. |