
package order
type CreateOrderItemInput struct {
	ProductID string `json:"product_id" binding:"required,uuid"`
	Qty       int    `json:"qty"        binding:"required,gte=1"`
}

type CreateOrderInput struct {
	Items      []CreateOrderItemInput `json:"items"       binding:"required,min=1"`
	LiveRoomID *string                `json:"live_room_id"`
}