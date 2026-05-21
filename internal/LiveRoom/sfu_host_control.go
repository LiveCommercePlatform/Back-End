package liveRoom

import (
	"errors"
	"time"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func endLiveRoomFromSFU(roomID uuid.UUID) {
	// اگر DB خطا داد، نمی‌خوای panic کنی، فقط best-effort
	var lr models.LiveRoom
	if err := database.DB.First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		return
	}

	// اگر قبلاً ended شده، فقط cleanup
	if lr.Status == models.LiveEnded {
		DestroySFURoom(roomID)
		return
	}

	// وضعیت رو ended کن
	_ = database.DB.Model(&models.LiveRoom{}).
		Where("id = ? AND status = ?", roomID, models.LiveLive).
		Updates(map[string]any{
			"status":    models.LiveEnded,
			"ended_at":  time.Now(),
		}).Error

	// به UI خبر بده (اگر EventsHub داری)
	if EventsHub != nil {
		ev := LiveRoomEvent{
			Type:   "live_room.ended",
			RoomID: roomID.String(),
			Data:   map[string]any{"reason": "host_disconnected"},
			TS:     time.Now().Unix(),
		}
		EventsHub.Broadcast(roomID, ev)
	}

	// همه PCها رو ببند و room state رو پاک کن
	DestroySFURoom(roomID)
}