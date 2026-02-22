package liveRoom



type CreateLiveRoomInput struct {
	Title       string `json:"title" binding:"required,min=3,max=120"`
	Description string `json:"description" binding:"max=2000"`
	IsRecorded  bool   `json:"is_recorded"`
}

type UpdateLiveRoomInput struct {
	Title       *string `json:"title" binding:"omitempty,min=3,max=120"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
	IsRecorded  *bool   `json:"is_recorded"`
}

type LiveChatMessage struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}
type LiveEvent struct {
	Type string      `json:"type"` // viewer_update | like_update
	Data interface{} `json:"data"`
}


type AttachProductsInput struct {
	ProductIDs []string `json:"product_ids" binding:"required,min=1"`
}

type PinProductInput struct {
	IsPinned bool `json:"is_pinned"`
}

type ReorderProductsInput struct {
	Items []struct {
		ProductID  string `json:"product_id" binding:"required"`
		SortOrder  int    `json:"sort_order"`
	} `json:"items" binding:"required,min=1"`
}

type LiveRoomProductItem struct {
	ProductID string `json:"product_id"`
	IsPinned  bool   `json:"is_pinned"`
	SortOrder int    `json:"sort_order"`
}

type ProductsUpdatedData struct {
	Action   string                `json:"action"`
	Products []LiveRoomProductItem `json:"products"`
}

type ViewPingInput struct {
	ViewerKey string `json:"viewer_key" binding:"required"`
}


type ReactionSummary struct {
	Likes    int64 `json:"likes"`
	Dislikes int64 `json:"dislikes"`
}


type chatIncoming struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	ClientMsgID string `json:"client_msg_id"`
}

type chatMessageData struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Text   string `json:"text"`
	TS     int64  `json:"ts"`
}

type chatEvent struct {
	Type   string          `json:"type"`
	RoomID string          `json:"room_id"`
	Data   chatMessageData `json:"data"`
	TS     int64           `json:"ts"`
}

type chatAck struct {
	Type        string `json:"type"`
	ClientMsgID string `json:"client_msg_id"`
	ID          string `json:"id"`
	TS          int64  `json:"ts"`
}

type chatError struct {
	Type  string `json:"type"`
	Error string `json:"error"`
	TS    int64  `json:"ts"`
}