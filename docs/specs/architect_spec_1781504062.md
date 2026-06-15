# Architecture Specification: Distributed Redis Rate Limiter

## Overview
The ZenGate AI Rate Limiter is a distributed service designed to enforce request quotas across multiple microservices. It utilizes **Redis** as the centralized state store and the **Generic Cell Rate Algorithm (GCRA)** or **Fixed Window** (based on latency requirements) to ensure atomic updates via Lua scripting.

## Interface Contracts (Go)

The interface is designed to be plug-and-play for any RPC middleware (gRPC/HTTP).

```go
package ratelimiter

import "context"

// Limiter defines the contract for rate enforcement.
type Limiter interface {
	// Allow checks if the request is permitted.
	// key: the unique identifier (e.g., API Key, IP, User ID).
	// limit: max requests allowed in window.
	// window: duration of the reset cycle.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// StatsReporter defines an interface for tracking usage metrics.
type StatsReporter interface {
	ReportUsage(ctx context.Context, key string, remaining int)
}
```

## Data Flow

1.  **Request Entry**: Client request hits the API Gateway.
2.  **Key Extraction**: Gateway extracts an identifier (e.g., `x-api-key`).
3.  **Lua Script Execution**: The `Limiter` service executes an atomic `EVAL` script in Redis:
    *   **GET** current counter for the key.
    *   **IF** counter < limit: **INCR** and **EXPIRE** (if new).
    *   **ELSE**: Return `false`.
4.  **Decision**: The service returns a 429 status if `Allow` returns `false`, or proceeds if `true`.

### Lua Script (Atomic Logic)
```lua
local current = redis.call("INCR", KEYS[1])
if tonumber(current) == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
if tonumber(current) > tonumber(ARGV[2]) then
    return 0
end
return 1
```

## Design Decisions & Trade-offs

| Decision | Trade-off |
| :--- | :--- |
| **Redis Lua Scripts** | **Pro**: Atomic operations avoid race conditions. **Con**: Complexity in script debugging. |
| **Fixed Window** | **Pro**: Simple, low memory overhead. **Con**: Allows double bursts at window edges. |
| **Pipelining** | We chose against pipelining in favor of individual atomic scripts to ensure per-request correctness at the cost of one extra RTT to Redis. |
| **Fail-open strategy** | If Redis is unreachable, the system defaults to "Allow" to ensure system availability over strict enforcement. |

## Dependencies

*   **Redis**: Requires `v6.0+` for reliable atomic scripting and memory efficiency.
*   **Go-Redis/v9**: Recommended driver for high-performance connection pooling.
*   **Infrastructure**: Redis cluster should be deployed in a Multi-AZ configuration to prevent rate-limiter downtime.

---
**Architect's Note:** For high-throughput environments (10k+ RPS), we should implement an **In-Memory Local Cache** (e.g., Go-Cache) in front of Redis to perform a "pre-check" every 100ms to reduce network saturation.