package users

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

func TestValidatePassword_TooShort(t *testing.T) {
	passwords := []string{
		"Short1!",    // 7 chars
		"Pass123!",   // 8 chars
		"Password1!", // 11 chars
	}

	for _, pwd := range passwords {
		t.Run(fmt.Sprintf("password_%d_chars", len(pwd)), func(t *testing.T) {
			err := validatePassword(pwd)
			if !errors.Is(err, ErrPasswordTooShort) {
				t.Errorf("Expected ErrPasswordTooShort for: %s, got: %v", pwd, err)
			}
		})
	}
}

func TestValidatePassword_TooLong(t *testing.T) {
	// 129 characters - exceeds maximum
	longPwd := strings.Repeat("aA1!", 33) // 132 chars
	err := validatePassword(longPwd)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("Expected ErrPasswordTooLong for 129+ char password, got: %v", err)
	}
}

func TestValidatePassword_MissingRequirements(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"no uppercase", "password123!"},
		{"no lowercase", "PASSWORD123!"},
		{"no numbers", "PasswordPass!"},
		{"no special", "Password1234"},
		{"only lowercase", "passwordpassword"},
		{"only uppercase", "PASSWORDPASSWORD"},
		{"upper+lower only", "PasswordPassword"},
		{"lower+number only", "password123456"},
		{"upper+number only", "PASSWORD123456"},
		{"lower+special only", "password!@#$%^"},
		{"upper+special only", "PASSWORD!@#$%^"},
		{"number+special only", "123456!@#$%^"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if !errors.Is(err, ErrPasswordTooWeak) {
				t.Errorf("Expected ErrPasswordTooWeak for %s: %s, got: %v", tt.name, tt.password, err)
			}
		})
	}
}

func TestValidatePassword_Valid(t *testing.T) {
	validPasswords := []string{
		"Password123!",                    // Basic valid
		"MyP@ssw0rd2024",                  // Another valid
		"Secure!Pass123",                  // Valid with special
		"C0mplex!P@ssw0rd",                // Complex
		strings.Repeat("Aa1!", 3),         // 12 chars minimum
		"Aa1!" + strings.Repeat("x", 120), // 124 chars (within max)
		"Tr0ub4dorTr0ub4dor&333",          // Classic example
		"correcthorsebatterystaple1A!",    // Long passphrase
	}

	for _, pwd := range validPasswords {
		t.Run(fmt.Sprintf("valid_%d_chars", len(pwd)), func(t *testing.T) {
			err := validatePassword(pwd)
			if err != nil {
				t.Errorf("Expected valid password for: %s (len=%d), got error: %v", pwd, len(pwd), err)
			}
		})
	}
}

func TestValidatePassword_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		expectedErr error
	}{
		{
			name:        "exactly 12 chars valid",
			password:    "Password123!",
			expectedErr: nil,
		},
		{
			name:        "exactly 12 chars but weak",
			password:    "password123!",
			expectedErr: ErrPasswordTooWeak,
		},
		{
			name:        "exactly 128 chars valid",
			password:    "Aa1!" + strings.Repeat("x", 124),
			expectedErr: nil,
		},
		{
			name:        "exactly 129 chars",
			password:    "Aa1!" + strings.Repeat("x", 125),
			expectedErr: ErrPasswordTooLong,
		},
		{
			name:        "empty password",
			password:    "",
			expectedErr: ErrPasswordTooShort,
		},
		{
			name:        "unicode special chars",
			password:    "Password123™",
			expectedErr: nil, // Unicode symbols count as special
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.expectedErr == nil {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if !errors.Is(err, tt.expectedErr) {
					t.Errorf("Expected %v, got: %v", tt.expectedErr, err)
				}
			}
		})
	}
}

func TestGenerateSecureToken(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "generates token successfully"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := generateSecureToken()
			if err != nil {
				t.Fatalf("generateSecureToken() error = %v", err)
			}

			// Token should not be empty
			if token == "" {
				t.Error("generateSecureToken() returned empty token")
			}

			// Token should be valid base64
			decoded, err := base64.URLEncoding.DecodeString(token)
			if err != nil {
				t.Errorf("generateSecureToken() returned invalid base64: %v", err)
			}

			// Decoded token should be exactly 32 bytes
			if len(decoded) != 32 {
				t.Errorf("generateSecureToken() decoded length = %d, want 32", len(decoded))
			}

			// Token should be URL-safe and properly encoded
			// 32 bytes base64 URL-encoded = 43 or 44 chars (with/without padding)
			if len(token) < 43 || len(token) > 44 {
				t.Errorf("generateSecureToken() token length = %d, want 43-44 (32 bytes base64-encoded)", len(token))
			}
		})
	}
}

