package auth

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const WSAccessTokenCookieName = "ws_access_token"

func WSAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// فقط cookie (browser) — چون هدف ما WS در webbrowser هست
		tokenString, err := c.Cookie(WSAccessTokenCookieName)
		if err != nil || strings.TrimSpace(tokenString) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_ws_token"})
			c.Abort()
			return
		}
		tokenString = strings.TrimSpace(tokenString)

		secret := []byte(os.Getenv("JWT_SECRET"))
		claims := &Claims{}

		parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))

		keyFunc := func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return secret, nil
		}

		_, parseErr := parser.ParseWithClaims(tokenString, claims, keyFunc)
		if parseErr != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_or_expired_ws_token"})
			c.Abort()
			return
		}

		uid, uidErr := uuid.Parse(claims.UserID)
		if uidErr != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_user_id_in_token"})
			c.Abort()
			return
		}

		c.Set("userID", uid)
		c.Set("role", claims.Role)

		c.Next()
	}
}