-- KEYS[1] = likes_set
-- KEYS[2] = dislikes_set
-- ARGV[1] = user_id

local likes = KEYS[1]
local dislikes = KEYS[2]
local uid = ARGV[1]

-- if already liked => toggle off
local alreadyLiked = redis.call("SISMEMBER", likes, uid)
if alreadyLiked == 1 then
  redis.call("SREM", likes, uid)
else
  -- switch from dislike to like
  redis.call("SREM", dislikes, uid)
  redis.call("SADD", likes, uid)
end

local likeCount = redis.call("SCARD", likes)
local dislikeCount = redis.call("SCARD", dislikes)

return {likeCount, dislikeCount}