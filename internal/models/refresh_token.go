package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RefreshToken struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	TokenHash string         `gorm:"uniqueIndex;not null" json:"-"`
	UserID    uuid.UUID      `gorm:"type:uuid;index;not null" json:"user_id"` 
	ExpiresAt time.Time      `json:"expires_at"`
	Revoked   bool           `gorm:"default:false" json:"revoked"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
