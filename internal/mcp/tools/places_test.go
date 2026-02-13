package tools

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/places"
)

// Test parseFloat
func TestParseFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected *float64
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "float64",
			input:    43.65,
			expected: float64Ptr(43.65),
		},
		{
			name:     "float32",
			input:    float32(43.65),
			expected: float64Ptr(float64(float32(43.65))), // Account for float32 precision
		},
		{
			name:     "int",
			input:    42,
			expected: float64Ptr(42.0),
		},
		{
			name:     "int32",
			input:    int32(42),
			expected: float64Ptr(42.0),
		},
		{
			name:     "int64",
			input:    int64(42),
			expected: float64Ptr(42.0),
		},
		{
			name:     "string valid",
			input:    "43.65",
			expected: float64Ptr(43.65),
		},
		{
			name:     "string empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "string whitespace",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "string invalid",
			input:    "not-a-number",
			expected: nil,
		},
		{
			name:     "unsupported type bool",
			input:    true,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFloat(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", *result)
				}
			} else {
				if result == nil {
					t.Errorf("expected %v, got nil", *tt.expected)
				} else if *result != *tt.expected {
					t.Errorf("expected %v, got %v", *tt.expected, *result)
				}
			}
		})
	}
}

// Test getString
func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "string value",
			input:    "test",
			expected: "test",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "number",
			input:    42,
			expected: "",
		},
		{
			name:     "bool",
			input:    true,
			expected: "",
		},
		{
			name:     "map",
			input:    map[string]int{"key": 1},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test buildPlaceURI
func TestBuildPlaceURI(t *testing.T) {
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
			expected: "https://test.example.com/places/01HX1234567890ABCDEFGHJKMN",
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
			result := buildPlaceURI(tt.baseURL, tt.ulid)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test buildPlaceAddress
func TestBuildPlaceAddress(t *testing.T) {
	tests := []struct {
		name      string
		place     places.Place
		expectNil bool
	}{
		{
			name: "full address",
			place: places.Place{
				StreetAddress: "123 Main St",
				City:          "Toronto",
				Region:        "ON",
				PostalCode:    "M5H 2N2",
				Country:       "CA",
			},
			expectNil: false,
		},
		{
			name: "partial address",
			place: places.Place{
				City:   "Vancouver",
				Region: "BC",
			},
			expectNil: false,
		},
		{
			name:      "no address",
			place:     places.Place{},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPlaceAddress(tt.place)
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Error("expected non-nil result")
				} else {
					if result["@type"] != "PostalAddress" {
						t.Errorf("expected @type PostalAddress, got %v", result["@type"])
					}
				}
			}
		})
	}
}

// Test buildPlaceGeo
func TestBuildPlaceGeo(t *testing.T) {
	lat := 43.65
	lon := -79.38

	tests := []struct {
		name      string
		place     places.Place
		expectNil bool
	}{
		{
			name: "with coordinates",
			place: places.Place{
				Latitude:  &lat,
				Longitude: &lon,
			},
			expectNil: false,
		},
		{
			name: "missing latitude",
			place: places.Place{
				Longitude: &lon,
			},
			expectNil: true,
		},
		{
			name: "missing longitude",
			place: places.Place{
				Latitude: &lat,
			},
			expectNil: true,
		},
		{
			name:      "no coordinates",
			place:     places.Place{},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPlaceGeo(tt.place)
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if result == nil {
					t.Error("expected non-nil result")
				} else {
					if result["@type"] != "GeoCoordinates" {
						t.Errorf("expected @type GeoCoordinates, got %v", result["@type"])
					}
				}
			}
		})
	}
}

// Helper functions
func float64Ptr(v float64) *float64 {
	return &v
}
