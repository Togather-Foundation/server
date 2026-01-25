package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret []byte
	expiry time.Duration
	issuer string
}

var (
	ErrMissingToken = errors.New("missing token")
	ErrInvalidToken = errors.New("invalid token")
)

func NewJWTManager(secret string, expiry time.Duration, issuer string) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		expiry: expiry,
		issuer: issuer,
	}
}

func (m *JWTManager) Generate(subject, role string) (string, error) {
	if subject == "" || role == "" {
		return "", ErrInvalidToken
	}

	now := time.Now()
	claims := &Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *JWTManager) Validate(tokenString string) (*Claims, error) {
	if strings.TrimSpace(tokenString) == "" {
		return nil, ErrMissingToken
	}

	parsed, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func TokenFromHeader(authHeader string) (string, error) {
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", ErrMissingToken
	}
	return strings.TrimSpace(parts[1]), nil
}
