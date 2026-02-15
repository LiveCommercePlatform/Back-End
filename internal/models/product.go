package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	// "github.com/gosimple/slug"
)

type Product struct {
	ID      uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`



	Title       string `gorm:"not null" json:"title"`
	Description string `gorm:"type:text" json:"description"`
	// Slug        string `gorm:"uniqueIndex;not null" json:"slug"`
	Price       int64  `gorm:"not null" json:"price"`

	Stock int `gorm:"default:0" json:"stock"`

	OwnerID uuid.UUID `gorm:"type:char(36);not null;index" json:"owner_id"`
	Owner  *User `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`


	ViewCount int64 `gorm:"default:0" json:"view_count"`
	LikeCount int64 `gorm:"default:0" json:"like_count"`
	DislikeCount int64   `gorm:"default:0" json:"dislike_count"`

	RatingCount  int64   `gorm:"default:0" json:"rating_count"`
	RatingSum    int64   `gorm:"default:0" json:"rating_sum"`
	RatingAvg    float64 `gorm:"default:0" json:"rating_avg"`

	Media    []ProductMedia `gorm:"constraint:OnDelete:CASCADE" json:"media"`
	Comments []Comment      `gorm:"constraint:OnDelete:CASCADE" json:"comments"`
	Reports  []ProductReport `gorm:"constraint:OnDelete:CASCADE" json:"reports"`

	CategoryID uint      `gorm:"index;not null" json:"category_id"`
	Category   *Category `gorm:"foreignKey:CategoryID" json:"category,omitempty"`

	Tags []Tag `gorm:"many2many:product_tags" json:"tags,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (p *Product) BeforeCreate(tx *gorm.DB) error {
	p.ID = uuid.New()
	// base := slug.Make(p.Title)
	// if base == "" { base = uuid.New().String()[:8] }
	// p.Slug = base + "-" + uuid.New().String()[:6]

return nil
}
