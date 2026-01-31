package auth 

type SignupInput struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type VerifyInput struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6"`
}

type LoginInput struct {
	Email string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}


type ForgotPasswordInput struct {
	Email string `json:"email" binding:"required,email"`
}


type ResetPasswordInput struct {
	Email string `json:"email" binding:"required,email"`
	Code string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}


type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
}


type RefreshInput struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	


type UpdateProfileInput struct {
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	PostalCode string `json:"postal_code"`
}

type LogoutInput struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}
