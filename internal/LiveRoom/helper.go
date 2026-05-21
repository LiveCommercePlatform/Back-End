package liveRoom

import (
	"context"
	"fmt"
	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func redisCtx(c *gin.Context) context.Context {
	// بهترین گزینه: با lifecycle request هماهنگ میشه (cancel/timeout)
	return c.Request.Context()
}

func mustGetAuth(c *gin.Context) (uuid.UUID, string, bool) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.UUID{}, "", false
	}
	uid, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return uuid.UUID{}, "", false
	}
	role := c.GetString("role")
	return uid, role, true
}

func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	raw := strings.TrimSpace(c.Param(name))
	id, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return uuid.UUID{}, false
	}
	return id, true
}

func isAdmin(role string) bool {
	return role == "admin"
}

func findLiveRoomByID(roomID uuid.UUID) (*models.LiveRoom, error) {
	var lr models.LiveRoom
	if err := database.DB.First(&lr, "id = ?", roomID).Error; err != nil {
		return nil, err
	}
	return &lr, nil
}

func requireManageRoom(c *gin.Context, lr *models.LiveRoom) bool {
	uid, role, ok := mustGetAuth(c)
	if !ok {
		return false
	}
	if lr.HostID == uid || isAdmin(role) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	return false
}

// policy: attach/pin/reorder/detach in scheduled + live, not in ended
func ensureRoomNotEnded(c *gin.Context, lr *models.LiveRoom) bool {
	if lr.Status == models.LiveEnded {
		c.JSON(http.StatusConflict, gin.H{"error": "room_ended"})
		return false
	}
	return true
}

func getViewerCount(c *gin.Context, roomID uuid.UUID, ttlSeconds int) (int64, error) {
	ctx := redisCtx(c)

	key := viewersZKey(roomID)
	now := time.Now().Unix()

	// چون score = expiresAt هست، هر چیزی که expiresAt <= now باشه یعنی منقضی شده
	if err := cache.Client.ZRemRangeByScore(
		ctx,
		key,
		"-inf",
		fmt.Sprintf("%d", now),
	).Err(); err != nil {
		return 0, err
	}

	cnt, err := cache.Client.ZCard(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	_ = cache.Client.Expire(ctx, key, time.Duration(ttlSeconds*10)*time.Second).Err()
	return cnt, nil
}

func loadRoomStatus(roomID uuid.UUID) (models.LiveStatus, error) {
	var lr models.LiveRoom
	err := database.DB.Select("id,status").First(&lr, "id = ?", roomID).Error
	if err != nil {
		return "", err
	}
	return lr.Status, nil
}

func runReactionScript(c *gin.Context, script *redis.Script, roomID uuid.UUID, userID uuid.UUID) (int64, int64, error) {
	ctx := redisCtx(c)

	keys := []string{likesKey(roomID), dislikesKey(roomID)}
	res, err := script.Run(ctx, cache.Client, keys, userID.String()).Result()
	if err != nil {
		return 0, 0, err
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 2 {
		return 0, 0, fmt.Errorf("unexpected_script_result")
	}

	likeCount, ok1 := arr[0].(int64)
	dislikeCount, ok2 := arr[1].(int64)
	if !ok1 || !ok2 {
		return 0, 0, fmt.Errorf("unexpected_script_result_type")
	}

	return likeCount, dislikeCount, nil
}

func publishReactionsIfLive(roomID uuid.UUID, likes int64, dislikes int64) {
	if EventsHub == nil {
		return
	}

	ev := LiveRoomEvent{
		Type:   "live_room.reactions.updated",
		RoomID: roomID.String(),
		Data: ReactionSummary{
			Likes:    likes,
			Dislikes: dislikes,
		},
		TS: time.Now().Unix(),
	}

	EventsHub.Broadcast(roomID, ev)
}

func generateViewerID() string {
	return uuid.New().String()
}
