package jsonld

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNewValidator(t *testing.T) {
	// Find repo root
	root := findRepoRoot(t)
	shapesDir := filepath.Join(root, "shapes")

	tests := []struct {
		name      string
		shapesDir string
		enabled   bool
		wantErr   bool
		wantNoOp  bool // Expect validator to be no-op (pyshacl not found)
	}{
		{
			name:      "valid shapes directory with validation enabled",
			shapesDir: shapesDir,
			enabled:   true,
			wantErr:   false,
		},
		{
			name:      "validation disabled",
			shapesDir: shapesDir,
			enabled:   false,
			wantErr:   false,
			wantNoOp:  true,
		},
		{
			name:      "nonexistent shapes directory",
			shapesDir: "/nonexistent/path",
			enabled:   true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewValidator(tt.shapesDir, tt.enabled)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewValidator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if tt.wantNoOp && v.enabled {
					t.Errorf("Expected validator to be disabled, but it's enabled")
				}
				if !tt.wantNoOp && tt.enabled && v.pyshaclPath == "" {
					// pyshacl not found, validator will be no-op
					t.Skip("pyshacl not installed, validator will be no-op")
				}
			}
		})
	}
}

func TestValidator_ValidateEvent(t *testing.T) {
	// Check if pyshacl is available
	_, err := exec.LookPath("pyshacl")
	if err != nil {
		t.Skip("pyshacl not installed, skipping validation tests")
	}

	root := findRepoRoot(t)
	shapesDir := filepath.Join(root, "shapes")

	v, err := NewValidator(shapesDir, true)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name    string
		data    map[string]any
		wantErr bool
	}{
		{
			name: "valid event with all required fields",
			data: map[string]any{
				"@context":  "https://schema.org",
				"@type":     "Event",
				"@id":       "https://example.org/events/test-123",
				"name":      "Test Event",
				"startDate": "2026-01-01T19:00:00Z",
				"location": map[string]any{
					"@type": "Place",
					"@id":   "https://example.org/places/test-456",
					"name":  "Test Venue",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid event missing required name",
			data: map[string]any{
				"@context":  "https://schema.org",
				"@type":     "Event",
				"@id":       "https://example.org/events/test-123",
				"startDate": "2026-01-01T19:00:00Z",
				"location": map[string]any{
					"@type": "Place",
					"@id":   "https://example.org/places/test-456",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid event missing required startDate",
			data: map[string]any{
				"@context": "https://schema.org",
				"@type":    "Event",
				"@id":      "https://example.org/events/test-123",
				"name":     "Test Event",
				"location": map[string]any{
					"@type": "Place",
					"@id":   "https://example.org/places/test-456",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid event missing required location",
			data: map[string]any{
				"@context":  "https://schema.org",
				"@type":     "Event",
				"@id":       "https://example.org/events/test-123",
				"name":      "Test Event",
				"startDate": "2026-01-01T19:00:00Z",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := v.ValidateEvent(ctx, tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEvent() error = %v, wantErr %v", err, tt.wantErr)
			}

			// If we expect an error, check it's a ValidationError
			if tt.wantErr && err != nil {
				if _, ok := err.(*ValidationError); !ok {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestValidator_DisabledValidation(t *testing.T) {
	root := findRepoRoot(t)
	shapesDir := filepath.Join(root, "shapes")

	// Create validator with validation disabled
	v, err := NewValidator(shapesDir, false)
	if err != nil {
		t.Fatalf("Failed to create disabled validator: %v", err)
	}

	// Invalid data should pass validation when disabled
	invalidData := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Event",
		"@id":      "https://example.org/events/test-123",
		// Missing required fields: name, startDate, location
	}

	ctx := context.Background()
	err = v.ValidateEvent(ctx, invalidData)
	if err != nil {
		t.Errorf("Expected no error with disabled validation, got: %v", err)
	}
}

// findRepoRoot finds the repository root by looking for go.mod
func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("repo root not found")
	return ""
}
