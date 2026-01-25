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
	store := &fakeKeyStore{key: &APIKey{ID: "1", Hash: HashAPIKey(key), IsActive: true}}

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

func TestValidateAPIKey_Expired(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	expired := time.Now().Add(-time.Hour)
	store := &fakeKeyStore{key: &APIKey{ID: "1", Hash: HashAPIKey(key), IsActive: true, ExpiresAt: &expired}}

	if _, err := ValidateAPIKey(ctx, store, "Bearer "+key); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected invalid api key, got %v", err)
	}
}

func TestValidateAPIKey_Inactive(t *testing.T) {
	ctx := context.Background()
	key := "abc12345secret"
	store := &fakeKeyStore{key: &APIKey{ID: "1", Hash: HashAPIKey(key), IsActive: false}}

	if _, err := ValidateAPIKey(ctx, store, "Bearer "+key); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expected invalid api key, got %v", err)
	}
}

func TestAPIKeyFromRequest(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	key, err := APIKeyFromRequest(req)
	if err != nil || key != "abc123" {
		t.Fatalf("expected abc123, got %s err %v", key, err)
	}
}
