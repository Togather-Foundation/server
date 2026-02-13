package tools

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/organizations"
)

// Test buildOrganizationURI
func TestBuildOrganizationURI(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		ulid     string
		expected string
	}{
		{
			name:     "valid URI",
			baseURL:  "https://test.example.com",
			ulid:     "01HX1234567890ABCDEFGHJKMN",
			expected: "https://test.example.com/organizations/01HX1234567890ABCDEFGHJKMN",
		},
		{
			name:     "empty baseURL",
			baseURL:  "",
			ulid:     "01HX1234567890ABCDEFGHJKMN",
			expected: "",
		},
		{
			name:     "empty ulid",
			baseURL:  "https://test.example.com",
			ulid:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildOrganizationURI(tt.baseURL, tt.ulid)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test buildOrganizationListItem
func TestBuildOrganizationListItem(t *testing.T) {
	tests := []struct {
		name    string
		org     organizations.Organization
		baseURL string
		wantID  string
	}{
		{
			name: "basic organization",
			org: organizations.Organization{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Name: "Test Org",
			},
			baseURL: "https://test.example.com",
			wantID:  "https://test.example.com/organizations/01HX1234567890ABCDEFGHJKMN",
		},
		{
			name: "with legal name",
			org: organizations.Organization{
				ULID:      "01HX1234567890ABCDEFGHJKMN",
				Name:      "Foundation",
				LegalName: "Foundation Inc.",
			},
			baseURL: "https://test.example.com",
			wantID:  "https://test.example.com/organizations/01HX1234567890ABCDEFGHJKMN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildOrganizationListItem(tt.org, tt.baseURL)

			if result["@type"] != "Organization" {
				t.Errorf("expected @type Organization, got %v", result["@type"])
			}
			if result["name"] != tt.org.Name {
				t.Errorf("expected name %q, got %v", tt.org.Name, result["name"])
			}
			if tt.wantID != "" && result["@id"] != tt.wantID {
				t.Errorf("expected @id %q, got %v", tt.wantID, result["@id"])
			}
		})
	}
}

// Test buildOrganizationPayload
func TestBuildOrganizationPayload(t *testing.T) {
	tests := []struct {
		name    string
		org     *organizations.Organization
		baseURL string
		wantNil bool
	}{
		{
			name:    "nil organization",
			org:     nil,
			baseURL: "https://test.example.com",
			wantNil: false, // Returns empty map, not nil
		},
		{
			name: "full organization",
			org: &organizations.Organization{
				ULID:        "01HX1234567890ABCDEFGHJKMN",
				Name:        "Full Org",
				LegalName:   "Full Organization Inc.",
				Description: "A complete organization",
				URL:         "https://fullorg.example.com",
			},
			baseURL: "https://test.example.com",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildOrganizationPayload(tt.org, tt.baseURL)

			if result == nil {
				if !tt.wantNil {
					t.Error("expected non-nil result")
				}
				return
			}

			if tt.org != nil {
				if result["@type"] != "Organization" {
					t.Errorf("expected @type Organization, got %v", result["@type"])
				}
				if result["name"] != tt.org.Name {
					t.Errorf("expected name %q, got %v", tt.org.Name, result["name"])
				}
			}
		})
	}
}

// contains is a test helper for checking substring matches in error messages.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
