package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// DeveloperClaims represents JWT claims for developer authentication.
// These claims are separate from admin claims to prevent privilege escalation.
type DeveloperClaims struct {
	DeveloperID uuid.UUID `json:"developer_id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // Always "developer"
	jwt.RegisteredClaims
}

var (
	ErrInvalidClaimsType = errors.New("invalid claims type")
	ErrWrongTokenType    = errors.New("token type mismatch: expected developer token")
)

// GenerateDeveloperToken creates a JWT token for a developer with specified expiry.
// The token includes developer-specific claims and is signed with the provided secret.
// developerID, email, and name should come from the developer domain object.
func GenerateDeveloperToken(developerID uuid.UUID, email, name, jwtSecret string, expiryHours int, issuer string) (string, time.Time, error) {
	if developerID == uuid.Nil {
		return "", time.Time{}, errors.New("developer ID cannot be nil")
	}
	if email == "" {
		return "", time.Time{}, errors.New("email cannot be empty")
	}
	if jwtSecret == "" {
		return "", time.Time{}, errors.New("jwt secret cannot be empty")
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(expiryHours) * time.Hour)

	claims := &DeveloperClaims{
		DeveloperID: developerID,
		Email:       email,
		Name:        name,
		Type:        "developer",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "developer",
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// ValidateDeveloperToken validates a JWT token and returns developer claims.
// Returns an error if the token is invalid, expired, or not a developer token.
func ValidateDeveloperToken(tokenString string, jwtSecret string) (*DeveloperClaims, error) {
	if strings.TrimSpace(tokenString) == "" {
		return nil, ErrMissingToken
	}
	if jwtSecret == "" {
		return nil, errors.New("jwt secret cannot be empty")
	}

	// Parse and validate token
	parsed, err := jwt.ParseWithClaims(tokenString, &DeveloperClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Extract claims
	claims, ok := parsed.Claims.(*DeveloperClaims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}

	// Explicitly verify this is a developer token to prevent privilege escalation
	if claims.Type != "developer" {
		return nil, ErrWrongTokenType
	}

	// Verify subject matches expected value
	if claims.Subject != "developer" {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
