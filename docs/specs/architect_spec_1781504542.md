## ZenGate AI Rate Limiter: Redis Implementation

### Overview

This document outlines the system architecture for a Redis-based rate limiter to be integrated with ZenGate AI. The rate limiter will be responsible for controlling the number of requests a client can make to specific API endpoints within a defined time window. This design prioritizes efficiency, scalability, and robustness by leveraging Redis's in-memory data structures and atomic operations.

The core components are:

1.  **Rate Limiter Service:** The primary service responsible for orchestrating the rate limiting logic.
2.  **Redis Client:** An abstraction layer to interact with the Redis database.
3.  **Rate Limiter Storage:** Redis itself, used to store request counts and timestamps.

The rate limiter will support common rate limiting strategies such as:

*   **Fixed Window Counter:** Resets the count at the beginning of each fixed window.
*   **Sliding Window Log:** Stores a timestamp for each request and counts requests within the current window.
*   **Sliding Window Counter:** A hybrid approach that uses counters but approximates a sliding window.

For this initial design, we will focus on the **Sliding Window Counter** strategy as it offers a good balance between accuracy and performance, and is well-suited for Redis implementation using sorted sets or a combination of keys.

### Interface Contracts (Go Interfaces)

```go
package ratelimiter

// RateLimiter defines the contract for any rate limiting implementation.
type RateLimiter interface {
	// Allow checks if a request is allowed based on the provided key and limits.
	// It returns true if allowed, false otherwise. It may also return an error
	// if there's an issue communicating with the underlying storage.
	Allow(key string, limit int, window time.Duration) (bool, error)

	// GetRemaining returns the number of remaining requests allowed for a given key
	// within its defined limits.
	GetRemaining(key string, limit int, window time.Duration) (int, error)

	// GetResetTime returns the time when the limit will be reset for a given key.
	// The returned time.Time should be in UTC.
	GetResetTime(key string, limit int, window time.Duration) (time.Time, error)
}

// RedisClient defines the contract for interacting with a Redis instance.
// This interface abstracts away the specific Redis client library used.
type RedisClient interface {
	// SetNX sets a key with a value and an expiration time if the key does not exist.
	// Returns true if the key was set, false otherwise.
	SetNX(key string, value string, expiration time.Duration) (bool, error)

	// Incr increments the integer value of a key by one.
	// Returns the new value of the key after the increment.
	Incr(key string) (int64, error)

	// ZAdd adds one or more members to a sorted set, or updates their score.
	// Returns the number of elements added or updated.
	ZAdd(key string, members map[string]float64) (int64, error)

	// ZRemRangeByScore removes all members in the sorted set at key with a score
	// between min and max.
	ZRemRangeByScore(key string, min, max float64) (int64, error)

	// ZCard returns the number of elements in the sorted set at key.
	ZCard(key string) (int64, error)

	// TTL returns the time to live for a key.
	// Returns a negative value if the key does not exist or has no expire.
	TTL(key string) (time.Duration, error)

	// Get returns the string value of a key.
	Get(key string) (string, error)

	// Del removes the specified keys.
	// Returns the number of keys that were removed.
	Del(keys ...string) (int64, error)

	// Pipelined executes a group of commands in a pipeline.
	// It returns a list of replies in the same order as the commands.
	Pipelined(ctx context.Context, fn func(redis.Pipeliner) error) ([]redis.Cmder, error)

	// Close closes the Redis client connection.
	Close() error
}
```

### Data Flow

```mermaid
graph TD
    Client -> API_Gateway[API Gateway/ZenGate AI Core]
    API_Gateway -> RateLimiterService[Rate Limiter Service]
    RateLimiterService -> RedisClient[Redis Client Abstraction]
    RedisClient -> Redis[Redis Server]

    subgraph Rate Limiter Service
        RLS_Logic[Rate Limiting Logic]
        RLS_Cache[Local Cache (Optional)]
    end

    RateLimiterService --> RLS_Logic
    RLS_Logic --> RLS_Cache
    RLS_Cache --> RedisClient
    RLS_Logic --> RedisClient

    subgraph Redis Client Abstraction
        RC_Commands[Redis Commands]
    end

    RedisClient --> RC_Commands
    RC_Commands --> Redis

    subgraph Redis Server
        RL_Store[Rate Limit Data (Keys, Scores)]
        Exp_Policy[Expiration Policies]
    end

    Redis --> RL_Store
    Redis --> Exp_Policy

    RateLimiterService --> API_Gateway
    API_Gateway --> Client
```

