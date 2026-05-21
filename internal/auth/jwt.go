package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateJWT(userID string, role string) (string, error) {
	secret := []byte(os.Getenv("JWT_SECRET"))
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}


func ParseAccessToken(tokenString string) (*Claims, error) {
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
        return nil, err
    }
    return claims, nil
}