local likesKey = KEYS[1]
local dislikesKey = KEYS[2]
local metaKey = KEYS[3]
local uid = ARGV[1]

local alreadyLiked = redis.call("SISMEMBER", likesKey, uid)
if alreadyLiked == 1 then
  local lc = redis.call("HGET", metaKey, "likes_count") or "0"
  local dc = redis.call("HGET", metaKey, "dislikes_count") or "0"
  return {lc, dc, "0"}
end

local removedDislike = redis.call("SREM", dislikesKey, uid)
local addedLike = redis.call("SADD", likesKey, uid)

-- update meta
if addedLike == 1 then
  redis.call("HINCRBY", metaKey, "likes_count", 1)
end
if removedDislike == 1 then
  redis.call("HINCRBY", metaKey, "dislikes_count", -1)
end

local lc = redis.call("HGET", metaKey, "likes_count") or "0"
local dc = redis.call("HGET", metaKey, "dislikes_count") or "0"
return {lc, dc, "1"}