# Architectural Specification: Distributed Redis Rate Limiter

This document specifies the system architecture, component contracts, and data flows for the distributed rate-limiting service within the ZenGate AI ecosystem. 

---

## Overview

In a highly distributed, microservices-based architecture, rate limiting is critical for protecting upstream services from denial-of-service (DoS) attacks, brute-forcing, API abuse, and cascading failures. 

This specification defines a **Distributed Rate Limiter** powered by Redis. It supports the **Sliding Window Log** algorithm (implemented via Redis Sorted Sets and executed atomically through Lua scripts) to guarantee precise, real-time rate enforcement across multi-tenant gateways and services, with sub-millisecond overhead.

### Key Objectives
* **Accuracy**: Eliminate the "bursting" issue common in Fixed Window algorithms.
* **Atomicity**: Avoid race conditions (Time-of-Check to Time-of-Use) when checking and decrementing limits under concurrent requests.
* **Low Latency**: Ensure rate-limiting decisions take $< 2\text{ms}$ overhead.
* **Resiliency**: Provide fail-open/fail-closed configuration strategies with fallback in-memory rate limiting if the Redis cluster becomes unavailable.

---

## Interface Contracts

The Go interfaces define clean abstraction barriers, separating the rate-limiting engine from specific transport layers (HTTP/gRPC) and concrete storage engines.

```go
package ratelimit

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRedisUnavailable  = errors.New("rate limiter: redis backend is unreachable")
	ErrInvalidParameters = errors.New("rate limiter: limit or window size must be positive")
)

// RateLimitResult defines the metadata returned on every rate limit evaluation.
type RateLimitResult struct {
	// Allowed indicates whether the request passed the rate limit check.
	Allowed bool

	// Remaining is the number of requests remaining within the current window.
	Remaining int64

	// Limit is the total capacity allowed per window.
	Limit int64

	// ResetAfter indicates the duration until the rate limit fully resets or replenishes.
	ResetAfter time.Duration
}

// RateLimiter is the core interface governing distributed rate-limiting operations.
type RateLimiter interface {
	// Allow evaluates a request against a rate limit defined by a key, capacity (limit),
	// and a rolling time window.
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (*RateLimitResult, error)

	// Reset manually clears the rate limit state for a given key.
	Reset(ctx context.Context, key string) error
}

// Strategy represents the underlying algorithm implementation (e.g., SlidingWindow, TokenBucket).
type Strategy string

const (
	StrategySlidingWindow Strategy = "sliding_window"
	StrategyTokenBucket   Strategy = "token_bucket"
)

// Options allows configuring the RateLimiter runtime behavior.
type Options struct {
	Strategy       Strategy
	FallbackToLocal bool          // If Redis is down, fallback to an in-memory limiter
	LocalCacheTTL  time.Duration // TTL for fallback cache
}

// Option defines a functional configuration option.
type Option func(*Options)
```

---

## Data Flow

### 1. Architectural Sequence Diagram

This diagram maps out how an incoming gateway request is validated against the Redis Rate Limiter utilizing an atomic Lua script execution.

```
Client         API Gateway        RateLimiter Service         Redis Cluster
  │                 │                     │                         │
  │─── Request ────>│                     │                         │
  │                 │─── Allow(Key,...) ─>│                         │
  │                 │                     │─── Execute Lua Script ─>│
  │                 │                     │    (ZREMRANGEBYSCORE,   │
  │                 │                     │     ZCARD, ZADD, EXPIRE)│
  │                 │                     │<─── Script Return ──────│
  │                 │                     │    [Allowed, Remaining] │
  │                 │<── RateLimitResult ─│                         │
  │                 │                     │                         │
  │                 │─── Update Headers ─>│                         │
  │                 │    (X-RateLimit-*)  │                         │
  │<── Response ────│                     │                         │
```

### 2. Step-by-Step Processing Flow

1. **Request Interception**: An HTTP/gRPC request hits the API Gateway. The middleware extracts rate-limiting keys based on the configured policy (e.g., `Client IP`, `JWT claims`, `API Key`, or `Client-ID`).
2. **Evaluation Call**: The middleware invokes `Allow(ctx, key, limit, window)`.
3. **Redis Execution (Lua)**: The RateLimiter engine invokes an atomic Lua script on Redis:
    * It computes the current timestamp window boundary (`now - window`).
    * **`ZREMRANGEBYSCORE`**: Cleans up expired elements older than the sliding window threshold.
    * **`ZCARD`**: Counts active entries remaining in the sliding window.
    * **Evaluation**: If `current_count < limit`, it executes `ZADD` to append the current timestamp as both the score and member, and updates the key's TTL (`EXPIRE`).
