package product

import (
	"context"
	"errors"
	"math"
	"time"

	"fmt"
	"net/http"

	// "os"
	// "path/filepath"
	// "strconv"
	"strings"
	// "time"

	// "livecommerce/internal/cache"
	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	// "github.com/gosimple/slug"
)

var ctx = context.Background()



// CreateProduct godoc
// @Summary Create product
// @Description Create a new product. Category must exist and must be a leaf category. Tags are strings; backend will normalize and get-or-create.
// @Tags products
// @Accept json
// @Produce json
// @Param input body CreateProductInput true "Create product input"
// @Success 201 {object} product.ProductResponseDTO
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products [post]
func CreateProduct(c *gin.Context) {
	var input CreateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uidVal, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid, ok := uidVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	tagNames := normalizeTags(input.Tags)

	var product models.Product
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// category must exist and must be leaf
		if err := validateLeafCategory(tx, input.CategoryID); err != nil {
			return err
		}

		tags, err := getOrCreateTags(tx, tagNames)
		if err != nil {
			return err
		}

		product = models.Product{
			Title:       strings.TrimSpace(input.Title),
			Description: strings.TrimSpace(input.Description),
			Price:       input.Price,
			Stock:       input.Stock,
			CoverImage:  strings.TrimSpace(input.CoverImage),
			OwnerID:     uid,
			CategoryID:  input.CategoryID,
			Tags:        tags,
		}

		if err := tx.Create(&product).Error; err != nil {
			return err
		}

		// preload برای response
		return tx.
			Preload("Category").
			Preload("Tags").
			Preload("Owner", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name") }).
			First(&product, "id = ?", product.ID.String()).Error
	})

	if err != nil {
		switch err.Error() {
		case "invalid_category":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category_id"})
		case "category_must_be_leaf":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Category must be a leaf"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
		}
		return
	}

	c.JSON(http.StatusCreated, ToProductResponseDTO(product))
}

// UpdateProductByID godoc
// @Summary Update product
// @Description Update product fields by id (UUID). Only product owner or admin can update. If tags is provided, tags will be replaced fully; if omitted, tags remain unchanged. Category must be a leaf category if provided.
// @Tags products
// @Accept json
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Param input body UpdateProductInput true "Update product input"
// @Success 200 {object} product.ProductResponseDTO
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id} [put]
func UpdateProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	var input UpdateProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uidVal, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid, ok := uidVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)

	var product models.Product

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		p, err := findProductByID(tx, idStr)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}
		product = *p

		if role != "admin" && product.OwnerID != uid {
			return fmt.Errorf("forbidden")
		}

		if input.Title != nil {
			product.Title = strings.TrimSpace(*input.Title)
		}
		if input.Description != nil {
			product.Description = strings.TrimSpace(*input.Description)
		}
		if input.Price != nil {
			product.Price = *input.Price
		}
		if input.Stock != nil {
			product.Stock = *input.Stock
		}
		if input.CoverImage != nil {
			product.CoverImage = strings.TrimSpace(*input.CoverImage)
		}

		if input.CategoryID != nil {
			if err := validateLeafCategory(tx, *input.CategoryID); err != nil {
				return err
			}
			product.CategoryID = *input.CategoryID
		}

		if err := tx.Save(&product).Error; err != nil {
			return err
		}

		if input.Tags != nil {
			names := normalizeTags(*input.Tags)
			tags, err := getOrCreateTags(tx, names)
			if err != nil {
				return err
			}
			if err := tx.Model(&product).Association("Tags").Replace(&tags); err != nil {
				return err
			}
		}

		pp, err := findProductByID(
			tx,
			product.ID.String(),
			"Category",
			"Tags",
			"OwnerSafe",
		)
		if err != nil {
			return err
		}
		product = *pp
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		case err.Error() == "forbidden":
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		case err.Error() == "invalid_id":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		case err.Error() == "invalid_category":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category_id"})
		case err.Error() == "category_must_be_leaf":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Category must be a leaf"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		}
		return
	}

	c.JSON(http.StatusOK, ToProductResponseDTO(product))
}


// GetProductByID godoc
// @Summary Get product by id
// @Description Get product details by UUID. Increments view_count only when requester is NOT admin and NOT the product owner. If unauthenticated, view_count increments.
// @Tags products
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} product.ProductResponseDTO
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /products/{id} [get]
func GetProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))

	product, err := findProductByID(
		database.DB,
		idStr,
		"OwnerSafe",
		"Media",
		"Category",
		"Tags",
	)
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

	uid, role, authed := getAuthInfo(c)

	shouldIncrement := true
	if authed && (role == "admin" || product.OwnerID == uid) {
		shouldIncrement = false
	}

	viewKey := viewsKey(product.ID) 

	if shouldIncrement {
		pipe := cache.Client.Pipeline()
		incrCmd := pipe.Incr(ctx, viewKey)
		pipe.Expire(ctx, viewKey, 7*24*time.Hour) // TTL safety
		_, _ = pipe.Exec(ctx)

		if v, err := incrCmd.Result(); err == nil {
			product.ViewCount = product.ViewCount + v
		}
	} else {
		if v, err := cache.Client.Get(ctx, viewKey).Int64(); err == nil {
			product.ViewCount = product.ViewCount + v
		}
	}

	c.JSON(http.StatusOK, ToProductResponseDTO(*product))
}


