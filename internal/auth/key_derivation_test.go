package auth

import (
	"bytes"
	"testing"
)

func TestDeriveKey(t *testing.T) {
	tests := []struct {
		name         string
		masterSecret []byte
		purpose      string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "valid derivation",
			masterSecret: []byte("this-is-a-secure-master-secret-for-testing"),
			purpose:      "test-purpose-v1",
			wantErr:      false,
		},
		{
			name:         "empty master secret",
			masterSecret: []byte{},
			purpose:      "test-purpose-v1",
			wantErr:      true,
			errMsg:       "master secret cannot be empty",
		},
		{
			name:         "nil master secret",
			masterSecret: nil,
			purpose:      "test-purpose-v1",
			wantErr:      true,
			errMsg:       "master secret cannot be empty",
		},
		{
			name:         "empty purpose string is allowed",
			masterSecret: []byte("test-secret"),
			purpose:      "",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := DeriveKey(tt.masterSecret, tt.purpose)

			if tt.wantErr {
				if err == nil {
					t.Errorf("DeriveKey() expected error, got nil")
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("DeriveKey() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("DeriveKey() unexpected error: %v", err)
				return
			}

			if len(key) != DerivedKeyLength {
				t.Errorf("DeriveKey() key length = %d, want %d", len(key), DerivedKeyLength)
			}
		})
	}
}

func TestDerivedKeysAreIndependent(t *testing.T) {
	masterSecret := []byte("shared-master-secret-for-all-keys")

	// Derive keys with different purposes
	adminKey, err := DeriveKey(masterSecret, "admin-purpose-v1")
	if err != nil {
		t.Fatalf("failed to derive admin key: %v", err)
	}

	developerKey, err := DeriveKey(masterSecret, "developer-purpose-v1")
	if err != nil {
		t.Fatalf("failed to derive developer key: %v", err)
	}

	otherKey, err := DeriveKey(masterSecret, "other-purpose-v1")
	if err != nil {
		t.Fatalf("failed to derive other key: %v", err)
	}

	// Keys derived with different purposes should be completely different
	if bytes.Equal(adminKey, developerKey) {
		t.Error("admin and developer keys are identical (should be cryptographically independent)")
	}
	if bytes.Equal(adminKey, otherKey) {
		t.Error("admin and other keys are identical (should be cryptographically independent)")
	}
	if bytes.Equal(developerKey, otherKey) {
		t.Error("developer and other keys are identical (should be cryptographically independent)")
	}
}

func TestDerivedKeysAreDeterministic(t *testing.T) {
	masterSecret := []byte("test-master-secret")
	purpose := "test-purpose"

	// Derive the same key multiple times
	key1, err := DeriveKey(masterSecret, purpose)
	if err != nil {
		t.Fatalf("first derivation failed: %v", err)
	}

	key2, err := DeriveKey(masterSecret, purpose)
	if err != nil {
		t.Fatalf("second derivation failed: %v", err)
	}

	key3, err := DeriveKey(masterSecret, purpose)
	if err != nil {
		t.Fatalf("third derivation failed: %v", err)
	}

	// Same inputs should produce identical keys
	if !bytes.Equal(key1, key2) {
		t.Error("first and second derivations produced different keys (should be deterministic)")
	}
	if !bytes.Equal(key1, key3) {
		t.Error("first and third derivations produced different keys (should be deterministic)")
	}
	if !bytes.Equal(key2, key3) {
		t.Error("second and third derivations produced different keys (should be deterministic)")
	}
}

func TestDeriveAdminJWTKey(t *testing.T) {
	masterSecret := []byte("test-master-secret-for-admin-jwt")

	key, err := DeriveAdminJWTKey(masterSecret)
	if err != nil {
		t.Fatalf("DeriveAdminJWTKey() failed: %v", err)
	}

	if len(key) != DerivedKeyLength {
		t.Errorf("DeriveAdminJWTKey() key length = %d, want %d", len(key), DerivedKeyLength)
	}

	// Should produce the same key consistently
	key2, _ := DeriveAdminJWTKey(masterSecret)
	if !bytes.Equal(key, key2) {
		t.Error("DeriveAdminJWTKey() is not deterministic")
	}
}

func TestDeriveDeveloperJWTKey(t *testing.T) {
	masterSecret := []byte("test-master-secret-for-developer-jwt")

	key, err := DeriveDeveloperJWTKey(masterSecret)
	if err != nil {
		t.Fatalf("DeriveDeveloperJWTKey() failed: %v", err)
	}

	if len(key) != DerivedKeyLength {
		t.Errorf("DeriveDeveloperJWTKey() key length = %d, want %d", len(key), DerivedKeyLength)
	}

	// Should produce the same key consistently
	key2, _ := DeriveDeveloperJWTKey(masterSecret)
	if !bytes.Equal(key, key2) {
		t.Error("DeriveDeveloperJWTKey() is not deterministic")
	}
}

func TestAdminAndDeveloperKeysAreSeparate(t *testing.T) {
	masterSecret := []byte("shared-master-secret")

	adminKey, err := DeriveAdminJWTKey(masterSecret)
	if err != nil {
		t.Fatalf("failed to derive admin key: %v", err)
	}

	developerKey, err := DeriveDeveloperJWTKey(masterSecret)
	if err != nil {
		t.Fatalf("failed to derive developer key: %v", err)
	}

	// Admin and developer keys should be cryptographically independent
	if bytes.Equal(adminKey, developerKey) {
		t.Error("admin and developer JWT keys are identical (prevents token confusion attacks)")
	}
}

func TestDifferentMasterSecretsProduceDifferentKeys(t *testing.T) {
	secret1 := []byte("first-master-secret")
	secret2 := []byte("second-master-secret")
	purpose := "test-purpose"

	key1, err := DeriveKey(secret1, purpose)
	if err != nil {
		t.Fatalf("failed to derive key from first secret: %v", err)
	}

	key2, err := DeriveKey(secret2, purpose)
	if err != nil {
		t.Fatalf("failed to derive key from second secret: %v", err)
	}

	// Different master secrets should produce different keys
	if bytes.Equal(key1, key2) {
		t.Error("different master secrets produced identical keys")
	}
}
