package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderStatus string

const (
	OrderPending    OrderStatus = "pending"
	OrderPaid       OrderStatus = "paid"
	OrderShipped    OrderStatus = "shipped"
	OrderDelivered  OrderStatus = "delivered"
	OrderCancelled  OrderStatus = "cancelled"
)

type Order struct {
	ID     uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	UserID uuid.UUID   `gorm:"type:uuid;not null;index" json:"user_id"`
	User   User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`

	Items []OrderItem `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"items,omitempty"`

	Status      OrderStatus `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	TotalAmount int64       `gorm:"not null" json:"total_amount"`

	ReceiverName string `gorm:"not null" json:"receiver_name"`
	Phone        string `gorm:"not null" json:"phone"`
	Address      string `gorm:"not null" json:"address"`
	PostalCode   string `gorm:"not null" json:"postal_code"`

	LiveRoomID *uuid.UUID `gorm:"type:uuid;index" json:"live_room_id,omitempty"`
	LiveRoom   *LiveRoom  `gorm:"foreignKey:LiveRoomID" json:"live_room,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OrderItem struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	ProductID uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	Product   *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`

	Qty        int   `gorm:"not null" json:"qty"`
	UnitPrice  int64 `gorm:"not null" json:"unit_price"`  // snapshot قیمت موقع خرید
	TotalPrice int64 `gorm:"not null" json:"total_price"` // qty * unit_price
}

func (o *Order) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}
