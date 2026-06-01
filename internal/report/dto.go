package report

import (
	"github.com/google/uuid"
)

type CreateReportInput struct {
	Type         string  `json:"type"           binding:"required,oneof=product comment user"`
	Reason       string  `json:"reason"         binding:"required,min=5,max=1000"`
	ProductID    *uuid.UUID `json:"product_id"`
	CommentID    *uint   `json:"comment_id"`
	TargetUserID *uuid.UUID `json:"target_user_id"`
}

type UpdateReportStatusInput struct {
	Status string `json:"status" binding:"required,oneof=new reviewing closed"`
}
