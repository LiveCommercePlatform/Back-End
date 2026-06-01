package admin

import (
	"net/http"
	"strconv"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ─── AdminListUsers godoc
// @Summary      لیست همه کاربران (admin)
// @Tags         admin
// @Produce      json
// @Param        role      query string false "فیلتر role: user | admin | banned"
// @Param        search    query string false "جستجو در نام یا ایمیل"
// @Param        page      query int    false "صفحه"
// @Param        page_size query int    false "تعداد"
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/users [get]
func AdminListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	q := database.DB.Model(&models.User{})

	if role := c.Query("role"); role != "" {
		q = q.Where("role = ?", role)
	}

	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		q = q.Where("name LIKE ? OR email LIKE ?", like, like)
	}

	var total int64
	q.Count(&total)

	var users []models.User
	q.Select("id, name, email, role, verified, phone, address, postal_code, created_at, updated_at").
		Order("created_at DESC").
		Limit(pageSize).Offset(offset).
		Find(&users)

	c.JSON(http.StatusOK, gin.H{
		"data":      users,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ─── AdminGetUser godoc
// @Summary      جزئیات کاربر (admin)
// @Tags         admin
// @Produce      json
// @Param        id path string true "User UUID"
// @Success      200 {object} models.User
// @Failure      404 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/users/{id} [get]
func AdminGetUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var user models.User
	if err := database.DB.
		Select("id, name, email, role, verified, phone, address, postal_code, created_at, updated_at").
		First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// ─── AdminBanUser godoc
// @Summary      بن کردن کاربر (admin)
// @Tags         admin
// @Produce      json
// @Param        id path string true "User UUID"
// @Success      200 {object} map[string]string
// @Failure      400 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/users/{id}/ban [patch]
func AdminBanUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	// نمیشه ادمین رو بن کرد
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}

	if user.Role == models.RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_ban_admin"})
		return
	}

	database.DB.Model(&user).Update("role", "banned")
	c.JSON(http.StatusOK, gin.H{"message": "user_banned"})
}

// ─── AdminUnbanUser godoc
// @Summary      آنبن کردن کاربر (admin)
// @Tags         admin
// @Produce      json
// @Param        id path string true "User UUID"
// @Success      200 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/users/{id}/unban [patch]
func AdminUnbanUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	if err := database.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Update("role", models.RoleUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unban_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user_unbanned"})
}

// ─── AdminDeleteUser godoc
// @Summary      حذف کاربر (admin)
// @Tags         admin
// @Produce      json
// @Param        id path string true "User UUID"
// @Success      204
// @Security     BearerAuth
// @Router       /admin/users/{id} [delete]
func AdminDeleteUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	if err := database.DB.Delete(&models.User{}, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}

	c.Status(http.StatusNoContent)
}

func AdminPromoteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "invalid_id"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(404, gin.H{"error": "user_not_found"})
		return
	}

	if user.Role == models.RoleAdmin {
		c.JSON(200, user)
		return
	}

	if err := database.DB.Model(&user).Update("role", models.RoleAdmin).Error; err != nil {
		c.JSON(500, gin.H{"error": "update_failed"})
		return
	}

	user.Role = models.RoleAdmin
	c.JSON(200, user)
}

func AdminDemoteUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, gin.H{"error": "invalid_id"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(404, gin.H{"error": "user_not_found"})
		return
	}

	if user.Role == models.RoleUser {
		c.JSON(200, user)
		return
	}

	if err := database.DB.Model(&user).Update("role", models.RoleUser).Error; err != nil {
		c.JSON(500, gin.H{"error": "update_failed"})
		return
	}

	user.Role = models.RoleUser
	c.JSON(200, user)
}
