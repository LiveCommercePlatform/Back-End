package auth

import (
	"context"
	"fmt"
	"livecommerce/internal/cache"
	"livecommerce/internal/database"
	"livecommerce/internal/email"
	"livecommerce/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)



var ctx = context.Background()
const (
	verifyTTL  = 10 * time.Minute
	resetTTL   = 10 * time.Minute
	refreshTTL = 7 * 24 * time.Hour
)

func mustGetUserID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.UUID{}, false
	}
	uid, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return uuid.UUID{}, false
	}
	return uid, true
}
func storeRefreshSession(userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	rt := models.RefreshToken{
		TokenHash: tokenHash,
		UserID:    userID, 
		ExpiresAt: expiresAt,
		Revoked:   false,
	}
	if err := database.DB.Create(&rt).Error; err != nil {
		return err
	}

	return cache.Client.Set(ctx, "refresh:"+tokenHash, userID, time.Until(expiresAt)).Err()
}

func revokeRefreshSession(tokenHash string) {
	_ = cache.Client.Del(ctx, "refresh:"+tokenHash).Err()
	_ = database.DB.Model(&models.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Update("revoked", true).Error
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

	code := Generate6DigitCode()
	err = cache.Client.Set(ctx, "verify:"+user.Email, code, verifyTTL).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store verification code"})
		return
	}

	subject := "Verify Your Email"
	body := fmt.Sprintf("<p>Your verification code is <b>%s</b></p>", code)
	if err := email.SendEmail(user.Email, subject, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send verification email"})
		return
	}

	accessToken, err := GenerateJWT(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	rawRefresh, hashedRefresh, err := GenerateRefreshTokenValue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}
	expiresAt := time.Now().Add(refreshTTL)
	if err := storeRefreshSession(user.ID, hashedRefresh, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store refresh session"})
		return
	}


	c.JSON(http.StatusOK, gin.H{
		"message": "Signup successful. Verification code sent.",
		"access_token":  accessToken,
		"refresh_token": rawRefresh,
		"user_id":       user.ID.String(),
		"role":          string(user.Role),
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
	storedCode, err := cache.Client.Get(ctx, "verify:"+input.Email).Result()
	if err != nil {
	c.JSON(http.StatusBadRequest, gin.H{"error": "Verification code expired or invalid"})
	return
	}

	if input.Code != storedCode {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid verification code"})
	return
	}


	_ = cache.Client.Del(ctx, "verify:"+input.Email).Err()
	var user models.User
	if err := database.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	user.Verified = true
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}
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

	accessToken, err := GenerateJWT(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	rawRefresh, hashedRefresh, err := GenerateRefreshTokenValue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	
	expiresAt := time.Now().Add(refreshTTL)
	if err := storeRefreshSession(user.ID, hashedRefresh, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store refresh session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"access_token":  accessToken,
		"refresh_token": rawRefresh, 
		"user_id":       user.ID.String(),
		"role":          string(user.Role),
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
	err := cache.Client.Set(ctx, "reset:"+input.Email, code, resetTTL).Err()
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

	cache.Client.Del(ctx, "reset:"+input.Email)
	


	hashedPassword, err  := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	var user models.User
	if err := database.DB.Where("email = ?", input.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := database.DB.Model(&models.User{}).
		Where("email = ?", input.Email).
		Update("password", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}	

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
// @Router /auth/change-password [post]
func ChangePassword(c *gin.Context) {
	userID, ok := mustGetUserID(c)
	if !ok {
		return
	}

	var input ChangePasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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

	oldHashed := HashRefreshToken(input.RefreshToken)

	// userIDStr, err := cache.Client.Get(ctx, "refresh:"+oldHashed).Result()
	// if err != nil {
	// 	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
	// 	return
	// }

	var rt models.RefreshToken
	if err := database.DB.Where("token_hash = ? AND revoked = false", oldHashed).First(&rt).Error; err != nil {
		revokeRefreshSession(oldHashed)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}
	if time.Now().After(rt.ExpiresAt) {
		revokeRefreshSession(oldHashed)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired"})
		return
	}
	_, err := cache.Client.Get(ctx, "refresh:"+oldHashed).Result()
	if err != nil {
		revokeRefreshSession(oldHashed) // هماهنگی DB/Redis
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh session expired"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", rt.UserID).Error; err != nil {
		revokeRefreshSession(oldHashed)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	// revokeRefreshSession(oldHashed)

	accessToken, err := GenerateJWT(user.ID.String(), string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	// revokeRefreshSession(oldHashed)

	newRaw, newHashed, err := GenerateRefreshTokenValue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	expiresAt := time.Now().Add(refreshTTL)
	if err := storeRefreshSession(user.ID, newHashed, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store refresh session"})
		return
	}
	revokeRefreshSession(oldHashed)
	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": newRaw,
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
    var input LogoutInput
    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

	hashed := HashRefreshToken(input.RefreshToken)
	revokeRefreshSession(hashed)

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
	userID, ok := mustGetUserID(c)
	if !ok {
		return
	}

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
	userID, ok := mustGetUserID(c)
	if !ok {
		return
	}

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

	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

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

	userID, ok := mustGetUserID(c)
	if !ok {
		return
	}

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
