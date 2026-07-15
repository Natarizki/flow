package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Natarizki/flow/pkg/utils"
)

var jwtSecret = []byte("flow-dev-secret-change-in-production")

const tokenExpiry = 7 * 24 * time.Hour

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

func SetSecret(secret string) {
	jwtSecret = []byte(secret)
}

func GenerateToken(userID, username, email string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "flow-daemon",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", utils.WrapError("TOKEN_SIGN", "failed to sign token", err)
	}
	return signed, nil
}

func ParseToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, utils.WrapError("TOKEN_PARSE", "invalid or expired token", err)
	}
	if !token.Valid {
		return nil, utils.ErrAuthFailed
	}
	return claims, nil
}

func RefreshToken(oldToken string) (string, error) {
	claims, err := ParseToken(oldToken)
	if err != nil {
		return "", err
	}
	return GenerateToken(claims.UserID, claims.Username, claims.Email)
}
