package message

type SendMessageInput struct {
	Name    string `json:"name"    binding:"required,min=2,max=100"`
	Email   string `json:"email"   binding:"required,email"`
	Content string `json:"content" binding:"required,min=10,max=2000"`
}
