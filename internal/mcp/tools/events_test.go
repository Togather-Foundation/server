package tools

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/organizations"
	"github.com/Togather-Foundation/server/internal/domain/places"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
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
			result := resolveEventLocation(context.Background(), tt.event, tt.baseURL, tt.resolver, zerolog.Nop())

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
			result := resolveEventOrganizer(context.Background(), "https://test.example.com", tt.orgID, tt.resolver, zerolog.Nop())

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
				ULID:      "01HX1234567890ABCDEFGHJKMN",
				Name:      "Event with Venue",
				PublicURL: "https://meetup.fixtures.togather.invalid/events/123",
				Synthetic: true,
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
			result := buildListItem(tt.event, tt.baseURL, nil, nil, nil, zerolog.Nop(), true)

			if result["@type"] != "Event" {
				t.Errorf("expected @type Event, got %v", result["@type"])
			}
			if result["name"] != tt.wantName {
				t.Errorf("expected name %q, got %v", tt.wantName, result["name"])
			}
			if tt.wantID != "" && result["@id"] != tt.wantID {
				t.Errorf("expected @id %q, got %v", tt.wantID, result["@id"])
			}
			if _, hasCtx := result["@context"]; !hasCtx {
				t.Error("expected @context on item in includeContext mode")
			}
			if got, present := result["url"]; present != (tt.event.PublicURL != "") || present && got != tt.event.PublicURL {
				t.Errorf("expected matching optional URL %q, got %v (present=%v)", tt.event.PublicURL, got, present)
			}
			if got, present := result["sel:synthetic"]; present != tt.event.Synthetic || present && got != true {
				t.Errorf("expected synthetic marker presence %v, got %v (present=%v)", tt.event.Synthetic, got, present)
			}
		})
	}
}

// TestBuildListItemOmitsContextInDocumentMode verifies that buildListItem with
// includeContext=false emits no per-item @context (the compact form used by the
// events tool's ?context=document mode).
func TestBuildListItemOmitsContextInDocumentMode(t *testing.T) {
	result := buildListItem(events.Event{ULID: "01HX1234567890ABCDEFGHJKMN", Name: "Test Event"}, "https://test.example.com", nil, nil, nil, zerolog.Nop(), false)

	if _, hasCtx := result["@context"]; hasCtx {
		t.Error("expected no per-item @context in document mode")
	}
	if result["@type"] != "Event" {
		t.Errorf("expected @type Event, got %v", result["@type"])
	}
}

// TestBuildEventPayloadIncludesProvenanceFields is the regression guard for
// #18: the MCP events(id) detail response must expose the same provenance
// fields the REST detail endpoint does — url, description, endDate — so MCP
// clients can verify an event's source without an N+1 REST fetch.
func TestBuildEventPayloadIncludesProvenanceFields(t *testing.T) {
	end := time.Date(2026, 7, 10, 21, 0, 0, 0, time.UTC)
	event := &events.Event{
		ULID:        "01HX1234567890ABCDEFGHJKMN",
		Name:        "Provenance Test",
		Description: "A test event with a resolvable source URL",
		PublicURL:   "https://meetup.fixtures.togather.invalid/events/123",
		Synthetic:   true,
		Occurrences: []events.Occurrence{
			{
				StartTime: time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC),
				EndTime:   &end,
			},
		},
	}

	payload := buildEventPayload(context.Background(), event, "https://test.example.com", nil, nil, nil, zerolog.Nop())

	if got, ok := payload["url"]; !ok || got != event.PublicURL {
		t.Errorf("expected url %q, got %v (present=%v)", event.PublicURL, got, ok)
	}
	if got := payload["sel:synthetic"]; got != true {
		t.Errorf("expected synthetic marker true, got %v", got)
	}
	if got, ok := payload["description"]; !ok || got != event.Description {
		t.Errorf("expected description %q, got %v (present=%v)", event.Description, got, ok)
	}
	if got, ok := payload["endDate"]; !ok || got != end.Format(time.RFC3339) {
		t.Errorf("expected endDate %q, got %v (present=%v)", end.Format(time.RFC3339), got, ok)
	}
}

