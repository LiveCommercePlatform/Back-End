package liveRoom

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex 
	Meta map[string]string
}

func (c *wsClient) write(msgType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(msgType, data)
}

func (c *wsClient) close() {
	_ = c.conn.Close()
}

type RoomHub struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]map[*wsClient]struct{}
}

func NewRoomHub() *RoomHub {
	return &RoomHub{
		rooms: make(map[uuid.UUID]map[*wsClient]struct{}),
	}
}

func (h *RoomHub) Add(roomID uuid.UUID, client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.rooms[roomID]; !ok {
		h.rooms[roomID] = make(map[*wsClient]struct{})
	}
	h.rooms[roomID][client] = struct{}{}
}

func (h *RoomHub) Remove(roomID uuid.UUID, client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns, ok := h.rooms[roomID]
	if !ok {
		return
	}
	delete(conns, client)
	if len(conns) == 0 {
		delete(h.rooms, roomID)
	}
}

func (h *RoomHub) Broadcast(roomID uuid.UUID, payload any) {
    b, err := json.Marshal(payload)
    if err != nil {
        return
    }

    h.mu.RLock()
    roomConns, ok := h.rooms[roomID]
    if !ok || len(roomConns) == 0 {
        h.mu.RUnlock()
        return
    }
    clients := make([]*wsClient, 0, len(roomConns))
    for c := range roomConns {
        clients = append(clients, c)
    }
    h.mu.RUnlock()

    // ← هر client در goroutine جداگانه
    var wg sync.WaitGroup
    for _, cl := range clients {
        wg.Add(1)
        go func(c *wsClient) {
            defer wg.Done()
            if err := c.write(websocket.TextMessage, b); err != nil {
                h.Remove(roomID, c)
                c.close()
            }
        }(cl)
    }
    wg.Wait()
}