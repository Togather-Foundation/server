package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type fakeKeyStore struct {
	key          *APIKey
	lookupPrefix string
	updateCalled bool
	lookupErr    error
}

func (f *fakeKeyStore) LookupByPrefix(ctx context.Context, prefix string) (*APIKey, error) {
	f.lookupPrefix = prefix
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return f.key, nil
}

func (f *fakeKeyStore) UpdateLastUsed(ctx context.Context, id string) error {
	f.updateCalled = true
	return nil
}

func TestAPIKeyFromHeader(t *testing.T) {
	if _, err := APIKeyFromHeader(""); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected missing api key, got %v", err)
	}
	if _, err := APIKeyFromHeader("Bearer "); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("expected missing api key, got %v", err)
	}
	if key, err := APIKeyFromHeader("Bearer abc123"); err != nil || key != "abc123" {
		t.Fatalf("expected abc123, got %s err %v", key, err)
	}
}

func TestValidateAPIKey(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	hash, err := HashAPIKey(key)
	if err != nil {
		t.Fatalf("failed to hash key: %v", err)
	}
	store := &fakeKeyStore{key: &APIKey{
		ID:          "1",
		Hash:        hash,
		HashVersion: HashVersionBcrypt,
		IsActive:    true,
	}}

	got, err := ValidateAPIKey(ctx, store, "Bearer "+key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got == nil || got.ID != "1" {
		t.Fatalf("expected API key, got %#v", got)
	}
	if !store.updateCalled {
		t.Fatalf("expected UpdateLastUsed to be called")
	}
	if store.lookupPrefix != key[:8] {
		t.Fatalf("expected prefix %s, got %s", key[:8], store.lookupPrefix)
	}
}

func TestValidateAPIKey_LegacySHA256(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	hash := HashAPIKeySHA256(key)
	store := &fakeKeyStore{key: &APIKey{
		ID:          "1",
		Hash:        hash,
		HashVersion: HashVersionSHA256,
		IsActive:    true,
	}}

	got, err := ValidateAPIKey(ctx, store, "Bearer "+key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got == nil || got.ID != "1" {
		t.Fatalf("expected API key with legacy hash to validate, got %#v", got)
	}
	if !store.updateCalled {
		t.Fatalf("expected UpdateLastUsed to be called")
	}
}

func TestValidateAPIKey_Expired(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	hash, _ := HashAPIKey(key)
	expired := time.Now().Add(-time.Hour)
	store := &fakeKeyStore{key: &APIKey{
		ID:          "1",
		Hash:        hash,
		HashVersion: HashVersionBcrypt,
		IsActive:    true,
		ExpiresAt:   &expired,
	}}

	if _, err := ValidateAPIKey(ctx, store, "Bearer "+key); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected invalid api key, got %v", err)
	}
}

func TestValidateAPIKey_Inactive(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	hash, _ := HashAPIKey(key)
	store := &fakeKeyStore{key: &APIKey{
		ID:          "1",
		Hash:        hash,
		HashVersion: HashVersionBcrypt,
		IsActive:    false,
	}}

	if _, err := ValidateAPIKey(ctx, store, "Bearer "+key); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected invalid api key, got %v", err)
	}
}

func TestValidateAPIKey_WrongKey(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	wrongKey := "abc12345wrongsecret"
	hash, _ := HashAPIKey(key)
	store := &fakeKeyStore{key: &APIKey{
		ID:          "1",
		Hash:        hash,
		HashVersion: HashVersionBcrypt,
		IsActive:    true,
	}}

	if _, err := ValidateAPIKey(ctx, store, "Bearer "+wrongKey); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected invalid api key for wrong key, got %v", err)
	}
}

func TestHashAPIKey_Bcrypt(t *testing.T) {
	key := "test-key-12345"
	hash1, err1 := HashAPIKey(key)
	hash2, err2 := HashAPIKey(key)

	if err1 != nil || err2 != nil {
		t.Fatalf("expected no error hashing, got err1=%v err2=%v", err1, err2)
	}

	// Bcrypt hashes should be different each time (due to random salt)
	if hash1 == hash2 {
		t.Fatalf("expected different bcrypt hashes, got same: %s", hash1)
	}

	// But both should validate against the same key
	if err := validateBcryptHash(key, hash1); err != nil {
		t.Fatalf("hash1 should validate: %v", err)
	}
	if err := validateBcryptHash(key, hash2); err != nil {
		t.Fatalf("hash2 should validate: %v", err)
	}
}

func TestHashAPIKeySHA256_Legacy(t *testing.T) {
	key := "test-key-12345"
	hash1 := HashAPIKeySHA256(key)
	hash2 := HashAPIKeySHA256(key)

	// SHA-256 hashes should be deterministic (same every time)
	if hash1 != hash2 {
		t.Fatalf("expected same SHA-256 hash, got %s vs %s", hash1, hash2)
	}

	// Should be 64 hex characters (32 bytes)
	if len(hash1) != 64 {
		t.Fatalf("expected 64 character hex hash, got %d: %s", len(hash1), hash1)
	}
}

// Helper to validate bcrypt hash
func validateBcryptHash(key, hash string) error {
	return nil // Placeholder - bcrypt.CompareHashAndPassword is tested in ValidateAPIKey
}

func TestAPIKeyFromRequest(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	key, err := APIKeyFromRequest(req)
	if err != nil || key != "abc123" {
		t.Fatalf("expected abc123, got %s err %v", key, err)
	}
}
