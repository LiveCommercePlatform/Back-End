package product

import (
	"errors"
	"fmt"
	"livecommerce/internal/models"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func normalizeTags(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))

	for _, t := range raw {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if len(t) < 2 || len(t) > 24 {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)

		if len(out) >= 10 {
			break
		}
	}
	return out
}

func validateLeafCategory(tx *gorm.DB, categoryID uint) error {
	var cat models.Category
	if err := tx.Select("id").First(&cat, categoryID).Error; err != nil {
		return fmt.Errorf("invalid_category")
	}

	var childrenCount int64
	if err := tx.Model(&models.Category{}).
		Where("parent_id = ?", categoryID).
		Count(&childrenCount).Error; err != nil {
		return err
	}
	if childrenCount > 0 {
		return fmt.Errorf("category_must_be_leaf")
	}
	return nil
}

func getOrCreateTags(tx *gorm.DB, names []string) ([]models.Tag, error) {
	if len(names) == 0 {
		return []models.Tag{}, nil
	}

	var existing []models.Tag
	if err := tx.Where("name IN ?", names).Find(&existing).Error; err != nil {
		return nil, err
	}
	existingMap := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		existingMap[t.Name] = struct{}{}
	}

	// 2) create missing (race-safe)
	for _, name := range names {
		if _, ok := existingMap[name]; ok {
			continue
		}
		nt := models.Tag{Name: name}
		_ = tx.Create(&nt).Error
		// اگر unique خورد، ignore می‌کنیم و بعداً دوباره fetch می‌کنیم
	}

	// 3) fetch again (final set)
	var tags []models.Tag
	if err := tx.Where("name IN ?", names).Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}



func findProductByID(tx *gorm.DB, idStr string, preloads ...string) (*models.Product, error) {
	idStr = strings.TrimSpace(idStr)
	pid, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid_id")
	}

	q := tx.Model(&models.Product{})
	for _, p := range preloads {
		switch p {
		case "Owner", "OwnerSafe":
			q = q.Preload("Owner", func(db *gorm.DB) *gorm.DB {
				return db.Select("id", "name")
			})
		default:
			q = q.Preload(p)
		}
	}

	var product models.Product
	if err := q.Where("id = ?", pid.String()).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}

	return &product, nil
}




func getAuthInfo(c *gin.Context) (uid uuid.UUID, role string, ok bool) {
	v, exists := c.Get("userID")
	if !exists {
		return uuid.UUID{}, "", false
	}
	u, ok1 := v.(uuid.UUID)
	if !ok1 {
		return uuid.UUID{}, "", false
	}
	rv, _ := c.Get("role")
	rs, _ := rv.(string)
	return u, rs, true
}

func mustGetAuth(c *gin.Context) (uuid.UUID, string, bool) {
	uidVal, ok := c.Get("userID")
	if !ok {
		return uuid.UUID{}, "", false
	}
	uid, ok := uidVal.(uuid.UUID)
	if !ok {
		return uuid.UUID{}, "", false
	}
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)
	return uid, role, true
}


func mapCommentToDTO(c models.Comment) CommentResponseDTO {
	return CommentResponseDTO{
		ID:        c.ID,
		Content:   c.Content,
		CreatedAt: c.CreatedAt,
		User: CommentUserDTO{
			ID:   c.UserID.String(),
			Name: c.User.Name,
		},
	}
}



func viewsKey(pid uuid.UUID) string    { return "product:" + pid.String() + ":views" }


type ProductSearchParams struct {
    Q          string
    CategoryID *uint
    MinPrice   *int64
    MaxPrice   *int64
    Tags       []string
    OwnerID    *uuid.UUID
    InStock    *bool
    Sort       string
    Page       int
    Limit      int
}

