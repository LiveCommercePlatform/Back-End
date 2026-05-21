package liveRoom

import (
	"encoding/json"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// var signalingUpgrader = websocket.Upgrader{
// 	CheckOrigin: func(r *http.Request) bool {
// 		return true
// 	},
// }


// WSSignaling godoc
// @Summary LiveRoom signaling websocket (WebRTC/SFU)
// @Description WebRTC signaling endpoint
// @Tags liveroom
// @Param id path string true "LiveRoom ID (uuid)"
// @Success 101 {string} string "Switching Protocols"
// @Router /ws/live-rooms/{id}/signaling [get]
func WSWebRTCSignaling(c *gin.Context) {

	// roomIDParam := c.Param("roomID")

	// roomID, err := uuid.Parse(roomIDParam)
	roomID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	userID, _, ok := mustGetAuth(c)
    if !ok {
        return
    }

	var lr models.LiveRoom
    if err := database.DB.Select("id,status,host_id").
        First(&lr, "id = ?", roomID).Error; err != nil {
        c.AbortWithStatus(http.StatusNotFound)
        return
    }
    if lr.Status != models.LiveLive {
        c.AbortWithStatus(http.StatusConflict)
        return
    }

	conn, err := upgrader.Upgrade(
		c.Writer,
		c.Request,
		nil,
	)
	if err != nil {
		return
	}

	client := NewWSClient(conn)

	go client.WritePump()

	room := getOrCreateSFURoom(roomID)

	session := NewSignalingSession(
		client,
		room,
	)
	session.UserID = &userID      
    session.RoomID = roomID       
    session.HostID = lr.HostID 

	defer session.Cleanup()


client.ReadPump(func(
    messageType int,
    data []byte,
) {

	if messageType != websocket.TextMessage {
		return
	}

	var msg SignalMessage

	if err := json.Unmarshal(
		data,
		&msg,
	); err != nil {
		return
	}

	session.Touch()

	switch msg.Type {

	case "join":
		handleJoin(session, msg)

	case "offer":
		handleOffer(session, msg)

	case "answer":
		handleAnswer(session, msg)

	case "ice_candidate":
		handleICECandidate(session, msg)

	case "leave":
		handleLeave(session)
		return
	}
})
}