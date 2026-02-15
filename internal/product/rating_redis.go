package product

import "github.com/google/uuid"

func ratingUserKey(pid uuid.UUID) string { return "product:" + pid.String() + ":ratings:user" }
func ratingDistKey(pid uuid.UUID) string { return "product:" + pid.String() + ":ratings:dist" }
func ratingMetaKey(pid uuid.UUID) string { return "product:" + pid.String() + ":ratings:meta" }