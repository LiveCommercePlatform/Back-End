package models

import "github.com/google/uuid"

type MediaType string

const (
	MediaImage MediaType = "image"
	MediaVideo MediaType = "video"
)

type ProductMedia struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProductID uuid.UUID `gorm:"type:uuid;index" json:"product_id"`
	Product   *Product   `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	URL       string    `gorm:"not null" json:"url"`
	Type      MediaType `gorm:"not null" json:"type"`
}



