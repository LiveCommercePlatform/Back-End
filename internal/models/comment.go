package models

import (
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID        uint       `gorm:"primaryKey" json:"id"`

	ProductID uuid.UUID `gorm:"type:uuid;index;not null" json:"product_id"`
	Product   *Product   `gorm:"constraint:OnDelete:CASCADE" json:"-"`

	UserID    uuid.UUID `gorm:"type:uuid;index;not null" json:"user_id"`
	User *User `gorm:"constraint:OnDelete:CASCADE" json:"user"`

	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
