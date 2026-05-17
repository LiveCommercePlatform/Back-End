package liveRoom

type JoinSignalPayload struct {
	RoomID string `json:"room_id"`
	IsHost bool   `json:"is_host"`
}