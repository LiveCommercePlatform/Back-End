package auth

import (
	"fmt"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func WSOptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// اگر cookie نبود => مهم نیست، viewer anonymous
		tokenString, err := c.Cookie(WSAccessTokenCookieName)
		if err != nil || strings.TrimSpace(tokenString) == "" {
			c.Next()
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
			// optional => ignore invalid cookie token
			c.Next()
			return
		}

		uid, uidErr := uuid.Parse(claims.UserID)
		if uidErr != nil {
			// optional => ignore bad payload
			c.Next()
			return
		}

		c.Set("userID", uid)
		c.Set("role", claims.Role)
		c.Next()
	}
}