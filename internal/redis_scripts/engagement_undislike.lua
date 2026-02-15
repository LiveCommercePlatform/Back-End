local dislikesKey = KEYS[1]
local metaKey = KEYS[2]
local uid = ARGV[1]

local removed = redis.call("SREM", dislikesKey, uid)
if removed == 1 then
  redis.call("HINCRBY", metaKey, "dislikes_count", -1)
end
local lc = redis.call("HGET", metaKey, "likes_count") or "0"
local dc = redis.call("HGET", metaKey, "dislikes_count") or "0"
return {lc, dc, tostring(removed)}