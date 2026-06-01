package liveRoom

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func (h *RoomHub) BroadcastToRole(roomID uuid.UUID, role string, payload any) {
    data, _ := json.Marshal(payload)

    h.mu.RLock()
    conns := h.rooms[roomID]
    clients := make([]*wsClient, 0)
    for c := range conns {
        if c.Meta["role"] == role {
            clients = append(clients, c)
        }
    }
    h.mu.RUnlock()  

    for _, c := range clients {
        c.write(websocket.TextMessage, data)
    }
}

func (h *RoomHub) BroadcastToTarget(roomID uuid.UUID, target string, payload any) {

	data, _ := json.Marshal(payload)

	h.mu.RLock()
	conns := h.rooms[roomID]
	h.mu.RUnlock()

	for c := range conns {
		if c.Meta["id"] == target {
			c.write(websocket.TextMessage, data)
		}
	}
}

func (h *RoomHub) PushViewerCount(roomID uuid.UUID) {

	viewerCount := 0

	h.mu.RLock()
	conns := h.rooms[roomID]
	h.mu.RUnlock()

	for c := range conns {
		if c.Meta["role"] == "viewer" {
			viewerCount++
		}
	}

	h.Broadcast(roomID, map[string]any{
		"type":  "viewer-count",
		"count": viewerCount,
	})
}
