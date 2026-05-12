package liveRoom

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ChatEventsHub *ChatHub

const (
	chatPongWait       = 60 * time.Second
	chatPingPeriod     = (chatPongWait * 9) / 10
	chatMaxMessageSize = 4096
	chatHistoryMaxKeep = 200
)

func chatKey(roomID uuid.UUID) string {
	return fmt.Sprintf("live:%s:chat", roomID.String())
}

func loadRoomLiveOnly(c *gin.Context, roomID uuid.UUID) (*models.LiveRoom, bool) {
	var lr models.LiveRoom
	if err := database.DB.Select("id,status").First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return nil, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return nil, false
	}

	if lr.Status != models.LiveLive {
		c.JSON(http.StatusConflict, gin.H{"error": "live_room_not_live"})
		return nil, false
	}

	return &lr, true
}

// WSChat godoc
// @Summary LiveRoom chat websocket
// @Description Authenticated realtime chat for a live room
// @Tags liveroom
// @Security BearerAuth
// @Param id path string true "LiveRoom ID (uuid)"
// @Success 101 {string} string "Switching Protocols"
// @Router /ws/live-rooms/{id}/chat [get]
func WSChat() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomID, ok := parseUUIDParam(c, "id")
		if !ok {
			return
		}

		// auth required
		userID, _, ok := mustGetAuth(c)
		if !ok {
			return
		}

		// live-only policy
		if _, ok := loadRoomLiveOnly(c, roomID); !ok {
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		if ChatEventsHub == nil {
			ChatEventsHub = NewChatHub()
		}

		client := &chatClient{conn: conn}
		ChatEventsHub.Add(roomID, client)
		defer func() {
			ChatEventsHub.Remove(roomID, client)
			client.close()
		}()

		conn.SetReadLimit(chatMaxMessageSize)
		_ = conn.SetReadDeadline(time.Now().Add(chatPongWait))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(chatPongWait))
		})

		// ping loop
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(chatPingPeriod)
			defer ticker.Stop()
			defer close(done)

			for range ticker.C {
				if err := client.writePing(); err != nil {
					return
				}
			}
		}()

		// read loop
		for {
			_, b, err := conn.ReadMessage()
			if err != nil {
				<-done
				return
			}

			var in chatIncoming
			if err := json.Unmarshal(b, &in); err != nil {
				_ = client.writeJSON(chatError{Type: "chat.error", Error: "invalid_json", TS: time.Now().Unix()})
				continue
			}

			if in.Type != "chat.send" {
				_ = client.writeJSON(chatError{Type: "chat.error", Error: "unsupported_type", TS: time.Now().Unix()})
				continue
			}

			text := strings.TrimSpace(in.Text)
			if text == "" {
				_ = client.writeJSON(chatError{Type: "chat.error", Error: "empty_text", TS: time.Now().Unix()})
				continue
			}
			if len(text) > 500 {
				_ = client.writeJSON(chatError{Type: "chat.error", Error: "text_too_long", TS: time.Now().Unix()})
				continue
			}

			msgID := uuid.NewString()
			now := time.Now().Unix()

			ev := chatEvent{
				Type:   "chat.message",
				RoomID: roomID.String(),
				Data: chatMessageData{
					ID:     msgID,
					UserID: userID.String(),
					Text:   text,
					TS:     now,
				},
				TS: now,
			}

			// persist in redis (best-effort)
			raw, _ := json.Marshal(ev)
			key := chatKey(roomID)

			// ✅ درست: context.Context از request
			ctx := c.Request.Context()

			pipe := cache.Client.Pipeline()
			pipe.LPush(ctx, key, raw)
			pipe.LTrim(ctx, key, 0, chatHistoryMaxKeep-1)
			pipe.Expire(ctx, key, 24*time.Hour)
			_, _ = pipe.Exec(ctx)

			// broadcast to room
			ChatEventsHub.Broadcast(roomID, ev)

			// ack to sender if client_msg_id provided
			if strings.TrimSpace(in.ClientMsgID) != "" {
				_ = client.writeJSON(chatAck{
					Type:        "chat.ack",
					ClientMsgID: in.ClientMsgID,
					ID:          msgID,
					TS:          time.Now().Unix(),
				})
			}
		}
	}
}

// GetChatHistory godoc
// @Summary Chat history
// @Description Get last N chat messages (authenticated)
// @Tags liveroom
// @Security BearerAuth
// @Produce json
// @Param id path string true "LiveRoom ID (uuid)"
// @Param limit query int false "max items (default 50, max 200)"
// @Success 200 {object} map[string]any
// @Failure 400,401,404,500 {object} map[string]string
// @Router /live-rooms/{id}/chat/history [get]
func GetChatHistory(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	// auth required
	if _, _, ok := mustGetAuth(c); !ok {
		return
	}

	// existence check
	var lr models.LiveRoom
	if err := database.DB.Select("id").First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	limit := 50
	if s := strings.TrimSpace(c.Query("limit")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_limit"})
			return
		}
		if n > 200 {
			n = 200
		}
		limit = n
	}

	key := chatKey(roomID)

	// ✅ درست: context.Context از request
	ctx := c.Request.Context()

	items, err := cache.Client.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	events := make([]chatEvent, 0, len(items))
	for _, it := range items {
		var ev chatEvent
		if err := json.Unmarshal([]byte(it), &ev); err == nil {
			events = append(events, ev)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"items": events,
	})
}