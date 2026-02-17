package tools

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// Test buildEventURI
func TestBuildEventURI(t *testing.T) {
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
			expected: "https://test.example.com/events/01HX1234567890ABCDEFGHJKMN",
		},
		{
			name:     "empty baseURL returns empty",
			baseURL:  "",
			ulid:     "01HX1234567890ABCDEFGHJKMN",
			expected: "",
		},
		{
			name:     "empty ulid returns empty",
			baseURL:  "https://test.example.com",
			ulid:     "",
			expected: "",
		},
		{
			name:     "both empty returns empty",
			baseURL:  "",
			ulid:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEventURI(tt.baseURL, tt.ulid)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test addMCPProvenance
func TestAddMCPProvenance(t *testing.T) {
	tests := []struct {
		name     string
		input    events.EventInput
		baseURL  string
		validate func(t *testing.T, result events.EventInput)
	}{
		{
			name:    "adds default source name when nil",
			input:   events.EventInput{},
			baseURL: "https://test.example.com",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source == nil {
					t.Fatal("expected source to be non-nil")
				}
				if result.Source.Name != "mcp-agent" {
					t.Errorf("expected source name 'mcp-agent', got %q", result.Source.Name)
				}
			},
		},
		{
			name: "preserves existing source name",
			input: events.EventInput{
				Source: &events.SourceInput{Name: "custom-agent"},
			},
			baseURL: "https://test.example.com",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source.Name != "custom-agent" {
					t.Errorf("expected source name 'custom-agent', got %q", result.Source.Name)
				}
			},
		},
		{
			name: "does not override non-empty source name",
			input: events.EventInput{
				Source: &events.SourceInput{Name: "   existing   "},
			},
			baseURL: "https://test.example.com",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source.Name != "   existing   " {
					t.Errorf("expected source name '   existing   ', got %q", result.Source.Name)
				}
			},
		},
		{
			name:    "sets source URL from baseURL",
			input:   events.EventInput{},
			baseURL: "https://test.example.com",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source.URL != "https://test.example.com" {
					t.Errorf("expected source URL 'https://test.example.com', got %q", result.Source.URL)
				}
			},
		},
		{
			name:    "uses fallback URL when baseURL empty",
			input:   events.EventInput{},
			baseURL: "",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source.URL != "https://mcp-agent" {
					t.Errorf("expected source URL 'https://mcp-agent', got %q", result.Source.URL)
				}
			},
		},
		{
			name:    "generates event ID when empty",
			input:   events.EventInput{},
			baseURL: "https://test.example.com",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source.EventID == "" {
					t.Error("expected non-empty event ID")
				}
				// Check that it's a valid ULID format (26 chars)
				if len(result.Source.EventID) != 26 {
					t.Errorf("expected 26-character ULID, got %d characters", len(result.Source.EventID))
				}
			},
		},
		{
			name: "preserves existing event ID",
			input: events.EventInput{
				Source: &events.SourceInput{EventID: "existing-id"},
			},
			baseURL: "https://test.example.com",
			validate: func(t *testing.T, result events.EventInput) {
				if result.Source.EventID != "existing-id" {
					t.Errorf("expected event ID 'existing-id', got %q", result.Source.EventID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addMCPProvenance(tt.input, tt.baseURL)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// Test buildEventLocation
func TestBuildEventLocation(t *testing.T) {
	venueID := "01HX5678901234ABCDEFGHJKMN"
	virtualURL := "https://zoom.us/meeting"

	tests := []struct {
		name       string
		event      events.Event
		baseURL    string
		expectNil  bool
		expectStr  string
		expectVirt bool
		expectVURL string
	}{
		{
			name: "venue ID in occurrence",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Occurrences: []events.Occurrence{
					{VenueULID: &venueID},
				},
			},
			baseURL:   "https://test.example.com",
			expectStr: "https://test.example.com/places/01HX5678901234ABCDEFGHJKMN",
		},
		{
			name: "primary venue ID",
			event: events.Event{
				ULID:             "01HX1234567890ABCDEFGHJKMN",
				PrimaryVenueULID: &venueID,
			},
			baseURL:   "https://test.example.com",
			expectStr: "https://test.example.com/places/01HX5678901234ABCDEFGHJKMN",
		},
		{
			name: "virtual URL in occurrence",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Occurrences: []events.Occurrence{
					{VirtualURL: &virtualURL},
				},
			},
			baseURL:    "https://test.example.com",
			expectVirt: true,
			expectVURL: virtualURL,
		},
		{
			name: "virtual URL at top level",
			event: events.Event{
				ULID:       "01HX1234567890ABCDEFGHJKMN",
				VirtualURL: virtualURL,
			},
			baseURL:    "https://test.example.com",
			expectVirt: true,
			expectVURL: virtualURL,
		},
		{
			name: "no location returns nil",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
			},
			baseURL:   "https://test.example.com",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEventLocation(tt.event, tt.baseURL)

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if tt.expectStr != "" {
				str, ok := result.(string)
				if !ok {
					t.Errorf("expected string, got %T: %v", result, result)
					return
				}
				if str != tt.expectStr {
					t.Errorf("expected %q, got %q", tt.expectStr, str)
				}
			}

			if tt.expectVirt {
				m, ok := result.(map[string]any)
				if !ok {
					t.Errorf("expected map, got %T", result)
					return
				}
				if m["@type"] != "VirtualLocation" {
					t.Errorf("expected @type VirtualLocation, got %v", m["@type"])
				}
				if m["url"] != tt.expectVURL {
					t.Errorf("expected url %q, got %v", tt.expectVURL, m["url"])
				}
			}
		})
	}
}

// Test buildListItem
func TestBuildListItem(t *testing.T) {
	venueID := "01HX5678901234ABCDEFGHJKMN"

	tests := []struct {
		name     string
		event    events.Event
		baseURL  string
		wantID   string
		wantName string
	}{
		{
			name: "basic event",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Name: "Test Event",
			},
			baseURL:  "https://test.example.com",
			wantID:   "https://test.example.com/events/01HX1234567890ABCDEFGHJKMN",
			wantName: "Test Event",
		},
		{
			name: "event with occurrence",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Name: "Event with Venue",
				Occurrences: []events.Occurrence{
					{VenueID: &venueID},
				},
			},
			baseURL:  "https://test.example.com",
			wantID:   "https://test.example.com/events/01HX1234567890ABCDEFGHJKMN",
			wantName: "Event with Venue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildListItem(tt.event, tt.baseURL)

			if result["@type"] != "Event" {
				t.Errorf("expected @type Event, got %v", result["@type"])
			}
			if result["name"] != tt.wantName {
				t.Errorf("expected name %q, got %v", tt.wantName, result["name"])
			}
			if tt.wantID != "" && result["@id"] != tt.wantID {
				t.Errorf("expected @id %q, got %v", tt.wantID, result["@id"])
			}
		})
	}
}
