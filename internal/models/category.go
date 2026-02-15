package models

import "time"

type Category struct {
	ID uint `gorm:"primaryKey" json:"id"`

	Key string `gorm:"uniqueIndex;size:64;not null" json:"key"`

	NameFa string `gorm:"size:120;not null" json:"name_fa"`

	ParentID *uint     `gorm:"index" json:"parent_id,omitempty"`
	Parent   *Category `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Children []Category `gorm:"foreignKey:ParentID" json:"children,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
