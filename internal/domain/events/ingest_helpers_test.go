package events

import (
	"testing"
)

func TestPrimaryVenueKey(t *testing.T) {
	tests := []struct {
		name     string
		input    EventInput
		expected string
	}{
		{
			name: "physical location with ID",
			input: EventInput{
				Location: &PlaceInput{ID: "place-123", Name: "Convention Center"},
			},
			expected: "place-123",
		},
		{
			name: "physical location without ID",
			input: EventInput{
				Location: &PlaceInput{Name: "Convention Center"},
			},
			expected: "Convention Center",
		},
		{
			name: "virtual location",
			input: EventInput{
				VirtualLocation: &VirtualLocationInput{URL: "https://zoom.us/j/123456"},
			},
			expected: "https://zoom.us/j/123456",
		},
		{
			name:     "no location",
			input:    EventInput{},
			expected: "",
		},
		{
			name: "prefers physical over virtual",
			input: EventInput{
				Location:        &PlaceInput{ID: "place-123", Name: "Center"},
				VirtualLocation: &VirtualLocationInput{URL: "https://zoom.us/j/123456"},
			},
			expected: "place-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := primaryVenueKey(tt.input)
			if result != tt.expected {
				t.Errorf("primaryVenueKey() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLocationID(t *testing.T) {
	tests := []struct {
		name     string
		input    *PlaceInput
		expected string
	}{
		{
			name:     "nil place",
			input:    nil,
			expected: "",
		},
		{
			name:     "place with ID",
			input:    &PlaceInput{ID: "place-123"},
			expected: "place-123",
		},
		{
			name:     "place with whitespace ID",
			input:    &PlaceInput{ID: "  place-456  "},
			expected: "place-456",
		},
		{
			name:     "place without ID",
			input:    &PlaceInput{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := locationID(tt.input)
			if result != tt.expected {
				t.Errorf("locationID() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestVirtualURL(t *testing.T) {
	tests := []struct {
		name     string
		input    EventInput
		expected string
	}{
		{
			name:     "no virtual location",
			input:    EventInput{},
			expected: "",
		},
		{
			name: "with virtual location",
			input: EventInput{
				VirtualLocation: &VirtualLocationInput{URL: "https://meet.google.com/abc"},
			},
			expected: "https://meet.google.com/abc",
		},
		{
			name: "virtual location with whitespace",
			input: EventInput{
				VirtualLocation: &VirtualLocationInput{URL: "  https://zoom.us/j/123  "},
			},
			expected: "https://zoom.us/j/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := virtualURL(tt.input)
			if result != tt.expected {
				t.Errorf("virtualURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSourceName(t *testing.T) {
	tests := []struct {
		name     string
		source   *SourceInput
		fallback string
		expected string
	}{
		{
			name:     "nil source, no fallback",
			source:   nil,
			fallback: "",
			expected: "unknown",
		},
		{
			name:     "nil source, with fallback",
			source:   nil,
			fallback: "My Event",
			expected: "My Event",
		},
		{
			name:     "source with name",
			source:   &SourceInput{Name: "EventBrite", EventID: "evt-123"},
			fallback: "Fallback",
			expected: "EventBrite",
		},
		{
			name:     "source without name but with eventID",
			source:   &SourceInput{EventID: "evt-123"},
			fallback: "Fallback",
			expected: "evt-123",
		},
		{
			name:     "source with empty strings",
			source:   &SourceInput{Name: "   ", EventID: "   "},
			fallback: "Fallback",
			expected: "Fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sourceName(tt.source, tt.fallback)
			if result != tt.expected {
				t.Errorf("sourceName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSourceLicense(t *testing.T) {
	tests := []struct {
		name     string
		input    EventInput
		expected string
	}{
		{
			name:     "no source, no license",
			input:    EventInput{},
			expected: "",
		},
		{
			name:     "top-level license only",
			input:    EventInput{License: "CC0-1.0"},
			expected: "CC0-1.0",
		},
		{
			name: "source license overrides top-level",
			input: EventInput{
				License: "CC0-1.0",
				Source:  &SourceInput{License: "CC-BY-4.0"},
			},
			expected: "CC-BY-4.0",
		},
		{
			name: "source with empty license falls back to top-level",
			input: EventInput{
				License: "CC0-1.0",
				Source:  &SourceInput{License: "   "},
			},
			expected: "CC0-1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sourceLicense(tt.input)
			if result != tt.expected {
				t.Errorf("sourceLicense() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSourceLicenseType(t *testing.T) {
	tests := []struct {
		name     string
		input    EventInput
		expected string
	}{
		{
			name:     "empty license",
			input:    EventInput{},
			expected: "unknown",
		},
		{
			name:     "CC0 short form",
			input:    EventInput{License: "CC0"},
			expected: "CC0",
		},
		{
			name:     "CC0-1.0",
			input:    EventInput{License: "CC0-1.0"},
			expected: "CC0",
		},
		{
			name:     "CC0 URL",
			input:    EventInput{License: "https://creativecommons.org/publicdomain/zero/1.0/"},
			expected: "CC0",
		},
		{
			name:     "CC-BY URL",
			input:    EventInput{License: "https://creativecommons.org/licenses/by/4.0/"},
			expected: "CC-BY",
		},
		{
			name:     "CC-BY short form",
			input:    EventInput{License: "cc-by-4.0"},
			expected: "CC-BY",
		},
		{
			name:     "unknown license",
			input:    EventInput{License: "Proprietary"},
			expected: "unknown",
		},
		{
			name: "case insensitive",
			input: EventInput{
				Source: &SourceInput{License: "Cc0-1.0"},
			},
			expected: "CC0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sourceLicenseType(tt.input)
			if result != tt.expected {
				t.Errorf("sourceLicenseType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFallbackOrUnknown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "unknown",
		},
		{
			name:     "valid value",
			input:    "My Value",
			expected: "My Value",
		},
		{
			name:     "value with whitespace",
			input:    "  My Value  ",
			expected: "My Value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fallbackOrUnknown(tt.input)
			if result != tt.expected {
				t.Errorf("fallbackOrUnknown() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSourceBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "full URL",
			input:    "https://example.com/events/123",
			expected: "https://example.com",
		},
		{
			name:     "URL with port",
			input:    "http://localhost:3000/api/events",
			expected: "http://localhost:3000",
		},
		{
			name:     "invalid URL returns as-is",
			input:    "not-a-url",
			expected: "not-a-url",
		},
		{
			name:     "URL without scheme",
			input:    "example.com/path",
			expected: "example.com/path",
		},
		{
			name:     "URL with query params",
			input:    "https://api.eventbrite.com/v3/events?id=123",
			expected: "https://api.eventbrite.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sourceBaseURL(tt.input)
			if result != tt.expected {
				t.Errorf("sourceBaseURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLicenseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string defaults to CC0",
			input:    "",
			expected: "https://creativecommons.org/publicdomain/zero/1.0/",
		},
		{
			name:     "whitespace defaults to CC0",
			input:    "   ",
			expected: "https://creativecommons.org/publicdomain/zero/1.0/",
		},
		{
			name:     "custom license URL",
			input:    "https://creativecommons.org/licenses/by/4.0/",
			expected: "https://creativecommons.org/licenses/by/4.0/",
		},
		{
			name:     "short license identifier",
			input:    "CC0-1.0",
			expected: "CC0-1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := licenseURL(tt.input)
			if result != tt.expected {
				t.Errorf("licenseURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNullableString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "valid string",
			input:    "Hello",
			expected: stringPtr("Hello"),
		},
		{
			name:     "string with leading/trailing whitespace",
			input:    "  Hello  ",
			expected: stringPtr("Hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nullableString(tt.input)
			if (result == nil) != (tt.expected == nil) {
				t.Errorf("nullableString() nil mismatch: got %v, want %v", result, tt.expected)
				return
			}
			if result != nil && tt.expected != nil && *result != *tt.expected {
				t.Errorf("nullableString() = %v, want %v", *result, *tt.expected)
			}
		})
	}
}

// Helper function for tests
func stringPtr(s string) *string {
	return &s
}
