package liveRoom

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type chatClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *chatClient) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, b)
}

func (c *chatClient) writePing() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

func (c *chatClient) close() { _ = c.conn.Close() }

type ChatHub struct {
	mu    sync.RWMutex
	rooms map[uuid.UUID]map[*chatClient]struct{}
}

func NewChatHub() *ChatHub {
	return &ChatHub{rooms: make(map[uuid.UUID]map[*chatClient]struct{})}
}

func (h *ChatHub) Add(roomID uuid.UUID, cl *chatClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.rooms[roomID]; !ok {
		h.rooms[roomID] = make(map[*chatClient]struct{})
	}
	h.rooms[roomID][cl] = struct{}{}
}

func (h *ChatHub) Remove(roomID uuid.UUID, cl *chatClient) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns, ok := h.rooms[roomID]
	if !ok {
		return
	}
	delete(conns, cl)
	if len(conns) == 0 {
		delete(h.rooms, roomID)
	}
}

func (h *ChatHub) Broadcast(roomID uuid.UUID, payload any) {
	// snapshot
	h.mu.RLock()
	roomConns, ok := h.rooms[roomID]
	if !ok || len(roomConns) == 0 {
		h.mu.RUnlock()
		return
	}
	clients := make([]*chatClient, 0, len(roomConns))
	for c := range roomConns {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, cl := range clients {
		if err := cl.writeJSON(payload); err != nil {
			h.Remove(roomID, cl)
			cl.close()
		}
	}
}