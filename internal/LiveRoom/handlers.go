package liveRoom

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var EventsHub *RoomHub


// CreateLiveRoom godoc
// @Summary Create live room
// @Description Create a new live room (product can be attached later)
// @Tags liveroom
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param input body liveRoom.CreateLiveRoomInput true "Live room data"
// @Success 201 {object} models.LiveRoom
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms [post]
func CreateLiveRoom(c *gin.Context) {
	var input CreateLiveRoomInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, _, ok := mustGetAuth(c)
	if !ok {
		return
	}

	lr := models.LiveRoom{
		HostID:      uid,
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Status:      models.LiveScheduled,
		IsRecorded:  input.IsRecorded,
	}

	if err := database.DB.Create(&lr).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_create_live_room"})
		return
	}

	c.JSON(http.StatusCreated, lr)
}

// ListLiveRooms godoc
// @Summary List live rooms
// @Description List live rooms with optional status filter
// @Tags liveroom
// @Produce json
// @Param status query string false "scheduled | live | ended"
// @Success 200 {array} models.LiveRoom
// @Failure 500 {object} map[string]string
// @Router /live-rooms [get]
func ListLiveRooms(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	hostID := strings.TrimSpace(c.Query("host_id"))

	q := database.DB.
		Model(&models.LiveRoom{}).
		Preload("Host").
		Order("created_at desc")
	if status != "" {
		q = q.Where("status = ?", status)
	}

	if hostID != "" {
		q = q.Where("host_id = ?", hostID)
	}

	var rooms []models.LiveRoom
	if err := q.Find(&rooms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	c.JSON(http.StatusOK, rooms)
}

// GetLiveRoomByID godoc
// @Summary Get live room
// @Description Get live room details by id
// @Tags liveroom
// @Produce json
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} models.LiveRoom
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /live-rooms/{id} [get]
func GetLiveRoomByID(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	var lr models.LiveRoom
	err := database.DB.
		Preload("Host").
		Preload("Products").
		First(&lr, "id = ?", id).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	c.JSON(http.StatusOK, lr)
}

// UpdateLiveRoom godoc
// @Summary Update live room
// @Description Update title/description/is_recorded (owner or admin)
// @Tags liveroom
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Param input body liveRoom.UpdateLiveRoomInput true "Update data"
// @Success 200 {object} models.LiveRoom
// @Failure 400,401,403,404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id} [patch]
func UpdateLiveRoom(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, role, ok := mustGetAuth(c)
	if !ok {
		return
	}

	var lr models.LiveRoom
	if err := database.DB.First(&lr, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	if lr.HostID != uid && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var input UpdateLiveRoomInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Title != nil {
		lr.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		lr.Description = strings.TrimSpace(*input.Description)
	}
	if input.IsRecorded != nil {
		lr.IsRecorded = *input.IsRecorded
	}

	if err := database.DB.Save(&lr).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_update_live_room"})
		return
	}

	c.JSON(http.StatusOK, lr)
}

