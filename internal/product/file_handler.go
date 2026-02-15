package product

import (
	"errors"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UploadMediaByProductID godoc
// @Summary Upload media (image or video) to a product
// @Description Uploads an image or video file to a product. Only the product owner can upload media.
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Param file formData file true "Media file (image or video)"
// @Success 201 {object} models.ProductMedia
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/media [post]
func UploadMediaByProductID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product id"})
		return
	}

	uid, _, ok := mustGetAuth(c)
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

	if p.OwnerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not allowed to upload media for this product"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}

	// size limit 10MB
	if file.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size exceeds 10MB"})
		return
	}

	// --- Detect content-type from file bytes ---
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file"})
		return
	}
	defer f.Close()

	header := make([]byte, 512)
	n, _ := f.Read(header)
	contentType := http.DetectContentType(header[:n])

	var mediaType models.MediaType
	var ext string

	// Images
	switch contentType {
	case "image/jpeg":
		mediaType, ext = models.MediaImage, ".jpg"
	case "image/png":
		mediaType, ext = models.MediaImage, ".png"
	case "image/webp":
		mediaType, ext = models.MediaImage, ".webp"
	// Videos (common)
	case "video/mp4":
		mediaType, ext = models.MediaVideo, ".mp4"
	case "video/quicktime":
		mediaType, ext = models.MediaVideo, ".mov"
	case "video/x-msvideo":
		mediaType, ext = models.MediaVideo, ".avi"
	default:
		rawExt := strings.ToLower(filepath.Ext(file.Filename))
		switch rawExt {
	case ".jpg", ".jpeg", ".png", ".webp":
				mediaType = models.MediaImage
				if rawExt == ".jpeg" {
					ext = ".jpg"
				} else {
					ext = rawExt
				}
			case ".mp4", ".mov", ".avi":
				mediaType = models.MediaVideo
				ext = rawExt
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported file type"})
				return
			}
		}

	// --- Save in product folder: uploads/products/{productID}/ ---
	dir := filepath.Join("uploads", "products", p.ID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare upload directory"})
		return
	}

	filename := uuid.New().String() + ext
	diskPath := filepath.Join(dir, filename)

	if err := c.SaveUploadedFile(file, diskPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	urlPath := "/uploads/products/" + p.ID.String() + "/" + filename

	media := models.ProductMedia{
		ProductID: p.ID,
		URL:       urlPath,
		Type:      mediaType,
	}

	if err := database.DB.Create(&media).Error; err != nil {
		_ = os.Remove(diskPath) 
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save media"})
		return
	}

	c.JSON(http.StatusCreated, media)
}



// DeleteMedia godoc
// @Summary Delete media
// @Description Deletes a media file by ID. Only the product owner can delete media.
// @Tags media
// @Produce json
// @Param id path uint true "Media ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /media/{id} [delete]
func DeleteMedia(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	idU64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || idU64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid media ID"})
		return
	}
	id := uint(idU64)

	uid, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var media models.ProductMedia
	if err := database.DB.First(&media, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Media not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch media"})
		return
	}

	// owner check
	var p models.Product
	if err := database.DB.Select("id", "owner_id").First(&p, "id = ?", media.ProductID.String()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	if p.OwnerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not allowed to delete this media"})
		return
	}

	// delete db record first
	if err := database.DB.Delete(&media).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete media record"})
		return
	}

	// delete disk file best-effort
	// media.URL should look like: /uploads/products/{pid}/{filename}
	rel := strings.TrimPrefix(media.URL, "/uploads/")
	diskPath := filepath.Join("uploads", rel) // uploads/products/{pid}/{filename}
	_ = os.Remove(diskPath)

	c.JSON(http.StatusOK, gin.H{"message": "Media deleted successfully"})
}