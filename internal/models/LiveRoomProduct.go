package models

import (
	"time"

	"github.com/google/uuid"
)

type LiveRoomProduct struct {
	ID         uint      `gorm:"primaryKey"`
	LiveRoomID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:ux_room_product"`
	ProductID  uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:ux_room_product"`


	IsPinned  bool `gorm:"default:false;index"`
	SortOrder int  `gorm:"default:0"`

	LiveRoom LiveRoom `gorm:"foreignKey:LiveRoomID;constraint:OnDelete:CASCADE"`
	Product  Product  `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time
	UpdatedAt time.Time
}