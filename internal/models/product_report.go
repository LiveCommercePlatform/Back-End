package models

import "time"

type ProductReport struct {
	ID        uint   `gorm:"primaryKey"`
	ProductID string `gorm:"type:uuid;index"`
	UserID    string `gorm:"type:uuid;index"`
	Reason    string `gorm:"type:text;not null"`
	CreatedAt time.Time
}
