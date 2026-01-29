package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/email"
	"livecommerce/internal/models"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)
var ctx = context.Background()
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
		UserID       string `json:"user_id" binding:"required"`
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	


type UpdateProfileInput struct {
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	PostalCode string `json:"postal_code"`
}



// var resetCodes = map[string]string{}
// var verificationCodes = map[string]string{}




// Signup godoc
// @Summary Register a new user
// @Description Create a new user and send verification code
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body SignupInput true "Signup data"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /auth/signup [post]
func Signup(c *gin.Context) {
	var input SignupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error hashing password"})
		return
	}

	user := models.User{
		Name:     input.Name,
		Email:    input.Email,
		Password: string(hashedPassword),
		Role:     models.RoleUser,
		Verified: false,
		CreatedAt: time.Now(),
	}

	if err := database.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already exists"})
		return
	}

	// code := fmt.Sprintf("%06d", rand.Intn(1000000))
	code := Generate6DigitCode()
	// verificationCodes[user.Email] = code
	err = cache.Client.Set(ctx, "verify:"+user.Email, code, 10*time.Minute).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store verification code"})
		return
	}

	subject := "Verify Your Email"
	body := fmt.Sprintf("<p>Your verification code is <b>%s</b></p>", code)
	email.SendEmail(user.Email, subject, body)

	token, err := GenerateJWT(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}
	rawRefresh, hashedRefresh, err :=GenerateRefreshTokenValue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}


	err = cache.Client.Set(ctx, "refresh:"+user.ID.String(), hashedRefresh, 7*24*time.Hour).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Signup successful. Verification code sent.",
		"access_token": token,
		"refresh_token": rawRefresh,
	})
}


// VerifyEmail godoc
// @Summary Verify user's email
// @Description Confirm email by verification code sent to user
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body VerifyInput true "Email and code"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /auth/verify [post]
func VerifyEmail(c *gin.Context) {
	var input VerifyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// expectedCode, exists := verificationCodes[input.Email]
	// if !exists || expectedCode != input.Code {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification code"})
	// 	return
	// }
	storedCode, err := cache.Client.Get(ctx, "verify:"+input.Email).Result()
	if err != nil {
	c.JSON(http.StatusBadRequest, gin.H{"error": "Verification code expired or invalid"})
	return
	}

	if input.Code != storedCode {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid verification code"})
	return
	}


	cache.Client.Del(ctx, "verify:"+input.Email)
	var user models.User
	if err := database.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	user.Verified = true
	database.DB.Save(&user)
	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully."})
}


// Login godoc
// @Summary Log in user
// @Description Authenticate user with email and password
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body LoginInput true "Login data"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /auth/login [post]
func Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := database.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	token, err := GenerateJWT(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	rawRefresh, hashedRefresh, err := GenerateRefreshTokenValue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}


	err = cache.Client.Set(ctx, "refresh:"+user.ID.String(), hashedRefresh, 7*24*time.Hour).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store refresh token"})
		return
	}


	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"access_token": token,
		"refresh_token": rawRefresh,
	})
}

// ForgotPassword godoc
// @Summary Request password reset code
// @Description Send 6-digit code to user's email for password reset
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body ForgotPasswordInput true "User email"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /auth/forgot-password [post]
func ForgotPassword(c *gin.Context) {
	var input ForgotPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user models.User
	if err := database.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	code := Generate6DigitCode()
	err := cache.Client.Set(ctx, "reset:"+input.Email, code, 10*time.Minute).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store reset code"})
		return
	}

	subject := "Reset Your Password"
	body := fmt.Sprintf("<p>Your password reset code is <b>%s</b></p>", code)
	if err := email.SendEmail(input.Email, subject, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send reset email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Reset code sent to your email"})
}

// ResetPassword godoc
// @Summary Reset user password
// @Description Update password using email and reset code
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body ResetPasswordInput true "Reset password data"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /auth/reset-password [post]
func ResetPassword(c *gin.Context) {
	var input ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	storedCode, err := cache.Client.Get(ctx, "reset:"+input.Email).Result()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reset code expired or invalid"})
		return
	}


	if input.Code != storedCode {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid reset code"})
		return
	}
	// expectedCode, exists := resetCodes[input.Email]
	// if !exists || expectedCode != input.Code {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reset code"})
	// 	return
	// }
	cache.Client.Del(ctx, "reset:"+input.Email)
	
	// var user models.User
	// if err := database.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
	// 	c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
	// 	return
	// }

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	database.DB.Model(&models.User{}).Where("email = ?", input.Email).Update("password", string(hashedPassword))	

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successful"})
}

// ChangePassword godoc
// @Summary Change password
// @Description Change current user password
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body ChangePasswordInput true "Change password payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /auth/change-password [patch]
func ChangePassword(c *gin.Context) {

	var input ChangePasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("userID")
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.CurrentPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Incorrect current password"})
		return
	}

	hashedNew, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user.Password = string(hashedNew)
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
}

// RefreshToken godoc
// @Summary Refresh access token using refresh token
// @Description Generates a new access token using a valid refresh token stored in Redis
// @Tags Authentication
// @Accept json
// @Produce json
// @Param input body RefreshInput true "Refresh Token Input"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /auth/refresh [post]
func RefreshToken(c *gin.Context) {


	var input RefreshInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	storedToken, err := cache.Client.Get(ctx, "refresh:"+input.UserID).Result()
	if err != nil || storedToken != input.RefreshToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", input.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	accessToken, err := GenerateJWT(user.ID.String(), string(user.Role))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
	})
}


// Logout godoc
// @Summary Logout user
// @Description Deletes refresh token from Redis using user ID from JWT
// @Tags Authentication
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/logout [post]
func Logout(c *gin.Context) {
	claims, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userClaims, ok := claims.(*Claims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}

	userID := userClaims.UserID

	err := cache.Client.Del(ctx, "refresh:"+userID).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}



// GetProfile godoc
// @Summary Get current user profile
// @Tags Profile
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /profile/get [get]
func GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{"user": user})
}



// UpdateProfile godoc
// @Summary Update user profile
// @Tags Profile
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param input body UpdateProfileInput true "Profile Info"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /profile/update [put]
func UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("userID")

	var input UpdateProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	user.Name = input.Name
	user.Phone = input.Phone
	user.Address = input.Address
	user.PostalCode = input.PostalCode

	database.DB.Save(&user)

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}



// GetUserByID godoc
// @Summary      Get user by ID
// @Description  Retrieve user information by user ID (admin or public)
// @Tags         Profile
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      404  {object}  map[string]string
// @Router       /profile/get/{id} [get]
func GetUserByID(c *gin.Context) {
	id := c.Param("id") 

	var user models.User
	if err := database.DB.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}


// ProfileCompleted godoc
// @Summary Check if user profile is completed
// @Description Returns whether the authenticated user's profile is completed
// @Tags Profile
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /profile/completed [get]
func ProfileCompleted(c *gin.Context) {

	userID, _ := c.Get("userID")

	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	missing := []string{}

	if !user.Verified {
		missing = append(missing, "email_verification")
	}
	if user.Name == "" {
		missing = append(missing, "name")
	}
	if user.Phone == "" {
		missing = append(missing, "phone")
	}
	if user.Address == "" {
		missing = append(missing, "address")
	}
	if user.PostalCode == "" {
		missing = append(missing, "postal_code")
	}

	c.JSON(http.StatusOK, gin.H{
		"completed":      len(missing) == 0,
		"missing_fields": missing,
	})
}