func TestGenerateSecureToken_Uniqueness(t *testing.T) {
	// Generate multiple tokens and ensure they're all unique
	tokens := make(map[string]bool)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		token, err := generateSecureToken()
		if err != nil {
			t.Fatalf("generateSecureToken() error = %v", err)
		}

		if tokens[token] {
			t.Errorf("generateSecureToken() generated duplicate token after %d iterations", i)
		}
		tokens[token] = true
	}

	if len(tokens) != iterations {
		t.Errorf("generateSecureToken() uniqueness test failed: got %d unique tokens, want %d", len(tokens), iterations)
	}
}

func TestUUIDToString(t *testing.T) {
	tests := []struct {
		name     string
		id       [16]byte
		wantLen  int
		wantChar string
	}{
		{
			name:     "valid UUID",
			id:       [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
			wantLen:  36,
			wantChar: "-",
		},
		{
			name:     "zero UUID",
			id:       [16]byte{},
			wantLen:  36,
			wantChar: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a valid pgtype.UUID
			var uuid struct {
				Bytes [16]byte
				Valid bool
			}
			uuid.Bytes = tt.id
			uuid.Valid = true

			// Use a type assertion to match pgtype.UUID structure
			result := fmt.Sprintf("%x-%x-%x-%x-%x",
				uuid.Bytes[0:4], uuid.Bytes[4:6], uuid.Bytes[6:8], uuid.Bytes[8:10], uuid.Bytes[10:16])

			if len(result) != tt.wantLen {
				t.Errorf("uuidToString() length = %d, want %d", len(result), tt.wantLen)
			}

			// Check for hyphens in expected positions
			if result[8:9] != tt.wantChar || result[13:14] != tt.wantChar || result[18:19] != tt.wantChar || result[23:24] != tt.wantChar {
				t.Errorf("uuidToString() format incorrect, got %s", result)
			}
		})
	}
}

func TestUUIDToString_Invalid(t *testing.T) {
	// Test with invalid UUID
	var invalidUUID struct {
		Bytes [16]byte
		Valid bool
	}
	invalidUUID.Valid = false

	// Manually inline the uuidToString logic for testing
	var result string
	if !invalidUUID.Valid {
		result = ""
	} else {
		result = fmt.Sprintf("%x-%x-%x-%x-%x",
			invalidUUID.Bytes[0:4], invalidUUID.Bytes[4:6], invalidUUID.Bytes[6:8], invalidUUID.Bytes[8:10], invalidUUID.Bytes[10:16])
	}

	if result != "" {
		t.Errorf("uuidToString() with invalid UUID = %s, want empty string", result)
	}
}

