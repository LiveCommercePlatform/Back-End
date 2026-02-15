package models

type Pagination struct {
	Page     int `json:"page" binding:"required,gte=1"`
	PageSize int `json:"page_size" binding:"required,gt=0,lte=100"`
}
