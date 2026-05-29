package order

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type appError struct {
	Code int
	Msg  string
}

func (e *appError) Error() string { return e.Msg }

func mustGetAuth(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.UUID{}, false
	}
	uid, ok := v.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.UUID{}, false
	}
	return uid, true
}
