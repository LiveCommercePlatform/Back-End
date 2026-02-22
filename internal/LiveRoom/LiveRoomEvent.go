package liveRoom

type LiveRoomEvent struct {
	Type   string      `json:"type"`
	RoomID string      `json:"room_id"`
	Data   interface{} `json:"data"`
	TS     int64       `json:"ts"`
}