4. **Parsing Response**: The script returns an array representing `[Allowed (0 or 1), Remaining]`.
5. **Fallback Handling**: If Redis returns an connection error, the RateLimiter checks if `FallbackToLocal` is enabled. If true, it falls back to a thread-safe thread-local in-memory rate limiter, raising a non-fatal warning metric.
6. **Response Generation**:
    * If **Allowed**: The request proceeds to the downstream handler. Headers are populated:
        * `X-RateLimit-Limit`: `limit`
        * `X-RateLimit-Remaining`: `remaining`
        * `X-RateLimit-Reset`: Duration until oldest element expires.
    * If **Denied**: The middleware halts execution, returning HTTP status `429 Too Many Requests`.

---

## Design Decisions & Trade-offs

### 1. Sliding Window Log (ZSET) vs. Token Bucket
* **Decision**: Implement **Sliding Window Log** via Redis Sorted Sets (`ZSET`) as the primary mechanism for low-to-medium throughput strict keys, with an optional **Token Bucket** algorithm for extremely high throughput scenarios.
* **Trade-off**: The Sliding Window Log is exceptionally precise and completely prevents burst-abuses at window boundaries. However, its memory consumption scales with the number of requests stored in the sorted set ($O(N)$ memory, where $N$ is the number of hits within the sliding window). 
* **Mitigation**: To prevent unbounded memory expansion under high-volume endpoints, the API Gateway applies a ceiling rule to enforce Token Bucket configurations for rules scaling $> 100,000 \text{ req/min}$.

### 2. Multi-Key Lua Execution
* **Decision**: All keys evaluated within a single transaction/Lua execution must hash to the same Redis Cluster node.
* **Trade-off**: Using multi-key scripts in a Redis Cluster can trigger `CROSSSLOT Keys in request don't hash to the same slot` errors.
* **Mitigation**: We restrict our Lua engine to run on a single key. Tenant IDs are encoded into the Redis key format to ensure hashing alignment, using Redis hash tags (e.g., `{tenant_12345}:ratelimit:client_ip`).

### 3. Fail-Open vs. Fail-Closed
* **Decision**: Implement a configurable fallback strategy inside the rate limiter library.
* **Trade-off**: 
    * *Fail-Open*: Keeps the platform operational during Redis split-brain or outages, but risks overloading downstream microservices.
    * *Fail-Closed*: Guarantees safety and downstream defense but sacrifices availability of the system.
* **Implementation**: The library will default to **Fail-Open** with a local, in-memory rate limiter fallback to buffer traffic during micro-outages, exposing Prometheus metrics alerts (`ratelimit_backend_fallback_active`).

---

## Technical Specifications & Redis Lua Script

To maintain atomic evaluation under high concurrency, the system executes the following Redis Lua script logic:

```lua
-- KEYS[1]: The rate limit key (e.g., {user_1}:rate_limit)
-- ARGV[1]: Current microsecond timestamp (Unix epoch micro)
-- ARGV[2]: Window size in microseconds
-- ARGV[3]: Maximum permitted capacity (Limit)
-- ARGV[4]: Key expiration time in seconds (Window size converted to seconds + buffer)

local rate_limit_key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local expire_time = tonumber(ARGV[4])

local clear_before = now - window

-- Remove outdated request records outside the sliding window
redis.call('ZREMRANGEBYSCORE', rate_limit_key, 0, clear_before)

-- Count current requests within the active sliding window
local current_requests = redis.call('ZCARD', rate_limit_key)

local allowed = 0
if current_requests < limit then
    -- Add the unique microsecond timestamp as score and member
    redis.call('ZADD', rate_limit_key, now, now)
    redis.call('EXPIRE', rate_limit_key, expire_time)
    allowed = 1
    current_requests = current_requests + 1
end

local remaining = limit - current_requests
return {allowed, remaining}
```

---

## Dependencies

The system architecture utilizes minimal, highly-performant external dependencies:

| Dependency | Purpose | Target Version |
|:---|:---|:---|
| `github.com/redis/go-redis/v9` | Standard Go Redis client with Cluster/Sentinel and context support. | `v9.4.0` or higher |
| `go.uber.org/atomic` | Thread-safe, lock-free state primitives for fallback operations. | `v1.11.0` or higher |
| `github.com/stretchr/testify` | Unit testing and suite runner assertions. | `v1.8.4` or higher |