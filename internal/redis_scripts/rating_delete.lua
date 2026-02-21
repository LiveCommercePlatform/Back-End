local userKey = KEYS[1]
local distKey = KEYS[2]
local metaKey = KEYS[3]

local uid = ARGV[1]

local oldStr = redis.call("HGET", userKey, uid)
if not oldStr then
  local count0 = redis.call("HGET", metaKey, "count") or "0"
  local sum0   = redis.call("HGET", metaKey, "sum") or "0"
  -- local avg0   = redis.call("HGET", metaKey, "avg") or "0"
  return { "", tostring(count0), tostring(sum0) }
end

local oldRating = tonumber(oldStr)
redis.call("HDEL", userKey, uid)
redis.call("HINCRBY", distKey, tostring(oldRating), -1)

local count = tonumber(redis.call("HGET", metaKey, "count") or "0")
local sum   = tonumber(redis.call("HGET", metaKey, "sum") or "0")

count = count - 1
sum   = sum - oldRating
if count < 0 then count = 0 end
if sum < 0 then sum = 0 end

-- local avg = 0
-- if count > 0 then
--   avg = sum / count
-- end

redis.call("HSET", metaKey, "count", tostring(count))
redis.call("HSET", metaKey, "sum", tostring(sum))
-- redis.call("HSET", metaKey, "avg", tostring(avg))

return { tostring(oldRating), tostring(count), tostring(sum)}