// TestBuildEventPayloadOmitsEmptyOptionalFields verifies the same omission
// behavior as the REST detail handler for unset URL and provenance fields.
func TestBuildEventPayloadOmitsEmptyOptionalFields(t *testing.T) {
	event := &events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Minimal Event",
		Occurrences: []events.Occurrence{
			{StartTime: time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)},
		},
	}

	payload := buildEventPayload(context.Background(), event, "https://test.example.com", nil, nil, nil, zerolog.Nop())

	for _, field := range []string{"url", "description", "endDate", "sel:synthetic"} {
		if _, present := payload[field]; present {
			t.Errorf("expected %q to be omitted when empty, but it was present: %v", field, payload[field])
		}
	}
}

func torontoLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("America/Toronto must be loadable (tzdata required): %v", err)
	}
	return loc
}

// listStubRepo embeds events.Repository and overrides List so listEvents can be
// exercised without a full repository implementation.
type listStubRepo struct {
	events.Repository
}

func (listStubRepo) List(_ context.Context, _ events.Filters, _ events.Pagination) (events.ListResult, error) {
	start := time.Date(2026, 7, 10, 23, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC)
	return events.ListResult{
		Events: []events.Event{{
			Name:        "Summer Concert",
			Occurrences: []events.Occurrence{{StartTime: start, EndTime: &end}},
		}},
		NextCursor: "cursor",
	}, nil
}

// TestBuildListItemEmitsNodeTimeZoneAndEndDate verifies list items render
// startDate (and the previously-omitted endDate) in the node's local civil time.
func TestBuildListItemEmitsNodeTimeZoneAndEndDate(t *testing.T) {
	loc := torontoLoc(t)
	start := time.Date(2026, 7, 10, 23, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC)

	item := buildListItem(events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Summer Concert",
		Occurrences: []events.Occurrence{
			{StartTime: start, EndTime: &end},
		},
	}, "https://test.example.com", nil, nil, loc, zerolog.Nop(), true)

	if item["startDate"] != "2026-07-10T19:00:00-04:00" {
		t.Errorf("expected startDate %q, got %v", "2026-07-10T19:00:00-04:00", item["startDate"])
	}
	if item["endDate"] != "2026-07-10T21:00:00-04:00" {
		t.Errorf("expected endDate %q, got %v", "2026-07-10T21:00:00-04:00", item["endDate"])
	}
}

// TestBuildEventPayloadEmitsNodeTimeZone verifies the detail payload carries a
// timeZone field and a DST-correct startDate offset.
func TestBuildEventPayloadEmitsNodeTimeZone(t *testing.T) {
	loc := torontoLoc(t)
	start := time.Date(2026, 7, 10, 23, 0, 0, 0, time.UTC)

	payload := buildEventPayload(context.Background(), &events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Summer Concert",
		Occurrences: []events.Occurrence{
			{StartTime: start},
		},
	}, "https://test.example.com", nil, nil, loc, zerolog.Nop())

	if payload["timeZone"] != "America/Toronto" {
		t.Errorf("expected timeZone %q, got %v", "America/Toronto", payload["timeZone"])
	}
	if payload["startDate"] != "2026-07-10T19:00:00-04:00" {
		t.Errorf("expected startDate %q, got %v", "2026-07-10T19:00:00-04:00", payload["startDate"])
	}
}

