package tools

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/places"
)

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
