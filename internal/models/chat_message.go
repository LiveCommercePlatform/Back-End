package models

import (
    "github.com/google/uuid"
    "time"
)

type ChatMessage struct {
    ID         uint      `gorm:"primaryKey" json:"id"`
    RoomID     uuid.UUID `gorm:"type:uuid;not null;index" json:"room_id"`
    UserID     uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
    UserName   string    `gorm:"not null" json:"user_name"`
    Text       string    `gorm:"type:text;not null" json:"text"`
    SentAt     time.Time `json:"sent_at"`
    CreatedAt  time.Time `json:"created_at"`
}