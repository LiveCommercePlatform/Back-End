package product

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/redis_scripts"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
    likeScript      = redis.NewScript(redis_scripts.EngagementLike)
    unlikeScript    = redis.NewScript(redis_scripts.EngagementUnlike)
    dislikeScript   = redis.NewScript(redis_scripts.EngagementDislike)
    undislikeScript = redis.NewScript(redis_scripts.EngagementUndislike)
)

const engagementTTL = 30 * 24 * time.Hour

func expireEngagementKeys(pid uuid.UUID) {
	lk := likesKey(pid)
	dk := dislikesKey(pid)
	mk := engageMetaKey(pid)

	pipe := cache.Client.Pipeline()
	pipe.Expire(ctx, lk, engagementTTL)
	pipe.Expire(ctx, dk, engagementTTL)
	pipe.Expire(ctx, mk, engagementTTL)
	_, _ = pipe.Exec(ctx)
}




// LikeProductByID godoc
// @Summary Like a product
// @Description Like a product (adds user to likes set). If user had disliked, it will be removed.
// @Tags engagement
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/like [post]
func LikeProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))

	uid, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		if err.Error() == "invalid_id" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	if p.OwnerID == uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are the owner, you cannot like your own product"})
		return
	}

	keys := []string{likesKey(p.ID), dislikesKey(p.ID), engageMetaKey(p.ID)}
	res, err := likeScript.Run(ctx, cache.Client, keys, uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to like product"})
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 3 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected like result"})
		return
	}

	changed := fmt.Sprint(arr[2]) == "1"
	if !changed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have already liked this product"})
		return
	}

	expireEngagementKeys(p.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Product liked"})
}



// UnlikeProductByID godoc
// @Summary Unlike a product
// @Description Remove like (removes user from likes set)
// @Tags engagement
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/like [delete]
func UnlikeProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))

	uid, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		if err.Error() == "invalid_id" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	if p.OwnerID == uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are the owner, you cannot unlike your own product"})
		return
	}

	keys := []string{likesKey(p.ID), engageMetaKey(p.ID)}
	res, err := unlikeScript.Run(ctx, cache.Client, keys, uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove like"})
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 3 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected unlike result"})
		return
	}

	removed := fmt.Sprint(arr[2]) == "1"
	if !removed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have not liked this product"})
		return
	}

	expireEngagementKeys(p.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Like removed"})
}


// DisLikeProductByID godoc
// @Summary Dislike a product
// @Description Dislike a product (adds user to dislikes set). If user had liked, it will be removed.
// @Tags engagement
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/dislike [post]
func DisLikeProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))

	uid, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		if err.Error() == "invalid_id" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	if p.OwnerID == uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are the owner, you cannot dislike your own product"})
		return
	}

	keys := []string{likesKey(p.ID), dislikesKey(p.ID), engageMetaKey(p.ID)}
	res, err := dislikeScript.Run(ctx, cache.Client, keys, uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to dislike product"})
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 3 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected dislike result"})
		return
	}

	changed := fmt.Sprint(arr[2]) == "1"
	if !changed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have already disliked this product"})
		return
	}

	expireEngagementKeys(p.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Product disliked"})
}




// UndislikeProductByID godoc
// @Summary Undislike a product
// @Description Remove dislike (removes user from dislikes set)
// @Tags engagement
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/dislike [delete]
func UndislikeProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))

	uid, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		if err.Error() == "invalid_id" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	if p.OwnerID == uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are the owner, you cannot undislike your own product"})
		return
	}

	keys := []string{dislikesKey(p.ID), engageMetaKey(p.ID)}
	res, err := undislikeScript.Run(ctx, cache.Client, keys, uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove dislike"})
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 3 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected undislike result"})
		return
	}

	removed := fmt.Sprint(arr[2]) == "1"
	if !removed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You have not disliked this product"})
		return
	}

	expireEngagementKeys(p.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Dislike removed"})
}



// GetProductStatisticsByID godoc
// @Summary Get product engagement statistics
// @Description Retrieve likes, dislikes, views, and engagement count for a product
// @Tags statistics
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} ProductStatsDTO
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /products/{id}/stats [get]
func GetProductStatisticsByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		if err.Error() == "invalid_id" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	meta, err := cache.Client.HMGet(ctx, engageMetaKey(p.ID), "likes_count", "dislikes_count").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch engagement meta"})
		return
	}

	likesDelta := int64(0)
	dislikesDelta := int64(0)

	if len(meta) >= 2 {
		if meta[0] != nil {
			likesDelta, _ = strconv.ParseInt(fmt.Sprint(meta[0]), 10, 64)
		}
		if meta[1] != nil {
			dislikesDelta, _ = strconv.ParseInt(fmt.Sprint(meta[1]), 10, 64)
		}
	}

	// ✅ DB + Delta
	likesTotal := p.LikeCount + likesDelta
	dislikesTotal := p.DislikeCount + dislikesDelta

	viewDelta, err := cache.Client.Get(ctx, viewsKey(p.ID)).Int64()
	if err != nil {
		viewDelta = 0
	}
	viewsTotal := p.ViewCount + viewDelta

	c.JSON(http.StatusOK, ProductStatsDTO{
		Likes:      likesTotal,
		Dislikes:   dislikesTotal,
		Views:      viewsTotal,
		Engagement: likesTotal + dislikesTotal,
	})
}


// GetMyProductEngagement godoc
// @Summary Get my engagement for a product
// @Description Returns whether the current authenticated user has liked or disliked the product.
// @Tags engagement
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} MyEngagementDTO
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/engagement/me [get]
func GetMyProductEngagement(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	uid, _, ok := mustGetAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	p, err := findProductByID(database.DB, idStr)
	if err != nil {
		if err.Error() == "invalid_id" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch product"})
		return
	}

	liked, err := cache.Client.SIsMember(ctx, likesKey(p.ID), uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check like status"})
		return
	}

	disliked, err := cache.Client.SIsMember(ctx, dislikesKey(p.ID), uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check dislike status"})
		return
	}

	c.JSON(http.StatusOK, MyEngagementDTO{
		Liked:    liked,
		Disliked: disliked,
	})
}