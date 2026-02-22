package auth

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func wsCookieSecure() bool {
	return os.Getenv("COOKIE_SECURE") == "true"
}

// POST /auth/ws-cookie  (auth required)
// Bearer token رو می‌گیره و تو cookie ws_access_token ذخیره می‌کنه
func SetWSChatCookie(c *gin.Context) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_token"})
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_token"})
		return
	}

	maxAge := int((24 * time.Hour).Seconds())

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		WSAccessTokenCookieName,
		token,
		maxAge,
		"/",
		"",
		wsCookieSecure(),
		true, // HttpOnly
	)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func ClearWSChatCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(WSAccessTokenCookieName, "", -1, "/", "", wsCookieSecure(), true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}