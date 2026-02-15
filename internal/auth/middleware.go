

package auth

import (
	"fmt"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func AuthMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid token"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		secret := []byte(os.Getenv("JWT_SECRET"))

		claims := &Claims{}

		parser := jwt.NewParser(
			jwt.WithValidMethods([]string{"HS256"}),
		)

		keyFunc := func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		}

		_, err := parser.ParseWithClaims(tokenString, claims, keyFunc)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if requiredRole != "" && claims.Role != requiredRole {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}
		uid, err := uuid.Parse(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id in token"})
			c.Abort()
			return
		}

		c.Set("userID", uid)
		c.Set("role", claims.Role)

		c.Next()
	}
}




func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.Next()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		secret := []byte(os.Getenv("JWT_SECRET"))

		claims := &Claims{} 

		parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))

		keyFunc := func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		}

		_, err := parser.ParseWithClaims(tokenString, claims, keyFunc)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		uid, err := uuid.Parse(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id in token"})
			c.Abort()
			return
		}

		c.Set("userID", uid)
		c.Set("role", claims.Role)

		c.Next()
	}
}




func GetUserDataFromContext(c *gin.Context) (uuid.UUID, string) {
    v, exists := c.Get("userID")
    if !exists {
        return uuid.UUID{}, ""
    }
    userID, _ := v.(uuid.UUID)

    role := c.GetString("role")
    return userID, role
}



func RequireProfileCompleted() gin.HandlerFunc {
	return func(c *gin.Context) {

		v, exists := c.Get("userID")
        if !exists {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
            c.Abort()
            return
        }

        userID, ok := v.(uuid.UUID)
        if !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
            c.Abort()
            return
        }

        var user models.User
        if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
            c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
            c.Abort()
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

		if len(missing) > 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "profile_not_completed",
				"missing_fields": missing,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}