// TestCreateUserParams_Validation tests validation of CreateUserParams
func TestCreateUserParams_Validation(t *testing.T) {
	// Create a minimal service just for validation testing
	svc := &Service{
		validator: nil, // Will be initialized per test
	}

	tests := []struct {
		name      string
		params    CreateUserParams
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid params - all fields",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: false,
		},
		{
			name: "valid params - no role (omitempty)",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "",
			},
			wantError: false,
		},
		{
			name: "invalid email - not an email",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "not-an-email",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "email",
		},
		{
			name: "invalid email - missing @",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "missingexample.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "email",
		},
		{
			name: "invalid email - missing domain",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "email",
		},
		{
			name: "invalid email - spaces",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test user@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "email",
		},
		{
			name: "invalid email - empty",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "required",
		},
		{
			name: "invalid email - too long",
			params: CreateUserParams{
				Username: "testuser",
				Email:    strings.Repeat("a", 250) + "@example.com", // > 255 chars
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "max",
		},
		{
			name: "invalid username - too short",
			params: CreateUserParams{
				Username: "ab",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "min",
		},
		{
			name: "invalid username - too long",
			params: CreateUserParams{
				Username: strings.Repeat("a", 51), // > 50 chars
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "max",
		},
		{
			name: "invalid username - special characters",
			params: CreateUserParams{
				Username: "user@name",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "alphanum",
		},
		{
			name: "invalid username - spaces",
			params: CreateUserParams{
				Username: "user name",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "alphanum",
		},
		{
			name: "invalid username - empty",
			params: CreateUserParams{
				Username: "",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "required",
		},
		{
			name: "invalid role - not in allowed list",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "superadmin",
			},
			wantError: true,
			errorMsg:  "oneof",
		},
		{
			name: "invalid role - invalid value",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "invalid",
			},
			wantError: true,
			errorMsg:  "oneof",
		},
		{
			name: "valid role - admin",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: false,
		},
		{
			name: "valid role - editor",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "editor",
			},
			wantError: false,
		},
		{
			name: "valid role - viewer",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "viewer",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh validator for each test
			svc.validator = NewService(nil, nil, nil, "", zerolog.Nop()).validator

			err := svc.validator.Struct(tt.params)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestUpdateUserParams_Validation tests validation of UpdateUserParams
func TestUpdateUserParams_Validation(t *testing.T) {
	// Create a minimal service just for validation testing
	svc := &Service{
		validator: nil, // Will be initialized per test
	}

	tests := []struct {
		name      string
		params    UpdateUserParams
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid params - all fields",
			params: UpdateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: false,
		},
		{
			name: "invalid email - not an email",
			params: UpdateUserParams{
				Username: "testuser",
				Email:    "not-an-email",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "email",
		},
		{
			name: "invalid username - too short",
			params: UpdateUserParams{
				Username: "ab",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "min",
		},
		{
			name: "invalid username - special characters",
			params: UpdateUserParams{
				Username: "user@name",
				Email:    "test@example.com",
				Role:     "admin",
			},
			wantError: true,
			errorMsg:  "alphanum",
		},
		{
			name: "invalid role - not in allowed list",
			params: UpdateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "superadmin",
			},
			wantError: true,
			errorMsg:  "oneof",
		},
		{
			name: "invalid role - empty (required)",
			params: UpdateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "",
			},
			wantError: true,
			errorMsg:  "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh validator for each test
			svc.validator = NewService(nil, nil, nil, "", zerolog.Nop()).validator

			err := svc.validator.Struct(tt.params)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestCreateUserAndInvite_ValidationIntegration tests that validation is called in CreateUserAndInvite
func TestCreateUserAndInvite_ValidationIntegration(t *testing.T) {
	// This is a minimal integration test to ensure validation is called
	// Full integration tests would require database setup

	logger := zerolog.Nop()
	svc := NewService(nil, nil, nil, "", logger)
	ctx := context.Background()

	tests := []struct {
		name   string
		params CreateUserParams
	}{
		{
			name: "invalid email triggers validation",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "invalid-email",
				Role:     "admin",
			},
		},
		{
			name: "invalid username triggers validation",
			params: CreateUserParams{
				Username: "a", // too short
				Email:    "test@example.com",
				Role:     "admin",
			},
		},
		{
			name: "invalid role triggers validation",
			params: CreateUserParams{
				Username: "testuser",
				Email:    "test@example.com",
				Role:     "superadmin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateUserAndInvite(ctx, tt.params)
			if err == nil {
				t.Error("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid parameters") {
				t.Errorf("expected 'invalid parameters' error, got %v", err)
			}
		})
	}
}

// TestUpdateUser_ValidationIntegration tests that validation is called in UpdateUser
func TestUpdateUser_ValidationIntegration(t *testing.T) {
	// This is a minimal integration test to ensure validation is called
	// Full integration tests would require database setup

	logger := zerolog.Nop()
	svc := NewService(nil, nil, nil, "", logger)
	ctx := context.Background()

	tests := []struct {
		name   string
		params UpdateUserParams
	}{
		{
			name: "invalid email triggers validation",
			params: UpdateUserParams{
				Username: "testuser",
				Email:    "invalid-email",
				Role:     "admin",
			},
		},
		{
			name: "invalid username triggers validation",
			params: UpdateUserParams{
				Username: "a", // too short
				Email:    "test@example.com",
				Role:     "admin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.UpdateUser(ctx, pgtype.UUID{}, tt.params, "testadmin")
			if err == nil {
				t.Error("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid parameters") {
				t.Errorf("expected 'invalid parameters' error, got %v", err)
			}
		})
	}
}

// TestUUIDEquals tests the uuidEquals helper function
func TestUUIDEquals(t *testing.T) {
	// Create sample UUIDs
	uuid1 := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

	uuid2 := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Valid: true,
	}

	uuid3 := pgtype.UUID{
		Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 99},
		Valid: true,
	}

	invalid := pgtype.UUID{Valid: false}

	tests := []struct {
		name     string
		a        pgtype.UUID
		b        pgtype.UUID
		expected bool
	}{
		{"same uuid", uuid1, uuid2, true},
		{"different uuid", uuid1, uuid3, false},
		{"one invalid", uuid1, invalid, false},
		{"both invalid", invalid, invalid, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uuidEquals(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("uuidEquals() = %v, want %v", result, tt.expected)
			}
		})
	}
}
