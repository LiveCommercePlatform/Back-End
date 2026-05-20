package liveRoom

import (
	"github.com/google/uuid"
)

func DestroySFURoom(
	roomID uuid.UUID,
) {

	sfuMu.Lock()

	room, ok := sfuRooms[roomID]

	if !ok {

		sfuMu.Unlock()

		return
	}

	delete(sfuRooms, roomID)

	sfuMu.Unlock()

	room.mu.Lock()

	host := room.Host

	viewers := make(
		[]*SFUPeer,
		0,
		len(room.Viewers),
	)

	for _, v := range room.Viewers {
		viewers = append(viewers, v)
	}

	forwarders := make(
		[]*SFUForwarder,
		0,
		len(room.Forwarders),
	)

	for _, f := range room.Forwarders {
		forwarders = append(
			forwarders,
			f,
		)
	}

	room.Host = nil

	room.Viewers =
		make(map[string]*SFUPeer)

	room.Forwarders =
		make(map[string]*SFUForwarder)

	room.mu.Unlock()

	if host != nil {
		host.Close()
	}

	for _, v := range viewers {

		if v != nil {
			v.Close()
		}
	}

	for _, f := range forwarders {

		if f != nil {
			f.Close()
		}
	}
}

func ResetSFURoom(
	roomID uuid.UUID,
) {

	DestroySFURoom(roomID)
}

func HasSFURoom(
	roomID uuid.UUID,
) bool {

	sfuMu.RLock()
	defer sfuMu.RUnlock()

	_, ok := sfuRooms[roomID]

	return ok
}

