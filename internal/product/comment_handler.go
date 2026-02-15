package product

import (
	"errors"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetProductCommentsByID godoc
// @Summary Get product comments
// @Description Get comments for a product with pagination
// @Tags products
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Param limit query int false "Number of comments (default 20, max 50)"
// @Param page query int false "Page number (default 1)"
// @Success 200 {object} PaginatedCommentsResponseDTO
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /products/{id}/comments [get]
func GetProductCommentsByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	limit := 20
	page := 1

	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil {
		if l > 0 && l <= 50 {
			limit = l
		}
	}
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}

	offset := (page - 1) * limit

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		switch {
		case err.Error() == "invalid_id":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		}
		return
	}

	var comments []models.Comment
	var total int64

	if err := database.DB.Model(&models.Comment{}).
		Where("product_id = ?", p.ID.String()).
		Count(&total).Error; err != nil {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count comments"})
	return
	}


	if err := database.DB.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "name")
		}).
		Where("product_id = ?", p.ID.String()).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	dtos := make([]CommentResponseDTO, 0, len(comments))
	for _, cmt := range comments {
		dtos = append(dtos, mapCommentToDTO(cmt))
	}

	c.JSON(http.StatusOK, PaginatedCommentsResponseDTO{
		Data: dtos,
		Pagination: PaginationMetaDTO{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: int(math.Ceil(float64(total) / float64(limit))),
		},
	})

}




// CreateComment godoc
// @Summary Add a comment to a product
// @Description Adds a new comment to a specific product by product id (UUID).
// @Tags comments
// @Accept json
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Param input body CreateCommentInput true "Comment content"
// @Success 201 {object} models.Comment
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/comments [post]
func CreateComment(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product id"})
		return
	}

	var input CreateCommentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		switch {
		case err.Error() == "invalid_id":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product id"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		}
		return
	}

	comment := models.Comment{
		UserID:    userID,
		ProductID: p.ID, 
		Content:   strings.TrimSpace(input.Content),
	}

	if err := database.DB.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	c.JSON(http.StatusCreated, comment)
}


// UpdateComment godoc
// @Summary Update a comment
// @Description Updates an existing comment. Only the comment owner or an admin can perform this action.
// @Tags comments
// @Accept json
// @Produce json
// @Param id path uint true "Comment ID"
// @Param input body UpdateCommentInput true "Comment content"
// @Success 200 {object} models.Comment
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /comments/{id} [put]
func UpdateComment(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	idU64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || idU64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}
	id := uint(idU64)

	userID, role, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var comment models.Comment
	if err := database.DB.First(&comment, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}

	if role != "admin" && comment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not allowed to update this comment"})
		return
	}

	var input UpdateCommentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	comment.Content = strings.TrimSpace(input.Content)

	if err := database.DB.Save(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update comment"})
		return
	}

	c.JSON(http.StatusOK, comment)
}



// DeleteComment godoc
// @Summary Delete a comment
// @Description Deletes a comment based on its ID. Only the comment owner or an admin can perform this action.
// @Tags comments
// @Produce json
// @Param id path uint true "Comment ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /comments/{id} [delete]
func DeleteComment(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	idU64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || idU64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
		return
	}
	id := uint(idU64)

	userID, role, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var comment models.Comment
	if err := database.DB.First(&comment, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}

	if role != "admin" && comment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not allowed to delete this comment"})
		return
	}

	if err := database.DB.Delete(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted successfully"})
}
