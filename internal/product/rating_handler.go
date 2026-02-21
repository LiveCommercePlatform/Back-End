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

var upsertRatingScript = redis.NewScript(redis_scripts.RatingUpsert)
var deleteRatingScript = redis.NewScript(redis_scripts.RatingDelete)



const ratingTTL = 30 * 24 * time.Hour

func expireRatingKeys(pid uuid.UUID) {
	uk := ratingUserKey(pid)
	dk := ratingDistKey(pid)
	mk := ratingMetaKey(pid)

	pipe := cache.Client.Pipeline()
	pipe.Expire(ctx, uk, ratingTTL)
	pipe.Expire(ctx, dk, ratingTTL)
	pipe.Expire(ctx, mk, ratingTTL)
	_, _ = pipe.Exec(ctx)
}

// UpsertProductRating godoc
// @Summary Upsert product rating (1..5)
// @Description Create or update current user's rating for a product (stored in Redis).
// @Tags ratings
// @Accept json
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Param input body UpsertRatingInput true "Rating input"
// @Success 200 {object} RatingUpsertResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/rating [post]
func UpsertProductRating(c *gin.Context) {
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
		c.JSON(http.StatusForbidden, gin.H{"error": "You are the owner, you cannot rate your own product"})
		return
	}

	var input UpsertRatingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Rating < 1 || input.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be between 1 and 5"})
		return
	}

	keys := []string{ratingUserKey(p.ID), ratingDistKey(p.ID), ratingMetaKey(p.ID)}
	res, err := upsertRatingScript.Run(ctx, cache.Client, keys, uid.String(), input.Rating).Result()
	if err != nil {
		if err.Error() == "rating_out_of_range" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be between 1 and 5"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upsert rating"})
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 5 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected rating result"})
		return
	}

	newStr := fmt.Sprint(arr[1])
	countDeltaStr := fmt.Sprint(arr[2])
	sumDeltaStr := fmt.Sprint(arr[3])

	newRating, _ := strconv.Atoi(newStr)
	countDelta, _ := strconv.ParseInt(countDeltaStr, 10, 64)
	sumDelta, _ := strconv.ParseInt(sumDeltaStr, 10, 64)

	// ✅ DB + delta
	totalCount := p.RatingCount + countDelta
	totalSum := p.RatingSum + sumDelta

	avg := 0.0
	if totalCount > 0 {
		avg = float64(totalSum) / float64(totalCount)
	}

	expireRatingKeys(p.ID)

	c.JSON(http.StatusOK, RatingUpsertResponse{
		ProductID:   p.ID.String(),
		MyRating:    newRating,
		RatingAvg:   avg,
		RatingCount: totalCount,
	})
}

// GetMyProductRating godoc
// @Summary Get my rating for a product
// @Tags ratings
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} MyRatingResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/rating/me [get]
func GetMyProductRating(c *gin.Context) {
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

	v, err := cache.Client.HGet(ctx, ratingUserKey(p.ID), uid.String()).Result()
	if err == redis.Nil {
		c.JSON(http.StatusOK, MyRatingResponse{MyRating: 0})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rating"})
		return
	}

	r, _ := strconv.Atoi(v)
	c.JSON(http.StatusOK, MyRatingResponse{MyRating: r})
}

// DeleteProductRating godoc
// @Summary Delete my rating for a product
// @Tags ratings
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} RatingUpsertResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id}/rating [delete]
func DeleteProductRating(c *gin.Context) {
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

	keys := []string{ratingUserKey(p.ID), ratingDistKey(p.ID), ratingMetaKey(p.ID)}
	res, err := deleteRatingScript.Run(ctx, cache.Client, keys, uid.String()).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete rating"})
		return
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) != 4 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unexpected rating result"})
		return
	}

	countDeltaStr := fmt.Sprint(arr[1])
	sumDeltaStr := fmt.Sprint(arr[2])

	countDelta, _ := strconv.ParseInt(countDeltaStr, 10, 64)
	sumDelta, _ := strconv.ParseInt(sumDeltaStr, 10, 64)

	totalCount := p.RatingCount + countDelta
	totalSum := p.RatingSum + sumDelta

	avg := 0.0
	if totalCount > 0 {
		avg = float64(totalSum) / float64(totalCount)
	}


	expireRatingKeys(p.ID)

	c.JSON(http.StatusOK, RatingUpsertResponse{
		ProductID:   p.ID.String(),
		MyRating:    0,
		RatingAvg:   avg,
		RatingCount: totalCount,
	})
}

// GetProductRatingSummary godoc
// @Summary Get product rating summary
// @Description Returns avg/count and breakdown(1..5) from Redis.
// @Tags ratings
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} RatingSummaryResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /products/{id}/rating/summary [get]
func GetProductRatingSummary(c *gin.Context) {
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

	meta, err := cache.Client.HMGet(ctx, ratingMetaKey(p.ID), "count", "sum").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rating meta"})
		return
	}
	countDelta := int64(0)
	sumDelta := int64(0)
	if len(meta) >= 2 {
		if meta[0] != nil {
			countDelta, _ = strconv.ParseInt(fmt.Sprint(meta[0]), 10, 64)
		}
		if meta[1] != nil {
			sumDelta, _ = strconv.ParseInt(fmt.Sprint(meta[1]), 10, 64)
		}
	}

	totalCount := p.RatingCount + countDelta
	totalSum := p.RatingSum + sumDelta

	avg := 0.0
	if totalCount > 0 {
		avg = float64(totalSum) / float64(totalCount)
	}

	distRaw, err := cache.Client.HGetAll(ctx, ratingDistKey(p.ID)).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rating breakdown"})
		return
	}
	breakdown := map[int]int64{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	for k, v := range distRaw {
		ki, e1 := strconv.Atoi(k)
		vi, e2 := strconv.ParseInt(v, 10, 64)
		if e1 == nil && e2 == nil && ki >= 1 && ki <= 5 {
			breakdown[ki] = vi
		}
	}

	expireRatingKeys(p.ID)

	c.JSON(http.StatusOK, RatingSummaryResponse{
		RatingAvg:   avg,
		RatingCount: totalCount,
		Breakdown:   breakdown,
	})
}