package liveRoom

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"livecommerce/internal/auth"
	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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

func InitChatHub() {
    ChatEventsHub = NewChatHub()
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


func mustGetAuthFromRequest(r *http.Request) (uuid.UUID, string, bool) {
	cookie, err := r.Cookie(auth.WSAccessTokenCookieName)
	if err != nil {
		return uuid.Nil, "", false
	}

	token := strings.TrimSpace(cookie.Value)
	if token == "" {
		return uuid.Nil, "", false
	}

	claims, err := auth.ParseAccessToken(token)
	if err != nil {
		return uuid.Nil, "", false
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, "", false
	}

	return userID, token, true
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

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		userID, _, ok := mustGetAuthFromRequest(c.Request)
		if !ok {
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(4401, "unauthorized"),
				time.Now().Add(time.Second),
			)
			return
		}

		if _, ok := loadRoomLiveOnly(c, roomID); !ok {
			return
		}

		var user models.User
		if err := database.DB.
			Select("id,name").
			First(&user, "id = ?", userID).Error; err != nil {
			return
		}

		userName := user.Name

		if ChatEventsHub == nil {
			ChatEventsHub = NewChatHub()
		}

		client := &chatClient{conn: conn}
		ChatEventsHub.Add(roomID, client)

		defer func() {
			ChatEventsHub.Remove(roomID, client)
			client.close()
		}()

		key := chatKey(roomID)

		history, err := cache.Client.
			LRange(c, key, 0, chatHistoryMaxKeep-1).
			Result()

		if err == nil {
			for i := len(history) - 1; i >= 0; i-- {
				conn.WriteMessage(websocket.TextMessage, []byte(history[i]))
			}
		}

		conn.SetReadLimit(chatMaxMessageSize)

		_ = conn.SetReadDeadline(time.Now().Add(chatPongWait))

		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(chatPongWait))
		})

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

		for {

			_, b, err := conn.ReadMessage()
			if err != nil {
				<-done
				return
			}

			var in chatIncoming

			if err := json.Unmarshal(b, &in); err != nil {
				continue
			}

			if in.Type != "chat.send" {
				continue
			}

			text := strings.TrimSpace(in.Text)

			if text == "" || len(text) > 500 {
				continue
			}

			msgID := uuid.NewString()
			now := time.Now().Unix()

			ev := chatEvent{
				Type:   "chat.message",
				RoomID: roomID.String(),
				Data: chatMessageData{
					ID:       msgID,
					UserID:   userID.String(),
					UserName: userName,
					Text:     text,
					TS:       now,
				},
				TS: now,
			}

			raw, _ := json.Marshal(ev)

			pipe := cache.Client.Pipeline()

			pipe.LPush(c, key, raw)
			pipe.LTrim(c, key, 0, chatHistoryMaxKeep-1)
			pipe.Expire(c, key, 24*time.Hour)

			_, _ = pipe.Exec(c)

			ChatEventsHub.Broadcast(roomID, ev)
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