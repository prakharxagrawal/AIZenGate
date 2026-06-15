# Architecture Specification: Redis Distributed Rate Limiter

This document details the technical specification, Go interface contracts, and architectural decisions for the distributed rate limiter of the **ZenGate AI** platform.

---

## Overview

The ZenGate AI Rate Limiter is a high-performance, distributed, and resilient rate-limiting service designed to protect downstream AI services, APIs, and gateway endpoints from abuse and brute-force spikes. 

To ensure absolute accuracy across scale-out gateway instances without introducing race conditions, the system uses a **Sliding Window Counter** algorithm implemented via atomic **Redis Lua Scripts**.

### Key Architectural Goals
1. **Low Latency**: Sub-millisecond execution overhead per evaluation.
2. **Atomicity**: No race conditions under highly concurrent client requests (accomplished via Redis single-threaded execution of Lua scripts).
3. **Resilience (Fail-Safe)**: Configurable fail-open or fail-closed behavior if Redis becomes unreachable.
4. **Accuracy**: Dynamic sliding window evaluation instead of a naive fixed-window reset block.

---

## Interface Contracts (Go)

Below are the clean Go interface definitions and data contracts designed for integrations within ZenGate AI services.

```go
package ratelimit

import (
	"context"
	"time"
)

// Limit defines the rate limit threshold over a given window.
type Limit struct {
	// Allowed is the maximum number of requests permitted in the window.
	Allowed int64
	// Window represents the sliding duration (e.g., 1 Minute, 1 Hour).
	Window time.Duration
}

// Result carries the outcome of a rate limit evaluation.
type Result struct {
	// Allowed indicates if the request is permitted to proceed.
	Allowed bool
	// Remaining is the number of allowed requests left in the current window.
	Remaining int64
	// Limit is the original capacity configuration.
	Limit int64
	// ResetAfter indicates how long the client must wait before the limit resets.
	ResetAfter time.Duration
}

// Limiter defines the core contract for rate-limiting operations.
type Limiter interface {
	// Allow evaluates rate-limiting for a unique key against a specific limit.
	Allow(ctx context.Context, key string, limit Limit) (*Result, error)
}

// Config houses configuration settings for the Redis Rate Limiter.
type Config struct {
	// RedisAddress is the hostname/port of the Redis cluster/standalone instance.
	RedisAddress string
	// Password for Redis authentication.
	Password string
	// DB index for Redis connection.
	DB int
	// ConnectionTimeout defines the dial timeout limit.
	ConnectionTimeout time.Duration
	// ReadTimeout defines the command execution timeout.
	ReadTimeout time.Duration
	// FailOpen determines if a Redis failure allows requests (true) or blocks them (false).
	FailOpen bool
}
```

---

## Data Flow

The rate limit lifecycle executes sequentially from the Gateway/Middleware down to the Redis instance:

```
[ Client Request ]
       │
       ▼
┌────────────────────────────────────────┐
│        ZenGate Gateway Middleware      │
│  - Extracts rate-limit identifier     │
│    (e.g., IP, API Key, Token JWT)      │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│         RedisRateLimiter.Allow()       │
│  - Formulates Redis keys & arguments   │
│  - Executes Lua Script (Atomic check)  │
└──────────────────┬─────────────────────┘
                   │
         ┌─────────┴─────────┐
         │ Is Redis Online?  │
         └────┬─────────┬────┘
              │ Yes     │ No
              │         │
              │         └──────────────┐
              ▼                        ▼
┌───────────────────────────┐    ┌─────────────────────────────────┐
│     Execute Lua Script    │    │       Fallback Handling         │
│  - Clean stale logs       │    │ If Config.FailOpen:             │
│  - Count active elements  │    │   -> Result{Allowed: true}      │
│  - Append current tick    │    │ If Not Config.FailOpen:         │
│  - Return remaining/TTL   │    │   -> Return 500/Degraded Error  │
└─────────────┬─────────────┘    └─────────────────┬───────────────┘
              │                                    │
              └─────────────────┬──────────────────┘
                                │
                                ▼
┌────────────────────────────────────────┐
│           Evaluate and Return          │
│ - Append custom headers to Client:     │
│   X-RateLimit-Limit: <limit>           │
│   X-RateLimit-Remaining: <remaining>   │
│   X-RateLimit-Reset: <reset_after>     │
│ - Block with HTTP 429 if !Allowed      │
└────────────────────────────────────────┘
```

### The Atomic Lua Script (Concept)
The Lua script utilizes a **Redis Sorted Set (ZSET)** to store unique timestamps of requests. This ensures an exact sliding window log model:

```lua
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

local clear_before = now - window

-- Remove elements older than the sliding window boundary
redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

-- Count current elements in the window
local current_requests = redis.call('ZCARD', key)

local allowed = false
if current_requests < limit then
    -- Add current request timestamp
    redis.call('ZADD', key, now, now)
    -- Set TTL on key slightly longer than window for clean memory reclamation
    redis.call('PEXPIRE', key, math.ceil(window / 1000))
    allowed = true
    current_requests = current_requests + 1
end

local remaining = limit - current_requests
return {allowed and 1 or 0, remaining}
```

---

## Design Decisions & Trade-offs

### 1. Sliding Window Log (ZSET) vs. Fixed Window Counter
* **Decision**: We chose **Sliding Window Log** via ZSET.
* **Trade-off**: Memory vs. Precision. Fixed Window Counter suffers from "bursting" near boundary edges (allowing up to $2 \times \text{Limit}$ within a short duration around the boundary transition). Sliding Window Log prevents this completely. Although ZSET requires slightly more memory per client tracked, for critical AI service endpoints, boundary accuracy is paramount to guard against systemic model-resource overload.

### 2. Atomic Lua Execution
* **Decision**: All rate-limiting evaluations are delegated to a single `EVALSHA` command executing pre-loaded Lua script on the Redis instance.
* **Trade-off**: This guarantees atomic operations and eliminates network roundtrips between checking the limit and incrementing the count. It places the computational load of cleaning old logs on Redis, which could impact extreme-scale operations; however, for our system scale, Redis handles this with sub-millisecond efficiency.

### 3. Fail-Open vs. Fail-Closed Modes
* **Decision**: Built-in fallback parameters inside `Config.FailOpen`.
* **Trade-off**: If Redis suffers a severe network partition or hardware failure:
  * *Fail-Open*: Allows all incoming API calls. Ensures high availability for customers, but poses danger of crashing downstream LLM inference steps.
  * *Fail-Closed*: Rejects API calls. Protects operational infrastructure, but reduces uptime during minor rate-limiting infrastructure blips. 
  * ZenGate AI delegates this toggle to the systems operator based on endpoint criticality.

---

## Dependencies

* **Go Runtime**: `1.21` or higher.
* **Redis Client Library**: `github.com/redis/go-redis/v9` (Supports cluster routing, context cancellation, and Sentinel auto-failover).
* **Redis Server**: `6.2` or higher (Supports key-expiration tracking inside sorted sets).