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

// Test parseCreateOrganizationParams
func TestParseCreateOrganizationParams(t *testing.T) {
	tests := []struct {
		name          string
		input         map[string]interface{}
		baseURL       string
		expectError   bool
		errorContains string
		validate      func(t *testing.T, params organizations.CreateParams)
	}{
		{
			name:          "missing name",
			input:         map[string]interface{}{},
			baseURL:       "https://test.example.com",
			expectError:   true,
			errorContains: "name is required",
		},
		{
			name: "valid minimal params",
			input: map[string]interface{}{
				"name": "Test Organization",
			},
			baseURL:     "https://test.example.com",
			expectError: false,
			validate: func(t *testing.T, params organizations.CreateParams) {
				if params.Name != "Test Organization" {
					t.Errorf("expected name 'Test Organization', got %q", params.Name)
				}
			},
		},
		{
			name: "with all fields",
			input: map[string]interface{}{
				"name":        "Community Foundation",
				"legalName":   "Community Foundation Inc.",
				"description": "A non-profit organization",
				"url":         "https://example.org",
			},
			baseURL:     "https://test.example.com",
			expectError: false,
			validate: func(t *testing.T, params organizations.CreateParams) {
				if params.Name != "Community Foundation" {
					t.Errorf("expected name 'Community Foundation', got %q", params.Name)
				}
				if params.LegalName != "Community Foundation Inc." {
					t.Errorf("expected legalName 'Community Foundation Inc.', got %q", params.LegalName)
				}
				if params.Description != "A non-profit organization" {
					t.Errorf("expected description, got %q", params.Description)
				}
				if params.URL != "https://example.org" {
					t.Errorf("expected URL, got %q", params.URL)
				}
			},
		},
		{
			name: "address from nested object",
			input: map[string]interface{}{
				"name": "Foundation",
				"address": map[string]interface{}{
					"streetAddress":   "123 Main St",
					"addressLocality": "Toronto",
					"addressRegion":   "ON",
					"postalCode":      "M5H 2N2",
					"addressCountry":  "CA",
				},
			},
			baseURL:     "https://test.example.com",
			expectError: false,
			validate: func(t *testing.T, params organizations.CreateParams) {
				if params.StreetAddress != "123 Main St" {
					t.Errorf("expected street '123 Main St', got %q", params.StreetAddress)
				}
				if params.AddressLocality != "Toronto" {
					t.Errorf("expected locality 'Toronto', got %q", params.AddressLocality)
				}
			},
		},
		{
			name: "address from top-level fields",
			input: map[string]interface{}{
				"name":            "Org",
				"streetAddress":   "456 Elm St",
				"addressLocality": "Vancouver",
				"addressRegion":   "BC",
			},
			baseURL:     "https://test.example.com",
			expectError: false,
			validate: func(t *testing.T, params organizations.CreateParams) {
				if params.StreetAddress != "456 Elm St" {
					t.Errorf("expected street '456 Elm St', got %q", params.StreetAddress)
				}
				if params.AddressRegion != "BC" {
					t.Errorf("expected region 'BC', got %q", params.AddressRegion)
				}
			},
		},
		{
			name: "federation URI from @id",
			input: map[string]interface{}{
				"name": "External Org",
				"@id":  "https://external.org/organizations/abc123",
			},
			baseURL:     "https://test.example.com",
			expectError: false,
			validate: func(t *testing.T, params organizations.CreateParams) {
				if params.FederationURI == nil {
					t.Error("expected FederationURI to be set")
				} else if *params.FederationURI != "https://external.org/organizations/abc123" {
					t.Errorf("expected federation URI, got %q", *params.FederationURI)
				}
			},
		},
		{
			name: "whitespace trimming",
			input: map[string]interface{}{
				"name":        "  Trimmed Name  ",
				"legalName":   "  Trimmed Legal  ",
				"description": "  Trimmed Desc  ",
			},
			baseURL:     "https://test.example.com",
			expectError: false,
			validate: func(t *testing.T, params organizations.CreateParams) {
				if params.Name != "Trimmed Name" {
					t.Errorf("expected trimmed name, got %q", params.Name)
				}
				if params.LegalName != "Trimmed Legal" {
					t.Errorf("expected trimmed legal name, got %q", params.LegalName)
				}
				if params.Description != "Trimmed Desc" {
					t.Errorf("expected trimmed description, got %q", params.Description)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := parseCreateOrganizationParams(tt.input, tt.baseURL)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorContains)
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				if tt.validate != nil {
					tt.validate(t, params)
				}
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

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
