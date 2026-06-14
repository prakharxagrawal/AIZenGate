-- =============================================================================
-- ZenGate AI — Sliding Window Log Rate Limiter (Lua Script)
-- =============================================================================
-- KEYS[1]: Client rate-limit key (e.g., "zg:rl:client_123")
-- ARGV[1]: Current timestamp (milliseconds)
-- ARGV[2]: Max requests allowed in the window
-- ARGV[3]: Window size (milliseconds)
-- =============================================================================

local key = KEYS[1]
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local window = tonumber(ARGV[3])

-- Remove logs older than the current window boundary
local clear_before = now - window
redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

-- Count the remaining active logs in the window
local current_requests = redis.call('ZCARD', key)

if current_requests < limit then
    -- Add the current request (using the timestamp as member and score)
    -- We append a random suffix to score to ensure uniqueness if multiple requests hit at the same millisecond
    local unique_member = now .. ':' .. math.random()
    redis.call('ZADD', key, now, unique_member)
    
    -- Set TTL on the key so it automatically gets cleaned up if idle
    redis.call('PEXPIRE', key, window)
    
    -- Allowed
    return 1
else
    -- Rate limited
    return 0
end
