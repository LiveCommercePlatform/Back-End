package order

import (
	"errors"
	"net/http"
	"strconv"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)



// ─── CreateOrder godoc
// @Summary      Create New Orders
// @Description  آیتم‌های سبد خرید رو می‌گیره، موجودی رو چک می‌کنه، سفارش می‌سازه
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        input body CreateOrderInput true "آیتم‌های سفارش"
// @Success      201 {object} models.Order
// @Failure      400 {object} map[string]string
// @Failure      401 {object} map[string]string
// @Failure      422 {object} map[string]string
// @Security     BearerAuth
// @Router       /orders [post]
func CreateOrder(c *gin.Context) {
	userID, ok := mustGetAuth(c)
	if !ok {
		return
	}

	var input CreateOrderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// load user برای snapshot آدرس
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user_not_found"})
		return
	}

	// profile باید کامل باشه
	if user.Phone == "" || user.Address == "" || user.PostalCode == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "profile_incomplete"})
		return
	}

	var order models.Order

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var totalAmount int64
		var items []models.OrderItem

		for _, it := range input.Items {
			productID, _ := uuid.Parse(it.ProductID)

			var product models.Product
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				First(&product, "id = ?", productID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return &appError{Code: 422, Msg: "product_not_found: " + it.ProductID}
				}
				return err
			}

			if product.Stock < it.Qty {
				return &appError{Code: 422, Msg: "insufficient_stock: " + product.Title}
			}

			// کم کن از موجودی
			if err := tx.Model(&product).
				UpdateColumn("stock", gorm.Expr("stock - ?", it.Qty)).Error; err != nil {
				return err
			}

			unitPrice := product.Price
			totalAmount += unitPrice * int64(it.Qty)

			items = append(items, models.OrderItem{
				ProductID:  productID,
				Qty:        it.Qty,
				UnitPrice:  unitPrice,
				TotalPrice: unitPrice * int64(it.Qty),
			})
		}

		order = models.Order{
			UserID:       userID,
			Status:       models.OrderPending,
			TotalAmount:  totalAmount,
			ReceiverName: user.Name,
			Phone:        user.Phone,
			Address:      user.Address,
			PostalCode:   user.PostalCode,
			Items:        items,
		}

		// live room اختیاری
		if input.LiveRoomID != nil && *input.LiveRoomID != "" {
			lrID, err := uuid.Parse(*input.LiveRoomID)
			if err == nil {
				order.LiveRoomID = &lrID
			}
		}

		return tx.Create(&order).Error
	})

	if err != nil {
		var ae *appError
		if errors.As(err, &ae) {
			c.JSON(ae.Code, gin.H{"error": ae.Msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "order_failed"})
		return
	}

	// load با preload برای response
	database.DB.Preload("Items.Product").First(&order, "id = ?", order.ID)
	c.JSON(http.StatusCreated, order)
}

// ─── ListMyOrders godoc
// @Summary      
// @Tags         orders
// @Produce      json
// @Param        page      query int false "صفحه (پیش‌فرض 1)"
// @Param        page_size query int false "تعداد (پیش‌فرض 10)"
// @Success      200 {object} map[string]interface{}
// @Failure      401 {object} map[string]string
// @Security     BearerAuth
// @Router       /orders [get]
func ListMyOrders(c *gin.Context) {
	userID, ok := mustGetAuth(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	var orders []models.Order
	var total int64

	database.DB.Model(&models.Order{}).Where("user_id = ?", userID).Count(&total)
	database.DB.Preload("Items.Product").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(pageSize).Offset(offset).
		Find(&orders)

	c.JSON(http.StatusOK, gin.H{
		"data":      orders,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ─── GetOrderByID godoc
// @Summary      جزئیات یک سفارش
// @Tags         orders
// @Produce      json
// @Param        id path string true "Order ID"
// @Success      200 {object} models.Order
// @Failure      403 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Security     BearerAuth
// @Router       /orders/{id} [get]
func GetOrderByID(c *gin.Context) {
	userID, ok := mustGetAuth(c)
	if !ok {
		return
	}
	role := c.GetString("role")

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var order models.Order
	if err := database.DB.Preload("Items.Product").
		First(&order, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
		return
	}

	// فقط صاحب سفارش یا admin می‌تونه ببینه
	if order.UserID != userID && role != string(models.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	c.JSON(http.StatusOK, order)
}

// ─── CancelOrder godoc
// @Summary      لغو سفارش
// @Description  فقط سفارش‌های pending رو میشه لغو کرد
// @Tags         orders
// @Produce      json
// @Param        id path string true "Order ID"
// @Success      200 {object} models.Order
// @Failure      400 {object} map[string]string
// @Failure      403 {object} map[string]string
// @Security     BearerAuth
// @Router       /orders/{id}/cancel [patch]
func CancelOrder(c *gin.Context) {
	userID, ok := mustGetAuth(c)
	if !ok {
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var order models.Order
	if err := database.DB.Preload("Items").
		First(&order, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
		return
	}

	if order.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if order.Status != models.OrderPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only_pending_orders_can_be_cancelled"})
		return
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// برگردوندن موجودی
		for _, item := range order.Items {
			if err := tx.Model(&models.Product{}).
				Where("id = ?", item.ProductID).
				UpdateColumn("stock", gorm.Expr("stock + ?", item.Qty)).Error; err != nil {
				return err
			}
		}
		return tx.Model(&order).Update("status", models.OrderCancelled).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel_failed"})
		return
	}

	order.Status = models.OrderCancelled
	c.JSON(http.StatusOK, order)
}

// ─── Admin: ListAllOrders godoc
// @Summary      لیست همه سفارش‌ها (admin)
// @Tags         admin
// @Produce      json
// @Param        status    query string false "فیلتر وضعیت"
// @Param        page      query int    false "صفحه"
// @Param        page_size query int    false "تعداد"
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/orders [get]
func AdminListOrders(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	q := database.DB.Model(&models.Order{}).Preload("User").Preload("Items.Product")

	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	q.Count(&total)

	var orders []models.Order
	q.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&orders)

	c.JSON(http.StatusOK, gin.H{
		"data":      orders,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ─── Admin: UpdateOrderStatus godoc
// @Summary      آپدیت وضعیت سفارش (admin)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        id   path string                     true "Order ID"
// @Param        body body map[string]string          true "status"
// @Success      200 {object} models.Order
// @Failure      400 {object} map[string]string
// @Security     BearerAuth
// @Router       /admin/orders/{id}/status [patch]
func AdminUpdateOrderStatus(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_id"})
		return
	}

	var body struct {
		Status models.OrderStatus `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validStatuses := map[models.OrderStatus]bool{
		models.OrderPending:   true,
		models.OrderPaid:      true,
		models.OrderShipped:   true,
		models.OrderDelivered: true,
		models.OrderCancelled: true,
	}
	if !validStatuses[body.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_status"})
		return
	}

	var order models.Order
	if err := database.DB.First(&order, "id = ?", orderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "order_not_found"})
		return
	}

	database.DB.Model(&order).Update("status", body.Status)
	c.JSON(http.StatusOK, order)
}

