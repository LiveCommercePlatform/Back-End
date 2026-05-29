package report

import (
	"net/http"
	"strconv"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ─── CreateReport godoc
// @Summary      Create a report
// @Description  Allows users to report a product, comment, or another user
// @Tags         reports
// @Accept       json
// @Produce      json
// @Param        input body CreateReportInput true "Report information"
// @Success      201 {object} models.Report
// @Failure      400 {object} map[string]string
// @Failure      401 {object} map[string]string
// @Security     BearerAuth
// @Router       /reports [post]
func CreateReport(c *gin.Context) {
	reporterID, ok := mustGetAuth(c)
	if !ok {
		return
	}

	var input CreateReportInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// چک کن که target ID داده شده
	if input.ProductID == nil && input.CommentID == nil && input.TargetUserID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at_least_one_target_required"})
		return
	}

	report := models.Report{
		ReporterID: reporterID,
		Type:       models.ReportType(input.Type),
		Status:     models.ReportStatusNew,
		Reason:     input.Reason,
	}

	if input.ProductID != nil {
		id, _ := uuid.Parse(*input.ProductID)
		report.ProductID = &id
	}
	if input.CommentID != nil {
		report.CommentID = input.CommentID
	}
	if input.TargetUserID != nil {
		id, _ := uuid.Parse(*input.TargetUserID)
		report.TargetUserID = &id
	}

	if err := database.DB.Create(&report).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
		return
	}

	c.JSON(http.StatusCreated, report)
}

// ─── AdminListReports godoc
// @Summary      List reports (admin)
// @Description  Returns paginated reports with optional filters
// @Tags         admin
// @Produce      json
// @Param        type      query string false "Report type: product | comment | user"
// @Param        status    query string false "Report status: new | reviewing | closed"
// @Param        page      query int    false "Page number"
// @Param        page_size query int    false "Items per page"
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/reports [get]
func AdminListReports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	q := database.DB.Model(&models.Report{}).Preload("Reporter")

	if t := c.Query("type"); t != "" {
		q = q.Where("type = ?", t)
	}
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}

	var total int64
	q.Count(&total)

	var reports []models.Report
	q.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&reports)

	c.JSON(http.StatusOK, gin.H{
		"data":      reports,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ─── AdminUpdateReportStatus godoc
// @Summary      Update report status (admin)
// @Description  Updates the status of a report
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        id   path string              true "Report ID"
// @Param        body body UpdateReportStatusInput true "New report status"
// @Success      200 {object} models.Report
// @Failure      400 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/reports/{id}/status [patch]
func AdminUpdateReportStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var input UpdateReportStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var report models.Report
	if err := database.DB.First(&report, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report_not_found"})
		return
	}

	database.DB.Model(&report).Update("status", input.Status)
	c.JSON(http.StatusOK, report)
}

// ─── AdminDeleteReport godoc
// @Summary      Delete report (admin)
// @Description  Deletes a report permanently
// @Tags         admin
// @Produce      json
// @Param        id path int true "Report ID"
// @Success      204
// @Failure      404 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/reports/{id} [delete]
func AdminDeleteReport(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	if err := database.DB.Delete(&models.Report{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete_failed"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ─── AdminBanUserFromReport godoc
// @Summary      Ban user from report (admin)
// @Description  Bans the target user referenced in the report and closes the report
// @Tags         admin
// @Produce      json
// @Param        id path int true "Report ID"
// @Success      200 {object} map[string]string
// @Failure      400 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/reports/{id}/ban-user [post]
func AdminBanUserFromReport(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var report models.Report
	if err := database.DB.First(&report, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report_not_found"})
		return
	}

	if report.TargetUserID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_target_user_in_report"})
		return
	}

	// تغییر role کاربر به banned (یا می‌تونی فیلد is_banned اضافه کنی)
	if err := database.DB.Model(&models.User{}).
		Where("id = ?", report.TargetUserID).
		Update("role", "banned").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ban_failed"})
		return
	}

	// بستن گزارش
	database.DB.Model(&report).Update("status", models.ReportStatusClosed)

	c.JSON(http.StatusOK, gin.H{"message": "user_banned"})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustGetAuth(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.UUID{}, false
	}
	uid, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.UUID{}, false
	}
	return uid, true
}
