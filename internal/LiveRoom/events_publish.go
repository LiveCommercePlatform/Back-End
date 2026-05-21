package liveRoom

import (
	"time"

	"livecommerce/internal/database"
	"livecommerce/internal/models"

	"github.com/google/uuid"
)

func fetchRoomProducts(roomID uuid.UUID) ([]LiveRoomProductItem, error) {
	var items []models.LiveRoomProduct
	if err := database.DB.
		Where("live_room_id = ?", roomID).
		Order("is_pinned desc, sort_order asc, id asc").
		Find(&items).Error; err != nil {
		return nil, err
	}

	out := make([]LiveRoomProductItem, 0, len(items))
	for _, it := range items {
		out = append(out, LiveRoomProductItem{
			ProductID: it.ProductID.String(),
			IsPinned:  it.IsPinned,
			SortOrder: it.SortOrder,
		})
	}
	return out, nil
}

func publishProductsUpdate(hub *RoomHub, roomID uuid.UUID, action string) {
	products, err := fetchRoomProducts(roomID)
	if err != nil {
		return
	}

	ev := LiveRoomEvent{
		Type:   "live_room.products.updated",
		RoomID: roomID.String(),
		Data: ProductsUpdatedData{
			Action:   action,
			Products: products,
		},
		TS: time.Now().Unix(),
	}

	hub.Broadcast(roomID, ev)
}