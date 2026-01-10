local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = 1

local last_tokens = tonumber(redis.call("hget", key, "tokens"))
local last_refreshed = tonumber(redis.call("hget", key, "last_refreshed"))

if last_tokens == nil then
    last_tokens = burst
    last_refreshed = now
end

local delta = math.max(0, now - last_refreshed)
local new_tokens = math.min(burst, last_tokens + (delta * rate))

local allowed = false
if new_tokens >= requested then
    new_tokens = new_tokens - requested
    allowed = true
end

redis.call("hset", key, "tokens", new_tokens, "last_refreshed", now)
redis.call("expire", key, math.ceil(burst / rate) * 2)

return allowed and 1 or 0
