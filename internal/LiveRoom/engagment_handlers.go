package liveRoom

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"livecommerce/internal/redis_scripts"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"github.com/redis/go-redis/v9"
)



func viewersZKey(roomID uuid.UUID) string {
	return fmt.Sprintf("live:%s:viewers_z", roomID.String())
}



// ViewPing godoc
// @Summary Viewer ping
// @Description Keep viewer alive for realtime viewer_count (public)
// @Tags liveroom
// @Accept json
// @Produce json
// @Param id path string true "Live room id (uuid)"
// @Param input body liveRoom.ViewPingInput true "viewer key"
// @Success 200 {object} map[string]any
// @Failure 400,404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/view/ping [post]
func ViewPing(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var lr models.LiveRoom
	if err := database.DB.Select("id,status").First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if lr.Status != models.LiveLive {
		c.JSON(http.StatusConflict, gin.H{"error":"live_room_not_live"})
		return
		}

	var in ViewPingInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	viewerKey := strings.TrimSpace(in.ViewerKey)
	if _, err := uuid.Parse(viewerKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_viewer_key"})
		return
	}

	ttlSeconds := 20
	expiresAt := time.Now().Unix() + int64(ttlSeconds)

	key := viewersZKey(roomID)

	ctx := c.Request.Context()

	if err := cache.Client.ZAdd(ctx, key, redis.Z{
		Score:  float64(expiresAt),
		Member: viewerKey,
	}).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}
	cnt, err := getViewerCount(c, roomID, ttlSeconds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"viewer_count": cnt})
}

// LiveRoomStats godoc
// @Summary Live room stats
// @Description Public stats (viewer_count realtime + totals from DB)
// @Tags liveroom
// @Produce json
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/stats [get]
func LiveRoomStats(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var lr models.LiveRoom
	if err := database.DB.Select("id,total_views,total_likes,total_dislikes").
		First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	ttlSeconds := 20
	cnt, err := getViewerCount(c, roomID, ttlSeconds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	c.JSON(200, gin.H{
	"viewer_count": cnt,
	"total_views": lr.TotalViews,
	"total_likes": lr.TotalLikes,
	"total_dislikes": lr.TotalDislikes,
	})
}






func likesKey(roomID uuid.UUID) string {
	return fmt.Sprintf("live:%s:likes", roomID.String())
}
func dislikesKey(roomID uuid.UUID) string {
	return fmt.Sprintf("live:%s:dislikes", roomID.String())
}

var (
	likeScript    = redis.NewScript(redis_scripts.LiveReactionlike)
	dislikeScript = redis.NewScript(redis_scripts.LiveReactionDislike)
	clearScript   = redis.NewScript(redis_scripts.LiveReactionClear)
)


// Like godoc
// @Summary Like a live room
// @Description Auth user likes the live room (removes dislike if exists). Broadcasts realtime counts if room is live.
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} liveRoom.ReactionSummary
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/reactions/like [post]
func Like(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, _, ok := mustGetAuth(c)
	if !ok {
		return
	}

	status, err := loadRoomStatus(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if status != models.LiveLive {
		c.JSON(http.StatusConflict, gin.H{"error":"live_room_not_live"})
		return
		}

	likeCount, dislikeCount, err := runReactionScript(c, likeScript, roomID, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	if status == models.LiveLive {
		publishReactionsIfLive(roomID, likeCount, dislikeCount)
	}

	c.JSON(http.StatusOK, ReactionSummary{Likes: likeCount, Dislikes: dislikeCount})
}

// Dislike godoc
// @Summary Dislike a live room
// @Description Auth user dislikes the live room (removes like if exists). Broadcasts realtime counts if room is live.
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} liveRoom.ReactionSummary
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/reactions/dislike [post]
func Dislike(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, _, ok := mustGetAuth(c)
	if !ok {
		return
	}

	status, err := loadRoomStatus(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

		if status != models.LiveLive {
	c.JSON(http.StatusConflict, gin.H{"error":"live_room_not_live"})
	return
	}

	likeCount, dislikeCount, err := runReactionScript(c, dislikeScript, roomID, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	if status == models.LiveLive {
		publishReactionsIfLive(roomID, likeCount, dislikeCount)
	}

	c.JSON(http.StatusOK, ReactionSummary{Likes: likeCount, Dislikes: dislikeCount})
}

// ClearReaction godoc
// @Summary Clear reaction
// @Description Remove like/dislike for current user. Broadcasts realtime counts if room is live.
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} liveRoom.ReactionSummary
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/reactions [delete]
func ClearReaction(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, _, ok := mustGetAuth(c)
	if !ok {
		return
	}

	status, err := loadRoomStatus(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	if status != models.LiveLive {
		c.JSON(http.StatusConflict, gin.H{"error":"live_room_not_live"})
		return
		}

	likeCount, dislikeCount, err := runReactionScript(c, clearScript, roomID, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	if status == models.LiveLive {
		publishReactionsIfLive(roomID, likeCount, dislikeCount)
	}

	c.JSON(http.StatusOK, ReactionSummary{Likes: likeCount, Dislikes: dislikeCount})
}

// ReactionSummaryHandler godoc
// @Summary Reactions summary
// @Description Public reactions count
// @Tags liveroom
// @Produce json
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} liveRoom.ReactionSummary
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/reactions/summary [get]
func ReactionSummaryHandler(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	// ensure room exists
	if _, err := loadRoomStatus(roomID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	ctx := c.Request.Context()
	likes, err1 := cache.Client.SCard(ctx, likesKey(roomID)).Result()
	dislikes, err2 := cache.Client.SCard(ctx, dislikesKey(roomID)).Result()
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}

	c.JSON(http.StatusOK, ReactionSummary{Likes: likes, Dislikes: dislikes})
}

// MyReaction godoc
// @Summary My reaction
// @Description Get current user's reaction (like/dislike/none)
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/reactions/me [get]
func MyReaction(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, _, ok := mustGetAuth(c)
	if !ok {
		return
	}

	// ensure room exists
	if _, err := loadRoomStatus(roomID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	ctx := c.Request.Context()
	liked, err := cache.Client.SIsMember(ctx, likesKey(roomID), uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}
	if liked {
		c.JSON(http.StatusOK, gin.H{"reaction": "like"})
		return
	}

	disliked, err := cache.Client.SIsMember(ctx, dislikesKey(roomID), uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis_error"})
		return
	}
	if disliked {
		c.JSON(http.StatusOK, gin.H{"reaction": "dislike"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"reaction": "none"})
}