// DeleteLiveRoom godoc
// @Summary Delete live room
// @Description Delete a live room (owner or admin). Cannot delete while live.
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 204 {string} string "No Content"
// @Failure 401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id} [delete]
func DeleteLiveRoom(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, role, ok := mustGetAuth(c)
	if !ok {
		return
	}

	var lr models.LiveRoom
	if err := database.DB.First(&lr, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	if lr.HostID != uid && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if lr.Status == models.LiveLive {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot_delete_while_live"})
		return
	}

	if err := database.DB.Delete(&models.LiveRoom{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_delete_live_room"})
		return
	}

	c.Status(http.StatusNoContent)
}

// StartLive godoc
// @Summary Start live room
// @Description Start a scheduled live room (owner or admin)
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} map[string]string
// @Failure 401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/start [post]
func StartLive(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, role, ok := mustGetAuth(c)
	if !ok {
		return
	}

	var lr models.LiveRoom
	if err := database.DB.First(&lr, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	if lr.HostID != uid && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if lr.Status != models.LiveScheduled {
		c.JSON(http.StatusConflict, gin.H{"error": "room_not_scheduled"})
		return
	}

	now := time.Now()
	if err := database.DB.Model(&lr).
		Where("status = ?", models.LiveScheduled).
		Updates(map[string]any{
			"status":     models.LiveLive,
			"started_at": &now,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_start_live"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "live_started"})
}

// EndLive godoc
// @Summary End live room
// @Description End a live room (owner or admin)
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "Live room id (uuid)"
// @Success 200 {object} map[string]string
// @Failure 401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/end [post]
func EndLive(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	uid, role, ok := mustGetAuth(c)
	if !ok {
		return
	}

	var lr models.LiveRoom
	if err := database.DB.First(&lr, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	if lr.HostID != uid && !isAdmin(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if lr.Status != models.LiveLive {
		c.JSON(http.StatusConflict, gin.H{"error": "room_not_live"})
		return
	}

	now := time.Now()
	var duration int64
	if lr.StartedAt != nil {
		duration = int64(now.Sub(*lr.StartedAt).Seconds())
	}

	ctx := c.Request.Context()
	likes, _ := cache.Client.SCard(ctx, likesKey(lr.ID)).Result()
	dislikes, _ := cache.Client.SCard(ctx, dislikesKey(lr.ID)).Result()

	if err := database.DB.Model(&lr).Updates(map[string]any{
		"status":   models.LiveEnded,
		"ended_at": &now,
		"duration": duration,
		"total_likes":   likes,    
    	"total_dislikes": dislikes,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_end_live"})
		return
	}
	cache.Client.Del(ctx, likesKey(lr.ID), dislikesKey(lr.ID))
	DestroySFURoom(id)
	c.JSON(http.StatusOK, gin.H{"message": "live_ended"})
}




// AttachProducts godoc
// @Summary Attach products to live room
// @Description Attach products (allowed in scheduled and live). Reject if any product already attached.
// @Tags liveroom
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "LiveRoom ID (uuid)"
// @Param input body liveRoom.AttachProductsInput true "Product IDs"
// @Success 200 {object} map[string]any
// @Failure 400,401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/products [post]
func AttachProducts(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	lr, err := findLiveRoomByID(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	if !requireManageRoom(c, lr) {
		return
	}
	if !ensureRoomNotEnded(c, lr) {
		return
	}

	var input AttachProductsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reqIDs := make([]uuid.UUID, 0, len(input.ProductIDs))
	seen := map[uuid.UUID]struct{}{}
	for _, raw := range input.ProductIDs {
		raw = strings.TrimSpace(raw)
		pid, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_product_id"})
			return
		}
		if _, exists := seen[pid]; exists {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_product_id_in_request"})
			return
		}
		seen[pid] = struct{}{}
		reqIDs = append(reqIDs, pid)
	}

	var existingProducts int64
	if err := database.DB.Model(&models.Product{}).
		Where("id IN ?", reqIDs).
		Count(&existingProducts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if existingProducts != int64(len(reqIDs)) {
		c.JSON(http.StatusNotFound, gin.H{"error": "one_or_more_products_not_found"})
		return
	}

	// check already attached (so we can reject duplicates, as you requested)
	var alreadyAttached []uuid.UUID
	if err := database.DB.Model(&models.LiveRoomProduct{}).
		Where("live_room_id = ? AND product_id IN ?", lr.ID, reqIDs).
		Pluck("product_id", &alreadyAttached).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if len(alreadyAttached) > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error": "product_already_attached",
			"product_ids": alreadyAttached,
		})
		return
	}

	recs := make([]models.LiveRoomProduct, 0, len(reqIDs))
	for _, pid := range reqIDs {
		recs = append(recs, models.LiveRoomProduct{
			LiveRoomID: lr.ID,
			ProductID:  pid,
		})
	}

	if err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "live_room_id"}, {Name: "product_id"}},
		DoNothing: true,
	}).Create(&recs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if lr.Status == models.LiveLive && EventsHub != nil {
    publishProductsUpdate(EventsHub, lr.ID, "attached")
	}

	c.JSON(http.StatusOK, gin.H{"message": "products_attached"})
}

// ListAttachedProducts godoc
// @Summary List live room products
// @Tags liveroom
// @Produce json
// @Param id path string true "LiveRoom ID (uuid)"
// @Success 200 {array} models.LiveRoomProduct
// @Failure 400,404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/products [get]
func ListAttachedProducts(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	// ensure room exists
	var lr models.LiveRoom
	if err := database.DB.Select("id").First(&lr, "id = ?", roomID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	var items []models.LiveRoomProduct
	if err := database.DB.
		Where("live_room_id = ?", roomID).
		Order("is_pinned desc, sort_order asc, id asc").
		Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}

	c.JSON(http.StatusOK, items)
}

// DetachProduct godoc
// @Summary Detach product from live room
// @Tags liveroom
// @Produce json
// @Security BearerAuth
// @Param id path string true "LiveRoom ID (uuid)"
// @Param productId path string true "Product ID (uuid)"
// @Success 204 {string} string "No Content"
// @Failure 400,401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/products/{productId} [delete]
func DetachProduct(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	productID, ok := parseUUIDParam(c, "productId")
	if !ok {
		return
	}

	lr, err := findLiveRoomByID(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if !requireManageRoom(c, lr) {
		return
	}
	if !ensureRoomNotEnded(c, lr) {
		return
	}

	res := database.DB.Delete(&models.LiveRoomProduct{}, "live_room_id = ? AND product_id = ?", roomID, productID)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "product_not_attached"})
		return
	}
	if lr.Status == models.LiveLive && EventsHub != nil {
    publishProductsUpdate(EventsHub, lr.ID, "detached")
	}
	c.Status(http.StatusNoContent)
}

