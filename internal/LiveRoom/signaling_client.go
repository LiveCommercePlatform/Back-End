package liveRoom

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
)

type WSClient struct {
	Conn *websocket.Conn

	// outbound messages
	Send chan any

	closeOnce sync.Once
	closed    chan struct{}
}

func NewWSClient(conn *websocket.Conn) *WSClient {

	return &WSClient{
		Conn:   conn,
		Send:   make(chan any, 256),
		closed: make(chan struct{}),
	}
}

func (c *WSClient) ReadPump(handler func(messageType int, data []byte)) {

	defer c.Close()

	c.Conn.SetReadLimit(maxMessageSize)

	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))

	c.Conn.SetPongHandler(func(string) error {

		return c.Conn.SetReadDeadline(
			time.Now().Add(pongWait),
		)
	})

	for {

		messageType, message, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}

		handler(messageType, message)
	}

	
}

func (c *WSClient) WritePump() {

	ticker := time.NewTicker(pingPeriod)

	defer func() {

		ticker.Stop()

		c.Close()
	}()

	for {

		select {

		case msg, ok := <-c.Send:

			_ = c.Conn.SetWriteDeadline(
				time.Now().Add(writeWait),
			)

			if !ok {

				_ = c.Conn.WriteMessage(
					websocket.CloseMessage,
					[]byte{},
				)

				return
			}

			switch v := msg.(type) {

			case []byte:

				if err := c.Conn.WriteMessage(
					websocket.TextMessage,
					v,
				); err != nil {
					return
				}

			default:

				if err := c.Conn.WriteJSON(v); err != nil {
					return
				}
			}

		case <-ticker.C:

			_ = c.Conn.SetWriteDeadline(
				time.Now().Add(writeWait),
			)

			if err := c.Conn.WriteMessage(
				websocket.PingMessage,
				nil,
			); err != nil {

				return
			}

		case <-c.closed:
			return
		}
	}
}

func (c *WSClient) SafeSend(v any) bool {

	defer func() {
		recover()
	}()

	select {

	case <-c.closed:
		return false

	case c.Send <- v:
		return true

	default:
		return false
	}
}

func (c *WSClient) Close() {

	c.closeOnce.Do(func() {

		close(c.closed)

		close(c.Send)

		_ = c.Conn.Close()
	})
}