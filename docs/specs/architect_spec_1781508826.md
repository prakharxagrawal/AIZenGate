System Architect agent for ZenGate AI.
Build a Redis rate limiter.

        *   Break down high-level tasks into technical specs.
        *   Define interface contracts (Go interfaces).
        *   Create data flow diagrams/descriptions.
        *   Ensure distributed systems best practices.
        *   Output format: Markdown (Overview, Interface Contracts, Data Flow, Design Decisions, Dependencies).

    *   *What is a rate limiter?* A mechanism to control the rate of requests sent or received by a network interface or service.
    *   *Why Redis?* Distributed state. Local memory won't work across multiple pods/instances of ZenGate AI.
    *   *Which Algorithm?*
        *   Fixed Window: Simple, but suffers from bursts at window boundaries.
        *   Sliding Window Log: Accurate, but memory-intensive.
        *   Token Bucket/Leaky Bucket: Smooths output, good for bursts.
        *   Sliding Window Counter: Good balance of accuracy and memory.
    *   *Decision:* I'll implement the **Sliding Window Counter** using Redis Lua scripts to ensure atomicity. This prevents race conditions between "read" and "increment" operations.

    *   *Overview:* Purpose of the component (protecting downstream services, preventing abuse).
    *   *Interface Contracts:* Need a generic `RateLimiter` interface in Go. It should take a key (user ID/IP) and a limit config.
    *   *Data Flow:* Client $\rightarrow$ Middleware $\rightarrow$ RateLimiter $\rightarrow$ Redis.
    *   *Design Decisions:*
        *   Redis Lua scripts for atomicity.
        *   Key naming convention (`ratelimit:{id}:{window}`).
        *   Handling "Allow" vs "Deny" and returning remaining quota.
    *   *Dependencies:* Redis client (go-redis), context.

    *   *Interface:*
        ```go
        type RateLimiter interface {
            Allow(ctx context.Context, key string, limit LimitConfig) (*RateLimitResult, error)
        }
        ```
    *   *Lua Script Logic:*
        1. Get current window.
        2. Get previous window.
        3. Calculate weight: $\text{count} = \text{current\_count} + (\text{prev\_count} \times (1 - \text{percentage\_of\_current\_window\_elapsed}))$.
        4. If $\text{count} < \text{limit}$, increment and return true.

    *   *Check:* Did I include Go interfaces? Yes.
    *   *Check:* Is it distributed? Yes, via Redis.
    *   *Check:* Is the format correct? Yes.# Technical Specification: Redis Distributed Rate Limiter

## Overview
The Redis Rate Limiter is a critical infrastructure component for ZenGate AI designed to protect downstream services from being overwhelmed by too many requests and to prevent API abuse. It implements a **Sliding Window Counter** algorithm to provide a smooth rate-limiting experience, avoiding the "burst" problem associated with Fixed Window counters.

The system is designed to be stateless at the application level, delegating all state management to a Redis cluster to ensure consistency across multiple instances of the ZenGate AI gateway.

## Interface Contracts

The rate limiter is defined by a generic interface to allow for potential strategy swaps (e.g., moving from Sliding Window to Token Bucket) without impacting the middleware layer.

```go
package ratelimit

import (
	"context"
	"time"
)

// LimitConfig defines the threshold for a specific rate limit rule
type LimitConfig struct {
	Limit  int           // Maximum number of requests allowed
	Window time.Duration // The time window for the limit (e.g., 1 minute)
}

// RateLimitResult contains the outcome of the rate limit check
type RateLimitResult struct {
	Allowed   bool      // Whether the request is permitted
	Remaining int       // Number of requests remaining in the current window
	ResetAfter time.Duration // Time until the window resets
}

// RateLimiter is the primary interface for rate limiting operations
type RateLimiter interface {
	// Allow checks if a request identified by the key is permitted
	// under the provided LimitConfig.
	Allow(ctx context.Context, key string, config LimitConfig) (*RateLimitResult, error)
}
```

## Data Flow

### Request Lifecycle
1. **Incoming Request**: A request hits the ZenGate AI Gateway.
2. **Identifier Extraction**: The Middleware extracts a unique identifier (e.g., `API_KEY`, `User_ID`, or `Client_IP`).
3. **Limit Lookup**: The system retrieves the `LimitConfig` associated with that user/tier.
4. **Redis Evaluation**:
    * The `RateLimiter` executes a **Lua Script** on the Redis server.
    * The Lua script calculates the weighted request count across the current and previous time windows.
    * The script atomically increments the counter if the limit is not exceeded.
5. **Response**:
    * **If Allowed**: Request proceeds to the target service.
    * **If Denied**: Gateway returns `429 Too Many Requests` with `X-RateLimit-Reset` headers.

### Data Flow Diagram
`Client` $\rightarrow$ `Middleware` $\rightarrow$ `RateLimiter (Go)` $\rightarrow$ `Lua Script (Redis)` $\rightarrow$ `Result` $\rightarrow$ `Middleware` $\rightarrow$ `Client/Service`

## Design Decisions & Trade-offs

### 1. Algorithm: Sliding Window Counter
We chose the Sliding Window Counter over Fixed Window and Token Bucket.
*   **Why not Fixed Window?** Fixed windows allow a burst of $2 \times Limit$ at the window boundary.
*   **Why not Token Bucket?** While excellent, Token Bucket requires more frequent writes to Redis to maintain the "last updated" timestamp for every key.
*   **Sliding Window Logic**: We calculate the weight of the previous window based on the current timestamp's position in the current window.
    *   *Formula*: $\text{count} = \text{current\_window\_count} + (\text{prev\_window\_count} \times (1 - \text{percentage\_of\_current\_window\_elapsed}))$

### 2. Atomicity via Lua Scripts
To prevent race conditions (Read-Modify-Write), all logic is encapsulated in a Redis Lua script.
*   **Decision**: Use `EVALSHA` for performance.
*   **Benefit**: Ensures that the check-and-increment operation is atomic, preventing users from bypassing the limit via concurrent requests.

### 3. Key Schema
Keys are structured to ensure automatic cleanup via TTLs.
*   **Format**: `rl:{key}:{window_timestamp}`
*   **TTL**: Set to $2 \times \text{Window Duration}$ to ensure the previous window data is available for the sliding calculation but doesn't leak memory.

### 4. Performance Trade-offs
*   **Latency**: Every request adds one Redis round-trip. We mitigate this by using Redis pipelines and ensuring the Redis cluster is deployed in the same VPC/Region as the gateway.
*   **Availability**: If Redis is unavailable, the system is designed to **Fail-Open**. It is better to allow a request through than to block all traffic due to a cache failure.

## Dependencies

| Dependency | Version | Purpose |
| :--- | :--- | :--- |
| `redis` | $\ge 6.2$ | Requires Lua 5.1 support and atomic increments. |
| `go-redis/v9` | Latest | Type-safe Go client for Redis communication. |
| `context` | Standard Lib | For cancellation and timeout propagation. |