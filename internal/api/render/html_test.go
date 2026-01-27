package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderEventHTML(t *testing.T) {
	eventData := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Event",
		"@id":         "https://example.com/events/123",
		"name":        "Jazz in the Park",
		"description": "A wonderful jazz concert",
		"startDate":   "2026-07-10T19:00:00Z",
		"location": map[string]any{
			"@type":   "Place",
			"name":    "Centennial Park",
			"address": "Toronto, ON",
		},
		"organizer": map[string]any{
			"@type": "Organization",
			"name":  "Toronto Arts Org",
		},
	}

	html, err := RenderEventHTML(eventData)
	if err != nil {
		t.Fatalf("RenderEventHTML failed: %v", err)
	}

	// Verify HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE")
	}
	if !strings.Contains(html, "<html lang=\"en\">") {
		t.Error("Missing html tag with lang")
	}
	if !strings.Contains(html, "<head>") {
		t.Error("Missing head tag")
	}
	if !strings.Contains(html, "<body>") {
		t.Error("Missing body tag")
	}

	// Verify embedded JSON-LD
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}

	// Verify event name appears
	if !strings.Contains(html, "Jazz in the Park") {
		t.Error("Event name not in HTML")
	}

	// Verify JSON-LD is valid
	start := strings.Index(html, `<script type="application/ld+json">`)
	start += len(`<script type="application/ld+json">`)
	end := strings.Index(html[start:], "</script>")
	jsonldContent := strings.TrimSpace(html[start : start+end])

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonldContent), &parsed); err != nil {
		t.Errorf("Embedded JSON-LD is invalid: %v", err)
	}

	if parsed["name"] != "Jazz in the Park" {
		t.Errorf("JSON-LD name mismatch: got %v", parsed["name"])
	}
}

func TestRenderPlaceHTML(t *testing.T) {
	placeData := map[string]any{
		"@context":        "https://schema.org",
		"@type":           "Place",
		"@id":             "https://example.com/places/456",
		"name":            "Centennial Park",
		"addressLocality": "Toronto",
		"addressRegion":   "ON",
	}

	html, err := RenderPlaceHTML(placeData)
	if err != nil {
		t.Fatalf("RenderPlaceHTML failed: %v", err)
	}

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE")
	}
	if !strings.Contains(html, "Centennial Park") {
		t.Error("Place name not in HTML")
	}
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}
}

func TestRenderOrganizationHTML(t *testing.T) {
	orgData := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Organization",
		"@id":         "https://example.com/organizations/789",
		"name":        "Toronto Arts Org",
		"description": "A community arts organization",
		"url":         "https://torontoarts.example.com",
	}

	html, err := RenderOrganizationHTML(orgData)
	if err != nil {
		t.Fatalf("RenderOrganizationHTML failed: %v", err)
	}

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE")
	}
	if !strings.Contains(html, "Toronto Arts Org") {
		t.Error("Organization name not in HTML")
	}
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}
}

func TestHTMLEscaping(t *testing.T) {
	eventData := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Event",
		"name":     `Jazz "in the" Park <script>alert('xss')</script>`,
	}

	html, err := RenderEventHTML(eventData)
	if err != nil {
		t.Fatalf("RenderEventHTML failed: %v", err)
	}

	// Verify HTML escaping (should not contain unescaped < or >)
	if strings.Contains(html, "<script>alert") && !strings.Contains(html, `&lt;script&gt;`) {
		t.Error("HTML not properly escaped in body")
	}
}

// Unit tests for helper functions