// PinProduct godoc
// @Summary Pin/unpin a product in live room
// @Tags liveroom
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "LiveRoom ID (uuid)"
// @Param productId path string true "Product ID (uuid)"
// @Param input body liveRoom.PinProductInput true "Pin toggle"
// @Success 200 {object} map[string]any
// @Failure 400,401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/products/{productId}/pin [patch]
func PinProduct(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	productID, ok := parseUUIDParam(c, "productId")
	if !ok {
		return
	}

	lr, err := findLiveRoomByID(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if !requireManageRoom(c, lr) {
		return
	}
	if !ensureRoomNotEnded(c, lr) {
		return
	}

	var input PinProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res := database.DB.Model(&models.LiveRoomProduct{}).
		Where("live_room_id = ? AND product_id = ?", roomID, productID).
		Update("is_pinned", input.IsPinned)

	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "product_not_attached"})
		return
	}

	if lr.Status == models.LiveLive && EventsHub != nil {
    publishProductsUpdate(EventsHub, lr.ID, "pinned")
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// ReorderProducts godoc
// @Summary Reorder products in live room
// @Tags liveroom
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "LiveRoom ID (uuid)"
// @Param input body liveRoom.ReorderProductsInput true "Reorder data"
// @Success 200 {object} map[string]any
// @Failure 400,401,403,404,409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /live-rooms/{id}/products/reorder [patch]
func ReorderProducts(c *gin.Context) {
	roomID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	lr, err := findLiveRoomByID(roomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "live_room_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if !requireManageRoom(c, lr) {
		return
	}
	if !ensureRoomNotEnded(c, lr) {
		return
	}

	var input ReorderProductsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, it := range input.Items {
		pid, err := uuid.Parse(strings.TrimSpace(it.ProductID))
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_product_id"})
			return
		}

		res := tx.Model(&models.LiveRoomProduct{}).
			Where("live_room_id = ? AND product_id = ?", roomID, pid).
			Update("sort_order", it.SortOrder)

		if res.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
			return
		}
		if res.RowsAffected == 0 {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"error": "product_not_attached"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
		return
	}
	if lr.Status == models.LiveLive && EventsHub != nil {
    publishProductsUpdate(EventsHub, lr.ID, "reordered")
	}

	c.JSON(http.StatusOK, gin.H{"message": "reordered"})
}