**Detailed Data Flow for `Allow` operation (Sliding Window Counter):**

1.  **Client Request:** A client makes a request to an API endpoint.
2.  **API Gateway/ZenGate AI Core:** Intercepts the request and determines the appropriate rate limiting key (e.g., user ID, IP address, API key) and the rate limit rules (limit and window).
3.  **Rate Limiter Service:** Receives the key, limit, and window.
4.  **Redis Client Abstraction:**
    *   Constructs a Redis key for the current window (e.g., `ratelimit:<key>:<timestamp_of_window_start>`).
    *   Starts a Redis pipeline.
    *   **Command 1 (INCR):** Increments the counter for the current window key. This operation is atomic.
    *   **Command 2 (EXPIRE):** Sets an expiration time on the counter key, equal to the defined `window`. This ensures old windows are cleaned up automatically.
    *   **Command 3 (TTL - Optional but Recommended):** Gets the TTL of the key to accurately determine the reset time for the client.
    *   Executes the pipeline.
5.  **Redis Server:**
    *   Executes the `INCR` command atomically.
    *   Sets the `EXPIRE` for the key.
    *   Returns the new count and TTL.
6.  **Redis Client Abstraction:**
    *   Parses the results from the pipeline.
    *   If the `INCR` result is less than or equal to the `limit`, the request is allowed.
    *   Calculates the `resetTime` based on the TTL received from Redis.
7.  **Rate Limiter Service:**
    *   If the request is allowed, it returns `true` and potentially the `remaining` and `resetTime` to the API Gateway.
    *   If the request is denied (counter > limit), it returns `false` and the `resetTime` to the API Gateway.
8.  **API Gateway/ZenGate AI Core:** Based on the `Allow` result:
    *   If allowed, forwards the request to the intended service.
    *   If denied, returns a `429 Too Many Requests` response to the client, including headers like `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset`.

**Alternative Data Flow using Sorted Sets (Sliding Window Log):**

For the Sliding Window Log, the data flow would be slightly different:

1.  **Client Request:** Similar to above.
2.  **API Gateway/ZenGate AI Core:** Similar to above.
3.  **Rate Limiter Service:** Similar to above.
4.  **Redis Client Abstraction:**
    *   Generates a unique timestamp for the current request (e.g., using `time.Now().UnixNano()`).
    *   Starts a Redis pipeline.
    *   **Command 1 (ZADD):** Adds the current request timestamp as a member with its timestamp as the score to a sorted set representing the window (e.g., `ratelimit_log:<key>`).
    *   **Command 2 (ZREMRANGEBYSCORE):** Removes all members from the sorted set whose scores (timestamps) are older than the start of the current window (e.g., `current_time - window`).
    *   **Command 3 (ZADD ... NX):** A common pattern is to use `ZADD` with `NX` to atomically add a member *only if it doesn't exist*. This can be used to check if a request with the *exact* same timestamp has already been processed. However, this requires careful handling of timestamp granularity. A more robust approach is to use `ZADD` to add, then `ZREMRANGEBYSCORE` to prune, and then `ZCARD` to get the count within the window.
    *   **Command 4 (ZCARD):** Gets the number of elements currently in the sorted set.
    *   **Command 5 (EXPIRE):** Sets an expiration on the sorted set itself, slightly longer than the window, to ensure it's eventually cleaned up.
    *   Executes the pipeline.
5.  **Redis Server:** Executes commands.
6.  **Redis Client Abstraction:**
    *   Parses results.
    *   If `ZCARD` result is less than or equal to `limit`, the request is allowed.
    *   Calculates `resetTime` by taking the score of the oldest element in the set and adding the `window` duration.
7.  **Rate Limiter Service:** Similar to above.
8.  **API Gateway/ZenGate AI Core:** Similar to above.

### Design Decisions & Trade-offs

