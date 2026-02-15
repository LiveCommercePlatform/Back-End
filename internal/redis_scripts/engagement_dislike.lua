local likesKey = KEYS[1]
local dislikesKey = KEYS[2]
local metaKey = KEYS[3]
local uid = ARGV[1]

local alreadyDisliked = redis.call("SISMEMBER", dislikesKey, uid)
if alreadyDisliked == 1 then
  local lc = redis.call("HGET", metaKey, "likes_count") or "0"
  local dc = redis.call("HGET", metaKey, "dislikes_count") or "0"
  return {lc, dc, "0"}
end

local removedLike = redis.call("SREM", likesKey, uid)
local addedDislike = redis.call("SADD", dislikesKey, uid)

if addedDislike == 1 then
  redis.call("HINCRBY", metaKey, "dislikes_count", 1)
end
if removedLike == 1 then
  redis.call("HINCRBY", metaKey, "likes_count", -1)
end

local lc = redis.call("HGET", metaKey, "likes_count") or "0"
local dc = redis.call("HGET", metaKey, "dislikes_count") or "0"
return {lc, dc, "1"}