package product

import "time"

type CreateProductInput struct {
	Title       string   `json:"title" binding:"required,min=2,max=200"`
	Description string   `json:"description"`
	Price       int64    `json:"price" binding:"required,gt=0"`
	Stock       int      `json:"stock" binding:"gte=0"`
	CategoryID  uint     `json:"category_id" binding:"required,gt=0"`
	Tags        []string `json:"tags"`
}

type UpdateProductInput struct {
	Title       *string   `json:"title" binding:"omitempty,min=2,max=200"`
	Description *string   `json:"description"`
	Price       *int64    `json:"price" binding:"omitempty,gt=0"`
	Stock       *int      `json:"stock" binding:"omitempty,gte=0"`
	CategoryID  *uint     `json:"category_id" binding:"omitempty,gt=0"`
	Tags        *[]string `json:"tags"`
}

type CategoryNode struct {
	ID       uint            `json:"id"`
	Key      string          `json:"key"`
	NameFa   string          `json:"name_fa"`
	ParentID *uint           `json:"parent_id,omitempty"`
	Children []*CategoryNode `json:"children,omitempty"`
}


type CategoryTreeResponse struct {
	Data []*CategoryNode `json:"data"`
}

type GetCommentsInput struct {
	Page     int `json:"page" binding:"required,gte=1"`
	PageSize int `json:"page_size" binding:"required,gt=0,lte=100"`
}

type CreateCommentInput struct {
	Content string `json:"content" binding:"required"`
}

type UpdateCommentInput struct {
	Content string `json:"content" binding:"required,min=1,max=2000"`
}

type CommentUserDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CommentResponseDTO struct {
	ID        uint           `json:"id"`
	Content   string         `json:"content"`
	CreatedAt time.Time      `json:"created_at"`
	User      CommentUserDTO `json:"user"`
}

type PaginatedCommentsResponseDTO struct {
	Data       []CommentResponseDTO `json:"data"`
	Pagination PaginationMetaDTO    `json:"pagination"`
}

type PaginationMetaDTO struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}


type ProductStatsDTO struct {
	Likes      int64 `json:"likes"`
	Dislikes   int64 `json:"dislikes"`
	Views      int64 `json:"views"`
	Engagement int64 `json:"engagement"`
}


type MyEngagementDTO struct {
	Liked    bool `json:"liked"`
	Disliked bool `json:"disliked"`
}


type ProductListItemDTO struct {
    ID         string   `json:"id"`
    Title      string   `json:"title"`
    Price      int64    `json:"price"`
    Stock      int      `json:"stock"`
    OwnerID    string   `json:"owner_id"`
    OwnerName  string   `json:"owner_name"`
    CategoryID uint     `json:"category_id"`
    CategoryFa string   `json:"category_name_fa"`
    Tags       []string `json:"tags"`
    ViewCount  int64    `json:"view_count"`
    LikeCount  int64    `json:"like_count"`
    CreatedAt  time.Time `json:"created_at"`
}

type ProductSearchResponseDTO struct {
    Data       []ProductListItemDTO `json:"data"`
    Pagination PaginationMetaDTO    `json:"pagination"`
}



type UpsertRatingInput struct {
	Rating int `json:"rating" binding:"required"`
}

type RatingSummaryResponse struct {
	RatingAvg   float64       `json:"rating_avg"`
	RatingCount int64         `json:"rating_count"`
	Breakdown   map[int]int64 `json:"breakdown"`
}

type MyRatingResponse struct {
	MyRating int `json:"my_rating"`
}

type RatingUpsertResponse struct {
	ProductID   string  `json:"product_id"`
	MyRating    int     `json:"my_rating"`
	RatingAvg   float64 `json:"rating_avg"`
	RatingCount int64   `json:"rating_count"`
}