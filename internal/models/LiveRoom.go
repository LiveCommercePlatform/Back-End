package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)




type LiveStatus string

const (
  LiveScheduled LiveStatus = "scheduled"
  LiveLive      LiveStatus = "live"
  LiveEnded     LiveStatus = "ended"
)

type LiveRoom struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey"`
	HostID uuid.UUID `gorm:"type:uuid;not null;index"`
	Host   User      `gorm:"foreignKey:HostID;constraint:OnDelete:CASCADE"`

	Title       string `gorm:"not null"`
	Description string

	Products []LiveRoomProduct `gorm:"foreignKey:LiveRoomID;constraint:OnDelete:CASCADE"`



	Status    LiveStatus  `gorm:"type:varchar(20);not null;default:'scheduled';index"`
	StartedAt *time.Time  `gorm:"index"`
	EndedAt   *time.Time

	IsRecorded bool `gorm:"default:false"`
	TotalViews int64 `gorm:"default:0"`
	TotalLikes int64 `gorm:"default:0"`
	TotalDislikes int64 `gorm:"default:0"`
	Duration   int64 `gorm:"default:0"`

	CreatedAt time.Time
	UpdatedAt time.Time
}


func (lr *LiveRoom) BeforeCreate(tx *gorm.DB) error {
  if lr.ID == uuid.Nil {
    lr.ID = uuid.New()
  }
  return nil
}