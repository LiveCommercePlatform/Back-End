local userKey = KEYS[1]
local distKey = KEYS[2]
local metaKey = KEYS[3]

local uid = ARGV[1]
local newRating = tonumber(ARGV[2])

if (newRating < 1 or newRating > 5) then
  return redis.error_reply("rating_out_of_range")
end

local oldStr = redis.call("HGET", userKey, uid)
local oldRating = nil
if oldStr then oldRating = tonumber(oldStr) end

local count = tonumber(redis.call("HGET", metaKey, "count") or "0")
local sum   = tonumber(redis.call("HGET", metaKey, "sum") or "0")

if not oldRating then
  redis.call("HSET", userKey, uid, newRating)
  redis.call("HINCRBY", distKey, tostring(newRating), 1)
  count = count + 1
  sum   = sum + newRating
  redis.call("HSET", metaKey, "count", count)
  redis.call("HSET", metaKey, "sum", sum)
else
  if oldRating ~= newRating then
    redis.call("HSET", userKey, uid, newRating)
    redis.call("HINCRBY", distKey, tostring(oldRating), -1)
    redis.call("HINCRBY", distKey, tostring(newRating), 1)
    sum = sum + (newRating - oldRating)
    redis.call("HSET", metaKey, "sum", sum)
  end
end

local avg = 0
if count > 0 then
  avg = sum / count
end
redis.call("HSET", metaKey, "avg", tostring(avg))

return { oldStr or "", tostring(newRating), tostring(count), tostring(sum), tostring(avg) }