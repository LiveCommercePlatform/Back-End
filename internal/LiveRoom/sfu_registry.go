package liveRoom

import (
	"sync"

	"github.com/google/uuid"
)

var (
	sfuMu sync.RWMutex

	sfuRooms =
		make(map[uuid.UUID]*SFURoom)
)

func getOrCreateSFURoom(
	roomID uuid.UUID,
) *SFURoom {

	sfuMu.Lock()
	defer sfuMu.Unlock()

	room, ok := sfuRooms[roomID]
	if ok {
		return room
	}

	room = NewSFURoom(roomID)

	sfuRooms[roomID] = room

	return room
}