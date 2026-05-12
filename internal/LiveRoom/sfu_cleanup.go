package liveRoom

import (
	// "sync"

	"github.com/google/uuid"
)

func DestroySFURoom(roomID uuid.UUID) {
	sfuMu.Lock()
	room, ok := sfuRooms[roomID]
	if !ok {
		sfuMu.Unlock()
		return
	}
	delete(sfuRooms, roomID)
	sfuMu.Unlock()

	// close peers خارج از lock سراسری
	room.mu.Lock()
	host := room.Host
	viewers := make([]*SFUPeer, 0, len(room.Viewers))
	for _, v := range room.Viewers {
		viewers = append(viewers, v)
	}
	room.Host = nil
	room.Viewers = map[string]*SFUPeer{}
	room.Forwarders = map[string]*SFUForwarder{}
	room.mu.Unlock()

	// Close PCها
	if host != nil && host.PC != nil {
		_ = host.PC.Close()
	}
	for _, v := range viewers {
		if v != nil && v.PC != nil {
			_ = v.PC.Close()
		}
	}
}

// اگر دوست داشتی هنگام start هم ریست کنی
func ResetSFURoom(roomID uuid.UUID) {
	DestroySFURoom(roomID)
}

// (اختیاری) برای test/debug
func HasSFURoom(roomID uuid.UUID) bool {
	sfuMu.Lock()
	defer sfuMu.Unlock()
	_, ok := sfuRooms[roomID]
	return ok
}

// فقط برای اینکه linter گیر نده اگر جایی sync لازم شد
// var _ = sync.Mutex{}