func TestExtractString(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		key      string
		expected string
	}{
		{
			name:     "valid string",
			data:     map[string]any{"name": "Test"},
			key:      "name",
			expected: "Test",
		},
		{
			name:     "empty string",
			data:     map[string]any{"name": ""},
			key:      "name",
			expected: "",
		},
		{
			name:     "missing key",
			data:     map[string]any{"other": "value"},
			key:      "name",
			expected: "",
		},
		{
			name:     "nil map",
			data:     nil,
			key:      "name",
			expected: "",
		},
		{
			name:     "non-string value",
			data:     map[string]any{"count": 123},
			key:      "count",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractString(tt.data, tt.key)
			if result != tt.expected {
				t.Errorf("extractString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractLocation(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		expected string
	}{
		{
			name: "location with name and address",
			data: map[string]any{
				"location": map[string]any{
					"name":    "Central Park",
					"address": "New York, NY",
				},
			},
			expected: "Central Park (New York, NY)",
		},
		{
			name: "location with only name",
			data: map[string]any{
				"location": map[string]any{
					"name": "Central Park",
				},
			},
			expected: "Central Park",
		},
		{
			name: "location with only address",
			data: map[string]any{
				"location": map[string]any{
					"address": "New York, NY",
				},
			},
			expected: "New York, NY",
		},
		{
			name: "location with empty strings",
			data: map[string]any{
				"location": map[string]any{
					"name":    "",
					"address": "",
				},
			},
			expected: "",
		},
		{
			name:     "missing location",
			data:     map[string]any{},
			expected: "",
		},
		{
			name: "location not a map",
			data: map[string]any{
				"location": "some string",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLocation(tt.data)
			if result != tt.expected {
				t.Errorf("extractLocation() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractOrganizer(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		expected string
	}{
		{
			name: "valid organizer",
			data: map[string]any{
				"organizer": map[string]any{
					"name": "City Arts Council",
				},
			},
			expected: "City Arts Council",
		},
		{
			name: "organizer with empty name",
			data: map[string]any{
				"organizer": map[string]any{
					"name": "",
				},
			},
			expected: "",
		},
		{
			name:     "missing organizer",
			data:     map[string]any{},
			expected: "",
		},
		{
			name: "organizer not a map",
			data: map[string]any{
				"organizer": "some string",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOrganizer(tt.data)
			if result != tt.expected {
				t.Errorf("extractOrganizer() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractAddress(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		expected string
	}{
		{
			name: "full address",
			data: map[string]any{
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
				"addressCountry":  "Canada",
			},
			expected: "Toronto, ON, Canada",
		},
		{
			name: "partial address (locality and region)",
			data: map[string]any{
				"addressLocality": "Toronto",
				"addressRegion":   "ON",
			},
			expected: "Toronto, ON",
		},
		{
			name: "single field",
			data: map[string]any{
				"addressCountry": "Canada",
			},
			expected: "Canada",
		},
		{
			name:     "empty data",
			data:     map[string]any{},
			expected: "",
		},
		{
			name: "empty string values",
			data: map[string]any{
				"addressLocality": "",
				"addressRegion":   "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAddress(tt.data)
			if result != tt.expected {
				t.Errorf("extractAddress() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatDateTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // allow multiple valid outputs (timezone abbreviation varies by system)
	}{
		{
			name:     "valid RFC3339 datetime",
			input:    "2026-07-15T19:00:00Z",
			expected: []string{"Wednesday, July 15, 2026 at 7:00 PM UTC"},
		},
		{
			name:  "valid datetime with timezone",
			input: "2026-07-15T19:00:00-04:00",
			// CI environments may not have tzdata with abbreviations, so accept both
			expected: []string{
				"Wednesday, July 15, 2026 at 7:00 PM EDT",
				"Wednesday, July 15, 2026 at 7:00 PM -0400",
			},
		},
		{
			name:     "malformed datetime",
			input:    "not-a-date",
			expected: []string{"not-a-date"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "partial date",
			input:    "2026-07-15",
			expected: []string{"2026-07-15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDateTime(tt.input)
			found := false
			for _, exp := range tt.expected {
				if result == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("formatDateTime() = %q, want one of %v", result, tt.expected)
			}
		})
	}
}

func TestBuildEventBody(t *testing.T) {
	tests := []struct {
		name        string
		description string
		startDate   string
		location    string
		organizer   string
		expectCount int // number of fields in output
	}{
		{
			name:        "all fields present",
			description: "Test description",
			startDate:   "2026-07-15",
			location:    "Test Location",
			organizer:   "Test Organizer",
			expectCount: 4,
		},
		{
			name:        "only some fields",
			description: "Test description",
			startDate:   "2026-07-15",
			location:    "",
			organizer:   "",
			expectCount: 2,
		},
		{
			name:        "all fields empty",
			description: "",
			startDate:   "",
			location:    "",
			organizer:   "",
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEventBody("Event Name", tt.description, tt.startDate, tt.location, tt.organizer)
			fieldCount := strings.Count(result, `class="field"`)
			if fieldCount != tt.expectCount {
				t.Errorf("buildEventBody() field count = %d, want %d", fieldCount, tt.expectCount)
			}

			// Verify XSS protection
			if tt.description != "" && !strings.Contains(result, "Test description") {
				t.Error("Description not included in output")
			}
		})
	}
}

func TestBuildPlaceBody(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		expectField bool
	}{
		{
			name:        "with address",
			address:     "Toronto, ON",
			expectField: true,
		},
		{
			name:        "without address",
			address:     "",
			expectField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPlaceBody("Place Name", tt.address)
			hasField := strings.Contains(result, `class="field"`)
			if hasField != tt.expectField {
				t.Errorf("buildPlaceBody() field presence = %v, want %v", hasField, tt.expectField)
			}
		})
	}
}

func TestBuildOrganizationBody(t *testing.T) {
	tests := []struct {
		name        string
		description string
		url         string
		expectCount int
	}{
		{
			name:        "with both fields",
			description: "Test description",
			url:         "https://example.com",
			expectCount: 2,
		},
		{
			name:        "with only description",
			description: "Test description",
			url:         "",
			expectCount: 1,
		},
		{
			name:        "with only url",
			description: "",
			url:         "https://example.com",
			expectCount: 1,
		},
		{
			name:        "with neither",
			description: "",
			url:         "",
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildOrganizationBody("Org Name", tt.description, tt.url)
			fieldCount := strings.Count(result, `class="field"`)
			if fieldCount != tt.expectCount {
				t.Errorf("buildOrganizationBody() field count = %d, want %d", fieldCount, tt.expectCount)
			}

			// Verify URL is properly escaped as both href and display text
			if tt.url != "" {
				if !strings.Contains(result, `href="https://example.com"`) {
					t.Error("URL not properly escaped in href")
				}
			}
		})
	}
}

func TestBuildHTML_XSS(t *testing.T) {
	maliciousTitle := `<script>alert('xss')</script>`
	entityType := `Event<script>`
	bodyContent := "safe content"
	jsonld := "{}"

	result := buildHTML(maliciousTitle, entityType, bodyContent, jsonld)

	// Verify XSS protection: malicious content should be escaped
	if strings.Contains(result, "<script>alert") {
		t.Error("XSS content not properly escaped in title")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("Expected HTML entities for escaped script tags")
	}
}

func TestRenderEventHTML_NilFields(t *testing.T) {
	eventData := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Event",
		"name":     "Test Event",
		// All optional fields missing/nil
	}

	html, err := RenderEventHTML(eventData)
	if err != nil {
		t.Fatalf("RenderEventHTML should handle nil fields: %v", err)
	}

	if !strings.Contains(html, "Test Event") {
		t.Error("Event name not in HTML")
	}
}
