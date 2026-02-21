package product

import "github.com/google/uuid"


func likesKey(pid uuid.UUID) string      { return "product:" + pid.String() + ":likes" }
func dislikesKey(pid uuid.UUID) string   { return "product:" + pid.String() + ":dislikes" }
func engageMetaKey(pid uuid.UUID) string { return "product:" + pid.String() + ":engagement:meta" }



