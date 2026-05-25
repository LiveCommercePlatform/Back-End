package models

import (
	"time"

	"github.com/google/uuid"
)


type ReportType   string
type ReportStatus string

const (
	ReportTypeProduct ReportType = "product"
	ReportTypeComment ReportType = "comment"
	ReportTypeUser    ReportType = "user"
)

const (
	ReportStatusNew       ReportStatus = "new"
	ReportStatusReviewing ReportStatus = "reviewing"
	ReportStatusClosed    ReportStatus = "closed"
)

type Report struct {
	ID         uint         `gorm:"primaryKey" json:"id"`
	ReporterID uuid.UUID    `gorm:"type:uuid;not null;index" json:"reporter_id"`
	Reporter   *User        `gorm:"foreignKey:ReporterID" json:"reporter,omitempty"`

	Type   ReportType   `gorm:"type:varchar(20);not null;index" json:"type"`
	Status ReportStatus `gorm:"type:varchar(20);not null;default:'new';index" json:"status"`
	Reason string       `gorm:"type:text;not null" json:"reason"`

	ProductID *uuid.UUID `gorm:"type:uuid;index" json:"product_id,omitempty"`
	CommentID *uint      `gorm:"index" json:"comment_id,omitempty"`
	TargetUserID *uuid.UUID `gorm:"type:uuid;index" json:"target_user_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}


type MessageStatus string

const (
	MessageUnread MessageStatus = "unread"
	MessageRead   MessageStatus = "read"
)

type Message struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"not null" json:"name"`
	Email   string `gorm:"not null" json:"email"`
	Content string `gorm:"type:text;not null" json:"content"`

	Status MessageStatus `gorm:"type:varchar(10);not null;default:'unread';index" json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
