package auth

import (
	"math/rand"
	"time"
	"fmt"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
)

// GenerateRefreshTokenValue returns a new raw token string (to send to client)
// and its SHA-256 hex hash (to store in DB).
func GenerateRefreshTokenValue() (raw string, hash string, err error) {
	r := uuid.NewString()
	h := sha256.Sum256([]byte(r))
	return r, hex.EncodeToString(h[:]), nil
}

// HashRefreshToken converts a client-provided raw refresh token to its stored hash.
func HashRefreshToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func Generate6DigitCode() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	code := fmt.Sprintf("%06d", r.Intn(1000000)) 
	return code
}

