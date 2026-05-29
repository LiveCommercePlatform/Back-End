package message

import (
	"net/http"
	"strconv"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
)

// ─── SendMessage godoc
// @Summary      Send contact message
// @Description  Allows anyone to send a contact/support message
// @Tags         messages
// @Accept       json
// @Produce      json
// @Param        input body SendMessageInput true "Message information"
// @Success      201 {object} models.Message
// @Failure      400 {object} map[string]string
// @Router       /messages [post]
func SendMessage(c *gin.Context) {
	var input SendMessageInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg := models.Message{
		Name:    input.Name,
		Email:   input.Email,
		Content: input.Content,
		Status:  models.MessageUnread,
	}

	if err := database.DB.Create(&msg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "send_failed"})
		return
	}

	c.JSON(http.StatusCreated, msg)
}

// ─── AdminListMessages godoc
// @Summary      List messages (admin)
// @Description  Returns paginated contact messages with optional status filtering
// @Tags         admin
// @Produce      json
// @Param        status    query string false "Message status: unread | read"
// @Param        page      query int    false "Page number"
// @Param        page_size query int    false "Items per page"
// @Success      200 {object} map[string]interface{}
// @Security     BearerAuth
// @Router       /admin/messages [get]
func AdminListMessages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	q := database.DB.Model(&models.Message{})

	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}

	var total int64
	q.Count(&total)

	var msgs []models.Message
	q.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&msgs)

	c.JSON(http.StatusOK, gin.H{
		"data":      msgs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ─── AdminMarkMessageRead godoc
// @Summary      Mark message as read (admin)
// @Description  Updates message status to read
// @Tags         admin
// @Produce      json
// @Param        id path int true "Message ID"
// @Success      200 {object} models.Message
// @Failure      404 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/messages/{id}/read [patch]
func AdminMarkMessageRead(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var msg models.Message
	if err := database.DB.First(&msg, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message_not_found"})
		return
	}

	database.DB.Model(&msg).Update("status", models.MessageRead)
	msg.Status = models.MessageRead
	c.JSON(http.StatusOK, msg)
}

// ─── AdminDeleteMessage godoc
// @Summary      Delete message (admin)
// @Description  Deletes a contact message permanently
// @Tags         admin
// @Produce      json
// @Param        id path int true "Message ID"
// @Success      204
// @Failure      404 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/messages/{id} [delete]
func AdminDeleteMessage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	if err := database.DB.Delete(&models.Message{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}

	c.Status(http.StatusNoContent)
}
