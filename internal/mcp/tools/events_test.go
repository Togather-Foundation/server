package tools

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
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

// mockPlaceResolver implements PlaceResolver for tests.
type mockPlaceResolver struct {
	places map[string]*places.Place
}

func (m *mockPlaceResolver) GetByULID(_ context.Context, ulid string) (*places.Place, error) {
	if p, ok := m.places[ulid]; ok {
		return p, nil
	}
	return nil, nil
}

// mockOrgResolver implements OrgResolver for tests.
type mockOrgResolver struct {
	orgs map[string]*organizations.Organization
}

func (m *mockOrgResolver) GetByULID(_ context.Context, ulid string) (*organizations.Organization, error) {
	if o, ok := m.orgs[ulid]; ok {
		return o, nil
	}
	return nil, nil
}

// Test resolveEventLocation
func TestResolveEventLocation(t *testing.T) {
	venueID := "01HX5678901234ABCDEFGHJKMN"
	virtualURL := "https://zoom.us/meeting"

	lat := 43.65
	lng := -79.38
	resolver := &mockPlaceResolver{
		places: map[string]*places.Place{
			venueID: {
				ULID:          venueID,
				Name:          "Test Venue",
				StreetAddress: "123 Main St",
				City:          "Toronto",
				Region:        "ON",
				PostalCode:    "M5V 1A1",
				Country:       "CA",
				Latitude:      &lat,
				Longitude:     &lng,
			},
		},
	}

	tests := []struct {
		name       string
		event      events.Event
		baseURL    string
		resolver   PlaceResolver
		expectNil  bool
		expectStr  string
		expectVirt bool
		expectVURL string
		expectEmb  bool // expect embedded Place object
	}{
		{
			name: "venue resolved to embedded Place",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Occurrences: []events.Occurrence{
					{VenueULID: &venueID},
				},
			},
			baseURL:   "https://test.example.com",
			resolver:  resolver,
			expectEmb: true,
		},
		{
			name: "venue falls back to URI without resolver",
			event: events.Event{
				ULID: "01HX1234567890ABCDEFGHJKMN",
				Occurrences: []events.Occurrence{
					{VenueULID: &venueID},
				},
			},
			baseURL:   "https://test.example.com",
			resolver:  nil,
			expectStr: "https://test.example.com/places/01HX5678901234ABCDEFGHJKMN",
		},
		{
			name: "primary venue resolved to embedded Place",
			event: events.Event{
				ULID:             "01HX1234567890ABCDEFGHJKMN",
				PrimaryVenueULID: &venueID,
			},
			baseURL:   "https://test.example.com",
			resolver:  resolver,
			expectEmb: true,
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
			result := resolveEventLocation(context.Background(), tt.event, tt.baseURL, tt.resolver)

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

			if tt.expectEmb {
				m, ok := result.(map[string]any)
				if !ok {
					t.Errorf("expected map, got %T: %v", result, result)
					return
				}
				if m["@type"] != "Place" {
					t.Errorf("expected @type Place, got %v", m["@type"])
				}
				if m["name"] != "Test Venue" {
					t.Errorf("expected name 'Test Venue', got %v", m["name"])
				}
				if m["@id"] != "https://test.example.com/places/01HX5678901234ABCDEFGHJKMN" {
					t.Errorf("expected @id URI, got %v", m["@id"])
				}
				addr, ok := m["address"].(map[string]any)
				if !ok {
					t.Fatal("expected address map")
				}
				if addr["streetAddress"] != "123 Main St" {
					t.Errorf("expected streetAddress '123 Main St', got %v", addr["streetAddress"])
				}
				geo, ok := m["geo"].(map[string]any)
				if !ok {
					t.Fatal("expected geo map")
				}
				if geo["latitude"] != 43.65 {
					t.Errorf("expected latitude 43.65, got %v", geo["latitude"])
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

// Test resolveEventOrganizer
func TestResolveEventOrganizer(t *testing.T) {
	orgID := "01HX9876543210ABCDEFGHJKMN"
	resolver := &mockOrgResolver{
		orgs: map[string]*organizations.Organization{
			orgID: {
				ULID:            orgID,
				Name:            "Test Org",
				URL:             "https://testorg.com",
				AddressLocality: "Toronto",
				AddressRegion:   "ON",
			},
		},
	}

	tests := []struct {
		name      string
		orgID     *string
		resolver  OrgResolver
		expectNil bool
		expectStr string
		expectEmb bool
	}{
		{
			name:      "nil org ID returns nil",
			orgID:     nil,
			resolver:  resolver,
			expectNil: true,
		},
		{
			name:      "empty org ID returns nil",
			orgID:     strPtr(""),
			resolver:  resolver,
			expectNil: true,
		},
		{
			name:      "resolved to embedded Organization",
			orgID:     &orgID,
			resolver:  resolver,
			expectEmb: true,
		},
		{
			name:      "falls back to URI without resolver",
			orgID:     &orgID,
			resolver:  nil,
			expectStr: "https://test.example.com/organizations/01HX9876543210ABCDEFGHJKMN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveEventOrganizer(context.Background(), "https://test.example.com", tt.orgID, tt.resolver)

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

			if tt.expectEmb {
				m, ok := result.(map[string]any)
				if !ok {
					t.Errorf("expected map, got %T: %v", result, result)
					return
				}
				if m["@type"] != "Organization" {
					t.Errorf("expected @type Organization, got %v", m["@type"])
				}
				if m["name"] != "Test Org" {
					t.Errorf("expected name 'Test Org', got %v", m["name"])
				}
				if m["url"] != "https://testorg.com" {
					t.Errorf("expected url 'https://testorg.com', got %v", m["url"])
				}
				addr, ok := m["address"].(map[string]any)
				if !ok {
					t.Fatal("expected address map")
				}
				if addr["addressLocality"] != "Toronto" {
					t.Errorf("expected addressLocality 'Toronto', got %v", addr["addressLocality"])
				}
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
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
			result := buildListItem(tt.event, tt.baseURL, nil, nil)

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
