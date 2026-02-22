local likes = KEYS[1]
local dislikes = KEYS[2]
local uid = ARGV[1]

redis.call("SREM", likes, uid)
redis.call("SREM", dislikes, uid)

local likeCount = redis.call("SCARD", likes)
local dislikeCount = redis.call("SCARD", dislikes)

return {likeCount, dislikeCount}