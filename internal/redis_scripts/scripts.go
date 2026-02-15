package redis_scripts

import _ "embed"

// Engagement
//go:embed engagement_like.lua
var EngagementLike string

//go:embed engagement_unlike.lua
var EngagementUnlike string

//go:embed engagement_dislike.lua
var EngagementDislike string

//go:embed engagement_undislike.lua
var EngagementUndislike string

// Rating
//go:embed rating_upsert.lua
var RatingUpsert string

//go:embed rating_delete.lua
var RatingDelete string