1.  **Strategy: Sliding Window Counter vs. Sliding Window Log:**
    *   **Sliding Window Counter (Chosen for initial implementation):**
        *   **Pros:** Very efficient in terms of memory and CPU usage in Redis. Uses simple `INCR` and `EXPIRE` commands. Good approximation of sliding window.
        *   **Cons:** Less accurate at the window boundaries compared to Sliding Window Log. Can slightly over-allow requests in the last few moments of a window.
        *   **Redis Implementation:** Uses a single key per window per client. `INCR` to count, `EXPIRE` to clean up.
    *   **Sliding Window Log:**
        *   **Pros:** Highly accurate. Tracks each individual request within the window.
        *   **Cons:** Can consume significantly more memory in Redis if request rates are very high, as it stores a timestamp for each request. More complex Redis commands (`ZADD`, `ZREMRANGEBYSCORE`, `ZCARD`).
        *   **Redis Implementation:** Uses a sorted set per client, storing timestamps as scores.

    *   **Decision:** Start with the **Sliding Window Counter** for its performance and simplicity. If accuracy becomes a critical issue, consider migrating to the **Sliding Window Log** or exploring hybrid approaches.

2.  **Redis Data Structures:**
    *   **Strings (for Fixed Window Counter):** Simple, but prone to accuracy issues at window boundaries.
    *   **Sorted Sets (for Sliding Window Log):** Good for logging and precise windowing.
    *   **Combination of Keys (for Sliding Window Counter):** A common approach is to use a key like `ratelimit:<key>:<window_timestamp>` and increment it. This allows for automatic cleanup via Redis expiration.

    *   **Decision:** For the Sliding Window Counter, we will use **String keys** with `INCR` and `EXPIRE`. Each distinct window will have its own key. The key will incorporate the client identifier and the timestamp of the window's start.

3.  **Atomic Operations:**
    *   Redis's atomic commands (`INCR`, `ZADD`, `ZREMRANGEBYSCORE`) are crucial for ensuring thread-safe rate limiting, especially when multiple requests from the same client arrive concurrently.
    *   **Decision:** Leverage Redis pipelines for executing multiple commands atomically within a single round trip, and rely on Redis's atomic command execution.

4.  **Key Naming Convention:**
    *   A clear and consistent naming convention is important for managing keys in Redis.
    *   **Decision:** Use a pattern like `ratelimit:<strategy>:<client_identifier>:<window_start_timestamp>`. For example, `ratelimit:sliding_counter:user_123:1678886400`.

5.  **Error Handling:**
    *   Network issues with Redis, Redis server errors, or invalid configurations can occur.
    *   **Decision:** The `RateLimiter` interface includes an `error` return. The `RateLimiterService` should log errors related to Redis communication and potentially fall back to a less strict policy (e.g., allow all, or allow a default higher limit) or fail open/closed depending on the desired resilience. For critical services, failing closed (denying requests) might be safer.

6.  **Distributed Nature:**
    *   ZenGate AI might run multiple instances of the Rate Limiter Service.
    *   **Decision:** Redis acts as the single source of truth for rate limiting state, making the rate limiting mechanism inherently distributed and consistent across multiple service instances.

7.  **Local Caching (Optional):**
    *   For extremely high throughput scenarios, a very short-lived local cache (e.g., in-memory map with TTL) within the `RateLimiterService` could reduce Redis load for repeated checks of the same key within a very small timeframe.
    *   **Decision:** Defer local caching. Redis is generally fast enough for most use cases. If Redis becomes a bottleneck, local caching can be introduced as an optimization, but it adds complexity (cache invalidation, consistency).

8.  **Expiration of Keys:**
    *   Redis automatically handles key expiration based on the `EXPIRE` command. This is essential for cleaning up old rate limiting data.
    *   **Decision:** Rely on Redis's TTL mechanism. Ensure the `EXPIRE` is set correctly on the relevant keys (counter keys or sorted sets).

### Dependencies

*   **Redis:** A running Redis server instance.
*   **Go Redis Client Library:** A well-maintained Go Redis client library (e.g., `go-redis/redis/v8`). The `RedisClient` interface abstracts this dependency.
*   **Time:** Standard Go `time` package for durations and timestamps.
*   **Context:** Standard Go `context` package for request cancellation and deadlines, especially when interacting with Redis.
*   **ZenGate AI Core/API Gateway:** The component that will consume the `RateLimiter` interface.