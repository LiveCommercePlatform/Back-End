package liveRoom

import (
	// "net/http"
	"time"

	// "livecommerce/internal/common/paramutil"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: allowWSOrigin,
}

const (
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 262144  
)





// WSLiveRoomEvents godoc
// @Summary LiveRoom events websocket
// @Description Subscribe to live room public events (products updates etc.)
// @Tags liveroom
// @Param id path string true "LiveRoom ID (uuid)"
// @Success 101 {string} string "Switching Protocols"
// @Router /ws/live-rooms/{id}/events [get]
func WSLiveRoomEvents(hub *RoomHub) gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, ok := parseUUIDParam(c, "id")
		if !ok {
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		client := &wsClient{
    conn: conn,
    Meta: map[string]string{}, }

		hub.Add(roomID, client)
		defer func() {
			hub.Remove(roomID, client)
			client.close()
		}()

		conn.SetReadLimit(maxMessageSize)

		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})

		// ping loop (از client.write استفاده می‌کنیم تا race با broadcast نشه)
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(pingPeriod)
			defer ticker.Stop()
			defer close(done)

			for {
				<-ticker.C
				if err := client.write(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				<-done
				return
			}
		}
	}
}