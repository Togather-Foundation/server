package auth

import (
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// DerivedKeyLength is the length of derived keys in bytes (32 bytes = 256 bits for HMAC-SHA256)
	DerivedKeyLength = 32

	// Key derivation purpose strings for HKDF
	purposeAdminJWT     = "togather-admin-jwt-v1"
	purposeDeveloperJWT = "togather-developer-jwt-v1"
)

// ErrInvalidMasterSecret is returned when the master secret is invalid
var ErrInvalidMasterSecret = errors.New("master secret cannot be empty")

// DeriveKey derives a cryptographic key from a master secret using HKDF-SHA256.
// This creates cryptographically independent keys from a single master secret.
//
// Parameters:
//   - masterSecret: The base secret to derive from (e.g., from JWT_SECRET env var)
//   - purpose: A unique purpose string that creates domain separation between keys
//
// Returns a 32-byte key suitable for HMAC-SHA256 signing.
//
// Security properties:
//   - Keys derived with different purpose strings are cryptographically independent
//   - Compromise of one derived key does not compromise the master secret or other derived keys
//   - HKDF-SHA256 provides secure key derivation as specified in RFC 5869
func DeriveKey(masterSecret []byte, purpose string) ([]byte, error) {
	if len(masterSecret) == 0 {
		return nil, ErrInvalidMasterSecret
	}

	// Create HKDF reader with SHA-256
	// salt=nil is acceptable per RFC 5869 (defaults to zeros)
	// info=purpose provides domain separation between different key uses
	hkdf := hkdf.New(sha256.New, masterSecret, nil, []byte(purpose))

	// Read DerivedKeyLength bytes from HKDF
	derivedKey := make([]byte, DerivedKeyLength)
	if _, err := io.ReadFull(hkdf, derivedKey); err != nil {
		return nil, err
	}

	return derivedKey, nil
}

// DeriveAdminJWTKey derives a key for signing admin JWT tokens.
// This key is cryptographically independent from the developer JWT key.
func DeriveAdminJWTKey(masterSecret []byte) ([]byte, error) {
	return DeriveKey(masterSecret, purposeAdminJWT)
}

// DeriveDeveloperJWTKey derives a key for signing developer JWT tokens.
// This key is cryptographically independent from the admin JWT key.
func DeriveDeveloperJWTKey(masterSecret []byte) ([]byte, error) {
	return DeriveKey(masterSecret, purposeDeveloperJWT)
}
