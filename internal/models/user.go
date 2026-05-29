package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleUser  Role = "user"
	RoleBanned Role = "banned"
)

type User struct {
	ID uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`
	Name      string    `json:"name"`
	Email     string    `gorm:"unique" json:"email"`
	Password  string    `json:"-"`
	Role      Role      `json:"role"`
	Verified  bool 		`gorm:"default:false" json:"verified"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	PostalCode string   `json:"postal_code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	u.ID = uuid.New()
	return
}
