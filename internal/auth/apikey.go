package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type APIKey struct {
	ID            string
	Prefix        string
	Hash          string
	Name          string
	SourceID      string
	Role          string
	RateLimitTier string
	IsActive      bool
	ExpiresAt     *time.Time
}

type APIKeyStore interface {
	LookupByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	UpdateLastUsed(ctx context.Context, id string) error
}

var (
	ErrMissingAPIKey = errors.New("missing api key")
	ErrInvalidAPIKey = errors.New("invalid api key")
)

func APIKeyFromRequest(r *http.Request) (string, error) {
	if r == nil {
		return "", ErrMissingAPIKey
	}
	return APIKeyFromHeader(r.Header.Get("Authorization"))
}

func APIKeyFromHeader(authHeader string) (string, error) {
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", ErrMissingAPIKey
	}
	key := strings.TrimSpace(parts[1])
	if key == "" || !utf8.ValidString(key) {
		return "", ErrInvalidAPIKey
	}
	return key, nil
}

func ValidateAPIKey(ctx context.Context, store APIKeyStore, authHeader string) (*APIKey, error) {
	if store == nil {
		return nil, ErrInvalidAPIKey
	}

	key, err := APIKeyFromHeader(authHeader)
	if err != nil {
		return nil, err
	}
	if len(key) < 8 {
		return nil, ErrInvalidAPIKey
	}

	prefix := key[:8]
	stored, err := store.LookupByPrefix(ctx, prefix)
	if err != nil || stored == nil {
		return nil, ErrInvalidAPIKey
	}
	if !stored.IsActive {
		return nil, ErrInvalidAPIKey
	}
	if stored.ExpiresAt != nil && stored.ExpiresAt.Before(time.Now()) {
		return nil, ErrInvalidAPIKey
	}

	providedHash := HashAPIKey(key)
	if subtle.ConstantTimeCompare([]byte(providedHash), []byte(stored.Hash)) != 1 {
		return nil, ErrInvalidAPIKey
	}

	_ = store.UpdateLastUsed(ctx, stored.ID)
	return stored, nil
}

func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