// TestBuildListItemAllDayRendering verifies all-day occurrences emit a date-only
// startDate and allDay=true, while a timed midnight occurrence emits RFC 3339.
func TestBuildListItemAllDayRendering(t *testing.T) {
	loc := torontoLoc(t)
	midnight := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)

	allDayItem := buildListItem(events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Art Show",
		Occurrences: []events.Occurrence{
			{StartTime: midnight, IsAllDay: true, Timezone: "America/Toronto"},
		},
	}, "https://test.example.com", nil, nil, loc, zerolog.Nop(), true)

	if allDayItem["startDate"] != "2026-11-01" {
		t.Errorf("all-day startDate = %v, want %q", allDayItem["startDate"], "2026-11-01")
	}
	if allDayItem["allDay"] != true {
		t.Errorf("all-day marker = %v, want true", allDayItem["allDay"])
	}

	timedItem := buildListItem(events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Art Show",
		Occurrences: []events.Occurrence{
			{StartTime: midnight, Timezone: "America/Toronto"},
		},
	}, "https://test.example.com", nil, nil, loc, zerolog.Nop(), true)

	if timedItem["startDate"] != "2026-11-01T00:00:00-04:00" {
		t.Errorf("timed startDate = %v, want %q", timedItem["startDate"], "2026-11-01T00:00:00-04:00")
	}
	if _, ok := timedItem["allDay"]; ok {
		t.Error("timed occurrence must not emit allDay")
	}
}

// TestBuildEventPayloadAllDayRendering verifies the detail payload emits date-only
// values for all-day occurrences.
func TestBuildEventPayloadAllDayRendering(t *testing.T) {
	loc := torontoLoc(t)
	midnight := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)

	payload := buildEventPayload(context.Background(), &events.Event{
		ULID: "01HX1234567890ABCDEFGHJKMN",
		Name: "Art Show",
		Occurrences: []events.Occurrence{
			{StartTime: midnight, IsAllDay: true, Timezone: "America/Toronto"},
		},
	}, "https://test.example.com", nil, nil, loc, zerolog.Nop())

	if payload["startDate"] != "2026-11-01" {
		t.Errorf("startDate = %v, want %q", payload["startDate"], "2026-11-01")
	}
	if payload["allDay"] != true {
		t.Errorf("allDay = %v, want true", payload["allDay"])
	}
}

// TestListEventsEnvelopeTimeZone verifies the list response envelope advertises
// the node's IANA zone once and that items carry a DST-correct offset.
func TestListEventsEnvelopeTimeZone(t *testing.T) {
	loc := torontoLoc(t)
	svc := events.NewService(listStubRepo{})
	et := NewEventTools(svc, nil, "https://test.example.com").WithLoc(loc).WithLogger(zerolog.Nop())

	res, err := et.listEvents(context.Background(), "", "", "", "", "", "", 50, "", "document", nil, nil, nil)
	if err != nil {
		t.Fatalf("listEvents returned error: %v", err)
	}
	if res == nil {
		t.Fatal("listEvents returned nil result")
	}

	payload, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured content map, got %T", res.StructuredContent)
	}
	if payload["timeZone"] != "America/Toronto" {
		t.Errorf("expected envelope timeZone %q, got %v", "America/Toronto", payload["timeZone"])
	}

	items, ok := payload["items"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item, got %T %v", payload["items"], payload["items"])
	}
	item0 := items[0]
	if item0["startDate"] != "2026-07-10T19:00:00-04:00" {
		t.Errorf("expected item startDate %q, got %v", "2026-07-10T19:00:00-04:00", item0["startDate"])
	}
}

func TestEventsHandlerProximityInvalidInput(t *testing.T) {
	for _, args := range []map[string]any{
		{"near_lat": 0.0}, {"radius": 5.0}, {"near_lat": 0.0, "near_lon": 0.0, "radius": 101.0},
		{"nearLat": 0.0, "nearLon": 0.0, "radiusKm": "bad"}, {"near_lat": 0.0, "near_lon": 0.0, "radius": nil},
	} {
		et := NewEventTools(events.NewService(listStubRepo{}), nil, "https://example.org")
		res, err := callTool(context.Background(), et.EventsHandler, args)
		if err != nil || res == nil || !res.IsError {
			t.Errorf("arguments %v: expected tool error, got %#v, %v", args, res, err)
		}
	}
}

