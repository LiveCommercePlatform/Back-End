package models

type Tag struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
    Name string `gorm:"size:24;not null;uniqueIndex:uidx_tags_name" json:"name"`
	Products []Product `gorm:"many2many:product_tags" json:"-"`
}

