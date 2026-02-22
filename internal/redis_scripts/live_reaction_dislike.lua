local likes = KEYS[1]
local dislikes = KEYS[2]
local uid = ARGV[1]

local alreadyDisliked = redis.call("SISMEMBER", dislikes, uid)
if alreadyDisliked == 1 then
  redis.call("SREM", dislikes, uid)
else
  redis.call("SREM", likes, uid)
  redis.call("SADD", dislikes, uid)
end

local likeCount = redis.call("SCARD", likes)
local dislikeCount = redis.call("SCARD", dislikes)

return {likeCount, dislikeCount}