type proximityEventsRepo struct {
	events.Repository
	listFn func(events.Filters, events.Pagination) (events.ListResult, error)
}

func (r proximityEventsRepo) List(_ context.Context, filters events.Filters, pagination events.Pagination) (events.ListResult, error) {
	return r.listFn(filters, pagination)
}

func TestEventsHandlerProximityConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       map[string]any
		wantRadius float64
		wantError  bool
		max        float64
	}{
		{"zero coordinates", map[string]any{"near_lat": 0.0, "near_lon": 0.0}, 2.5, false, 7},
		{"explicit radius", map[string]any{"near_lat": 0.0, "near_lon": 0.0, "radius": 7.0}, 7, false, 7},
		{"aliases", map[string]any{"nearLat": 0.0, "nearLon": 0.0, "radiusKm": 6.0}, 6, false, 7},
		{"configured cap", map[string]any{"near_lat": 0.0, "near_lon": 0.0, "radius": 7.01}, 0, true, 7},
		{"malformed alias alongside canonical", map[string]any{"near_lat": 0.0, "near_lon": 0.0, "radius": 5.0, "radiusKm": "bad"}, 0, true, 7},
		{"null coordinate", map[string]any{"near_lat": nil, "near_lon": 0.0}, 0, true, 7},
		{"null alias alongside canonical", map[string]any{"near_lat": 0.0, "near_lon": 0.0, "radius": 5.0, "radiusKm": nil}, 0, true, 7},
		{"raised maximum", map[string]any{"near_lat": 0.0, "near_lon": 0.0, "radius": 200.0}, 200, false, 250},
		{"raised cap enforced", map[string]any{"near_lat": 0.0, "near_lon": 0.0, "radius": 251.0}, 0, true, 250},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			repo := proximityEventsRepo{listFn: func(filters events.Filters, _ events.Pagination) (events.ListResult, error) {
				called = true
				require.True(t, filters.PublicOnly)
				require.NotNil(t, filters.Latitude)
				require.NotNil(t, filters.Longitude)
				require.Equal(t, 0.0, *filters.Latitude)
				require.Equal(t, 0.0, *filters.Longitude)
				require.Equal(t, tc.wantRadius, *filters.RadiusKm)
				return events.ListResult{}, nil
			}}
			svc := events.NewService(repo).WithEventSearchConfig(config.EventSearchConfig{DefaultRadiusKM: 2.5, MaxRadiusKM: tc.max})
			et := NewEventTools(svc, nil, "https://example.org")
			result, err := callTool(context.Background(), et.EventsHandler, tc.args)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tc.wantError, result.IsError)
			require.Equal(t, !tc.wantError, called)
		})
	}
}

func TestEventsToolProximitySchema(t *testing.T) {
	tool := NewEventTools(nil, nil, "").EventsTool()
	for _, name := range []string{"near_lat", "near_lon", "radius"} {
		property, ok := tool.InputSchema.Properties[name].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "number", property["type"])
		require.NotContains(t, tool.InputSchema.Required, name)
	}
}

type proximityGetEventsRepo struct{ events.Repository }

func (proximityGetEventsRepo) GetPublicByULID(_ context.Context, id string) (*events.Event, error) {
	return &events.Event{ULID: id, Name: "Public event", LifecycleState: "published"}, nil
}

func TestEventsHandlerIDIgnoresProximityFilters(t *testing.T) {
	et := NewEventTools(events.NewService(proximityGetEventsRepo{}), nil, "https://example.org")
	result, err := callTool(context.Background(), et.EventsHandler, map[string]any{"id": "01HYX3KQW7ERTV9XNBM2P8QJZF", "near_lat": "invalid", "radius": nil})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
}