// DeleteProductByID godoc
// @Summary Delete product
// @Description Delete a product by id (UUID). Only owner or admin can delete.
// @Tags products
// @Produce json
// @Param id path string true "Product id (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /products/{id} [delete]
func DeleteProductByID(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		return
	}

	uidVal, ok := c.Get("userID")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid, ok := uidVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		p, err := findProductByID(tx, idStr)
		if err != nil {
			if err.Error() == "invalid_id" {
				return err
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		if role != "admin" && p.OwnerID != uid {
			return fmt.Errorf("forbidden")
		}

		if err := tx.Model(p).Association("Tags").Clear(); err != nil {
			return err
		}

		if err := tx.Delete(&models.Product{}, "id = ?", p.ID.String()).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		switch {
		case err.Error() == "invalid_id":
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		case err.Error() == "forbidden":
			c.JSON(http.StatusForbidden, gin.H{"error": "You are not allowed to delete this product"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}










// GetCategoryTree godoc
// @Summary Get categories tree (full depth)
// @Description Returns the full category tree (all levels) as nested children. Root categories have parent_id = null.
// @Tags categories
// @Produce json
// @Success 200 {object} CategoryTreeResponse
// @Failure 500 {object} map[string]string
// @Router /categories/tree [get]
func GetCategoryTree(c *gin.Context) {
	var cats []models.Category
	if err := database.DB.
		Select("id", "key", "name_fa", "parent_id").
		Order("parent_id ASC, id ASC").
		Find(&cats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}

	nodes := make(map[uint]*CategoryNode, len(cats))
	for _, cat := range cats {
		nodes[cat.ID] = &CategoryNode{
			ID:       cat.ID,
			Key:      cat.Key,
			NameFa:   cat.NameFa,
			ParentID: cat.ParentID,
			Children: []*CategoryNode{},
		}
	}

	roots := make([]*CategoryNode, 0)
	for _, cat := range cats {
		node := nodes[cat.ID]
		if node.ParentID == nil {
			roots = append(roots, node)
			continue
		}
		parent := nodes[*node.ParentID]
		if parent == nil {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}

	c.JSON(http.StatusOK, gin.H{"data": roots})
}




// SearchProducts godoc
// @Summary Search products
// @Description Search/filter products by query params with pagination.
// @Tags products
// @Produce json
// @Param q query string false "Search in title/description"
// @Param category_id query int false "Category id"
// @Param min_price query int false "Min price"
// @Param max_price query int false "Max price"
// @Param tags query string false "Comma-separated tags (e.g. apple,iphone)"
// @Param owner_id query string false "Owner id (UUID)"
// @Param in_stock query bool false "Only stock>0"
// @Param sort query string false "newest|price_asc|price_desc|views|likes"
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 50"
// @Success 200 {object} ProductSearchResponseDTO
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /products [get]
func SearchProducts(c *gin.Context) {
    params, err := parseProductSearchParams(c)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    q := buildProductsSearchQuery(database.DB, params)

    // count total
    var total int64
    if err := q.Count(&total).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
        return
    }

    offset := (params.Page - 1) * params.Limit

    var products []models.Product
    if err := q.Limit(params.Limit).Offset(offset).Find(&products).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
        return
    }

    out := make([]ProductListItemDTO, 0, len(products))
    for _, p := range products {
        out = append(out, mapProductToListDTO(p))
    }

    c.JSON(http.StatusOK, ProductSearchResponseDTO{
        Data: out,
        Pagination: PaginationMetaDTO{
            Page:       params.Page,
            Limit:      params.Limit,
            Total:      total,
            TotalPages: int(math.Ceil(float64(total) / float64(params.Limit))),
        },
    })
}




// GetOwnerProducts godoc
// @Summary Get owner products
// @Description Get paginated products for an owner (public).
// @Tags products
// @Produce json
// @Param owner_id path string true "Owner id (UUID)"
// @Param q query string false "Search in title/description"
// @Param category_id query int false "Category id"
// @Param min_price query int false "Min price"
// @Param max_price query int false "Max price"
// @Param tags query string false "Comma-separated tags"
// @Param in_stock query bool false "Only stock>0"
// @Param sort query string false "newest|price_asc|price_desc|views|likes"
// @Param page query int false "default 1"
// @Param limit query int false "default 20, max 50"
// @Success 200 {object} ProductSearchResponseDTO
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /profile/{owner_id}/products [get]
func GetOwnerProducts(c *gin.Context) {
    ownerIDStr := strings.TrimSpace(c.Param("owner_id"))
    oid, err := uuid.Parse(ownerIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid owner_id"})
        return
    }

    params, err := parseProductSearchParams(c)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    params.OwnerID = &oid 

    q := buildProductsSearchQuery(database.DB, params)

    var total int64
    if err := q.Count(&total).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
        return
    }

    offset := (params.Page - 1) * params.Limit

    var products []models.Product
    if err := q.Limit(params.Limit).Offset(offset).Find(&products).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
        return
    }

    out := make([]ProductListItemDTO, 0, len(products))
    for _, p := range products {
        out = append(out, mapProductToListDTO(p))
    }

    c.JSON(http.StatusOK, ProductSearchResponseDTO{
        Data: out,
        Pagination: PaginationMetaDTO{
            Page:       params.Page,
            Limit:      params.Limit,
            Total:      total,
            TotalPages: int(math.Ceil(float64(total) / float64(params.Limit))),
        },
    })
}