func parseProductSearchParams(c *gin.Context) (ProductSearchParams, error) {
    p := ProductSearchParams{
        Q:     strings.TrimSpace(c.Query("q")),
        Sort:  strings.TrimSpace(c.DefaultQuery("sort", "newest")),
        Page:  1,
        Limit: 20,
    }

    if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 50 {
        p.Limit = l
    }
    if pg, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && pg > 0 {
        p.Page = pg
    }

    if s := strings.TrimSpace(c.Query("category_id")); s != "" {
        v, err := strconv.Atoi(s)
        if err != nil || v <= 0 { return p, fmt.Errorf("invalid_category_id") }
        u := uint(v)
        p.CategoryID = &u
    }

    if s := strings.TrimSpace(c.Query("min_price")); s != "" {
        v, err := strconv.ParseInt(s, 10, 64)
        if err != nil || v < 0 { return p, fmt.Errorf("invalid_min_price") }
        p.MinPrice = &v
    }
    if s := strings.TrimSpace(c.Query("max_price")); s != "" {
        v, err := strconv.ParseInt(s, 10, 64)
        if err != nil || v < 0 { return p, fmt.Errorf("invalid_max_price") }
        p.MaxPrice = &v
    }

    if s := strings.TrimSpace(c.Query("tags")); s != "" {
        raw := strings.Split(s, ",")
        p.Tags = normalizeTags(raw)
    }

    if s := strings.TrimSpace(c.Query("owner_id")); s != "" {
        oid, err := uuid.Parse(s)
        if err != nil { return p, fmt.Errorf("invalid_owner_id") }
        p.OwnerID = &oid
    }

    if s := strings.TrimSpace(c.Query("in_stock")); s != "" {
        v := strings.ToLower(s)
        b := (v == "true" || v == "1")
        p.InStock = &b
    }

    return p, nil
}

func buildProductsSearchQuery(tx *gorm.DB, params ProductSearchParams) *gorm.DB {
    q := tx.Model(&models.Product{}).
        Preload("Category").
        Preload("Tags").
        Preload("Owner", func(db *gorm.DB) *gorm.DB { return db.Select("id", "name") })

    // SQLite-friendly text search
    if params.Q != "" {
        qq := "%" + strings.ToLower(params.Q) + "%"
        q = q.Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ?", qq, qq)
    }

    if params.CategoryID != nil {
        q = q.Where("category_id = ?", *params.CategoryID)
    }
    if params.MinPrice != nil {
        q = q.Where("price >= ?", *params.MinPrice)
    }
    if params.MaxPrice != nil {
        q = q.Where("price <= ?", *params.MaxPrice)
    }
    if params.OwnerID != nil {
        q = q.Where("owner_id = ?", params.OwnerID.String())
    }
    if params.InStock != nil && *params.InStock {
        q = q.Where("stock > 0")
    }

    // tags OR filter (برای MVP بهتره OR)
    if len(params.Tags) > 0 {
        q = q.Joins("JOIN product_tags pt ON pt.product_id = products.id").
            Joins("JOIN tags t ON t.id = pt.tag_id").
            Where("t.name IN ?", params.Tags).
            Group("products.id")
    }

    switch params.Sort {
    case "price_asc":
        q = q.Order("price ASC")
    case "price_desc":
        q = q.Order("price DESC")
    case "views":
        q = q.Order("view_count DESC")
    case "likes":
        q = q.Order("like_count DESC")
    default: // newest
        q = q.Order("created_at DESC")
    }

    return q
}

func mapProductToListDTO(p models.Product) ProductListItemDTO {
    tags := make([]string, 0, len(p.Tags))
    for _, t := range p.Tags {
        tags = append(tags, t.Name)
    }
	var firstMediaURL string
        if len(p.Media) > 0 {
        firstMediaURL = p.Media[0].URL
    }
    dto := ProductListItemDTO{
        ID:         p.ID.String(),
        Title:      p.Title,
        Price:      p.Price,
        Stock:      p.Stock,
        OwnerID:    p.OwnerID.String(),
        CategoryID: p.CategoryID,
        Tags:       tags,
        ViewCount:  p.ViewCount,
        LikeCount:  p.LikeCount,
        CreatedAt:  p.CreatedAt,
		Media: firstMediaURL,
    }
    if p.Owner != nil { dto.OwnerName = p.Owner.Name }
    if p.Category != nil { dto.CategoryFa = p.Category.NameFa }
    return dto
}





