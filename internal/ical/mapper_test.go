package ical

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// defaultMapperOpts returns MapperOptions for tests with sensible defaults.
func defaultMapperOpts() MapperOptions {
	return MapperOptions{
		SourceURL:  "https://example.com/feed.ics",
		SourceName: "test-source",
		TrustLevel: 5,
		License:    "CC0-1.0",
		Timezone:   "America/Toronto",
	}
}

func TestMapToEventInputs_BasicFieldMapping(t *testing.T) {
	t.Parallel()

	toronto, _ := time.LoadLocation("America/Toronto")
	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:            "basic-001",
				Summary:        "Community Meetup",
				Description:    "A great gathering",
				Location:       "Toronto Public Library",
				URL:            "https://example.com/meetup",
				Start:          time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:            time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				Organizer:      "Jane Smith",
				OrganizerEmail: "jane@example.com",
				Categories:     []string{"Community", "Social"},
				GeoLat:         43.6532,
				GeoLon:         -79.3832,
				HasGeo:         true,
				Status:         "CONFIRMED",
			},
		},
	}

	opts := defaultMapperOpts()
	_ = toronto

	results, warnings, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	ei := results[0]

	// Name
	if ei.Name != "Community Meetup" {
		t.Errorf("Name = %q, want %q", ei.Name, "Community Meetup")
	}

	// Description
	if ei.Description != "A great gathering" {
		t.Errorf("Description = %q, want %q", ei.Description, "A great gathering")
	}

	// StartDate / EndDate
	wantStart := "2026-04-15T19:00:00Z"
	if ei.StartDate != wantStart {
		t.Errorf("StartDate = %q, want %q", ei.StartDate, wantStart)
	}
	wantEnd := "2026-04-15T21:00:00Z"
	if ei.EndDate != wantEnd {
		t.Errorf("EndDate = %q, want %q", ei.EndDate, wantEnd)
	}

	// URL
	if ei.URL != "https://example.com/meetup" {
		t.Errorf("URL = %q", ei.URL)
	}

	// Location
	if ei.Location == nil {
		t.Fatal("Location is nil")
	}
	if ei.Location.Name != "Toronto Public Library" {
		t.Errorf("Location.Name = %q", ei.Location.Name)
	}
	if ei.Location.Latitude != 43.6532 {
		t.Errorf("Location.Latitude = %f", ei.Location.Latitude)
	}
	if ei.Location.Longitude != -79.3832 {
		t.Errorf("Location.Longitude = %f", ei.Location.Longitude)
	}

	// Organizer
	if ei.Organizer == nil {
		t.Fatal("Organizer is nil")
	}
	if ei.Organizer.Name != "Jane Smith" {
		t.Errorf("Organizer.Name = %q", ei.Organizer.Name)
	}
	if ei.Organizer.Email != "jane@example.com" {
		t.Errorf("Organizer.Email = %q", ei.Organizer.Email)
	}

	// Keywords
	if len(ei.Keywords) != 2 || ei.Keywords[0] != "Community" || ei.Keywords[1] != "Social" {
		t.Errorf("Keywords = %v, want [Community Social]", ei.Keywords)
	}

	// Source
	if ei.Source == nil {
		t.Fatal("Source is nil")
	}
	if ei.Source.EventID != "basic-001" {
		t.Errorf("Source.EventID = %q, want %q", ei.Source.EventID, "basic-001")
	}
	if ei.Source.URL != "https://example.com/feed.ics" {
		t.Errorf("Source.URL = %q", ei.Source.URL)
	}
	if ei.Source.Name != "test-source" {
		t.Errorf("Source.Name = %q", ei.Source.Name)
	}
	if ei.Source.License != "CC0-1.0" {
		t.Errorf("Source.License = %q", ei.Source.License)
	}

	// License
	if ei.License != "CC0-1.0" {
		t.Errorf("License = %q", ei.License)
	}
}

func TestMapToEventInputs_FloatingTimeWithFallback(t *testing.T) {
	t.Parallel()

	// Floating time: parsed in time.Local, should be re-interpreted in the
	// configured fallback timezone.
	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "float-001",
				Summary: "Floating Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.Local),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.Local),
			},
		},
	}

	opts := defaultMapperOpts()
	opts.Timezone = "America/Toronto"

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	ei := results[0]

	// The time should be 19:00 in America/Toronto, not UTC or Local.
	toronto, _ := time.LoadLocation("America/Toronto")
	wantStart := time.Date(2026, 4, 15, 19, 0, 0, 0, toronto).Format(time.RFC3339)
	if ei.StartDate != wantStart {
		t.Errorf("StartDate = %q, want %q", ei.StartDate, wantStart)
	}
}

func TestMapToEventInputs_AllDayEvent(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "allday-001",
				Summary: "Conference Day",
				Start:   time.Date(2026, 4, 15, 0, 0, 0, 0, time.Local),
				End:     time.Date(2026, 4, 16, 0, 0, 0, 0, time.Local),
				AllDay:  true,
			},
		},
	}

	opts := defaultMapperOpts()
	opts.Timezone = "America/Toronto"

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	ei := results[0]

	// All-day: floating time re-interpreted in source TZ.
	// Should produce midnight in America/Toronto as RFC 3339.
	toronto, _ := time.LoadLocation("America/Toronto")
	wantStart := time.Date(2026, 4, 15, 0, 0, 0, 0, toronto).Format(time.RFC3339)
	if ei.StartDate != wantStart {
		t.Errorf("StartDate = %q, want %q", ei.StartDate, wantStart)
	}
}

func TestMapToEventInputs_HTMLStripping(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:         "html-001",
				Summary:     "HTML <b>Event</b>",
				Description: "<h1>Welcome</h1><p>This is <strong>bold</strong>.</p><script>alert('xss')</script>",
				Start:       time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:         time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	ei := results[0]

	// sanitize.Text strips all HTML.
	if ei.Name != "HTML Event" {
		t.Errorf("Name = %q, want %q", ei.Name, "HTML Event")
	}
	// Description should have HTML tags stripped.
	if containsHTML(ei.Description) {
		t.Errorf("Description still contains HTML: %q", ei.Description)
	}
}

func TestMapToEventInputs_GEOParsing(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "geo-001",
				Summary: "GEO Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				GeoLat:  43.6532,
				GeoLon:  -79.3832,
				HasGeo:  true,
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Location == nil {
		t.Fatal("Location is nil")
	}
	if results[0].Location.Latitude != 43.6532 {
		t.Errorf("Latitude = %f, want 43.6532", results[0].Location.Latitude)
	}
	if results[0].Location.Longitude != -79.3832 {
		t.Errorf("Longitude = %f, want -79.3832", results[0].Location.Longitude)
	}
}

func TestMapToEventInputs_CategoriesMultiValueFiltering(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:        "cat-001",
				Summary:    "Category Event",
				Start:      time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:        time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				Categories: []string{"Music", "Jazz", "", "  ", "Blues"},
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Empty strings and whitespace-only should be filtered.
	wantKeywords := []string{"Music", "Jazz", "Blues"}
	if len(results[0].Keywords) != len(wantKeywords) {
		t.Fatalf("Keywords = %v, want %v", results[0].Keywords, wantKeywords)
	}
	for i, kw := range results[0].Keywords {
		if kw != wantKeywords[i] {
			t.Errorf("Keywords[%d] = %q, want %q", i, kw, wantKeywords[i])
		}
	}
}

func TestMapToEventInputs_CancelledSkipped(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "cancel-001",
				Summary: "Cancelled Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				Status:  "CANCELLED",
			},
			{
				UID:     "active-001",
				Summary: "Active Event",
				Start:   time.Date(2026, 4, 16, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 16, 21, 0, 0, 0, time.UTC),
				Status:  "CONFIRMED",
			},
		},
	}

	results, warnings, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancelled events are debug-logged, not warned.
	if len(results) != 1 {
		t.Fatalf("expected 1 result (cancelled skipped), got %d", len(results))
	}
	if results[0].Name != "Active Event" {
		t.Errorf("Name = %q, want %q", results[0].Name, "Active Event")
	}
	// No warnings for cancelled events.
	for _, w := range warnings {
		if contains(w, "CANCELLED") || contains(w, "cancel") {
			t.Errorf("unexpected warning about cancelled event: %s", w)
		}
	}
}

func TestMapToEventInputs_DefaultLocation(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "noloc-001",
				Summary: "No Location Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				// No Location, no GEO.
			},
		},
	}

	opts := defaultMapperOpts()
	opts.DefaultLocation = &events.PlaceInput{
		Name:            "Default Venue",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
	}

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Location == nil {
		t.Fatal("Location is nil, expected default")
	}
	if results[0].Location.Name != "Default Venue" {
		t.Errorf("Location.Name = %q, want %q", results[0].Location.Name, "Default Venue")
	}
}

func TestMapToEventInputs_URLNonHTTP(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "url-001",
				Summary: "FTP URL Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				URL:     "ftp://example.com/event",
			},
		},
	}

	results, warnings, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Non-http(s) URL should be skipped.
	if results[0].URL != "" {
		t.Errorf("URL = %q, want empty (non-http skipped)", results[0].URL)
	}
	if len(warnings) == 0 {
		t.Error("expected warning for non-http URL")
	}
}

func TestMapToEventInputs_WebcalNormalization(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "webcal-001",
				Summary: "Webcal Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				URL:     "webcal://example.com/events.ics",
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// webcal:// should be normalized to https://.
	if results[0].URL != "https://example.com/events.ics" {
		t.Errorf("URL = %q, want %q", results[0].URL, "https://example.com/events.ics")
	}
}

func TestMapToEventInputs_OrganizerCNAndEmail(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:            "org-001",
				Summary:        "Organizer Event",
				Start:          time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:            time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				Organizer:      "Jane Smith",
				OrganizerEmail: "jane@example.com",
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	org := results[0].Organizer
	if org == nil {
		t.Fatal("Organizer is nil")
	}
	if org.Name != "Jane Smith" {
		t.Errorf("Organizer.Name = %q", org.Name)
	}
	if org.Email != "jane@example.com" {
		t.Errorf("Organizer.Email = %q", org.Email)
	}
}

func TestMapToEventInputs_OrganizerNoCN(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:            "org-nocn-001",
				Summary:        "No CN Event",
				Start:          time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:            time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
				OrganizerEmail: "jane@example.com",
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	org := results[0].Organizer
	if org == nil {
		t.Fatal("Organizer is nil")
	}
	// Fallback: use email local-part as name.
	if org.Name != "jane" {
		t.Errorf("Organizer.Name = %q, want %q (email local-part fallback)", org.Name, "jane")
	}
}

func TestMapToEventInputs_DurationEndDate(t *testing.T) {
	t.Parallel()

	// Start + Duration computed by parse.go → End is already set.
	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:      "dur-001",
				Summary:  "Duration Event",
				Start:    time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:      time.Date(2026, 4, 15, 21, 30, 0, 0, time.UTC),
				Duration: 2*time.Hour + 30*time.Minute,
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantEnd := "2026-04-15T21:30:00Z"
	if results[0].EndDate != wantEnd {
		t.Errorf("EndDate = %q, want %q", results[0].EndDate, wantEnd)
	}
}

func TestMapToEventInputs_RecurrenceIDException(t *testing.T) {
	t.Parallel()

	// Master recurring event + exception that replaces one occurrence.
	masterStart := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	// Adjust to a Tuesday for weekly recurrence.
	for masterStart.Weekday() != time.Tuesday {
		masterStart = masterStart.AddDate(0, 0, 1)
	}

	exceptionOriginal := masterStart.AddDate(0, 0, 7)    // second occurrence
	exceptionNew := exceptionOriginal.Add(4 * time.Hour) // rescheduled 4h later

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "rec-001",
				Summary: "Weekly Meeting",
				Start:   masterStart,
				End:     masterStart.Add(time.Hour),
				RRULE:   "FREQ=WEEKLY;COUNT=3",
			},
			{
				UID:          "rec-001",
				Summary:      "Rescheduled Meeting",
				RecurrenceID: exceptionOriginal,
				Start:        exceptionNew,
				End:          exceptionNew.Add(time.Hour),
			},
		},
	}

	opts := defaultMapperOpts()
	opts.HorizonDays = 90

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 occurrences (COUNT=3), with the second replaced by exception.
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// All should have composite Source.EventID (UID:startDate).
	for _, r := range results {
		if r.Source.EventID == "rec-001" {
			t.Errorf("Source.EventID should be composite, got bare UID %q", r.Source.EventID)
		}
	}

	// Find the rescheduled one.
	var found bool
	for _, r := range results {
		if r.Name == "Rescheduled Meeting" {
			found = true
			wantStart := exceptionNew.Format(time.RFC3339)
			if r.StartDate != wantStart {
				t.Errorf("exception StartDate = %q, want %q", r.StartDate, wantStart)
			}
		}
	}
	if !found {
		t.Error("expected to find rescheduled meeting in results")
	}
}

func TestMapToEventInputs_RecurringCompositeEventID(t *testing.T) {
	t.Parallel()

	dtstart := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "daily-001",
				Summary: "Daily Standup",
				Start:   dtstart,
				End:     dtstart.Add(30 * time.Minute),
				RRULE:   "FREQ=DAILY;COUNT=3",
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Each should have UID:startDate composite.
	for i, r := range results {
		expectedStart := dtstart.AddDate(0, 0, i).Format(time.RFC3339)
		expectedID := "daily-001:" + expectedStart
		if r.Source.EventID != expectedID {
			t.Errorf("result %d: Source.EventID = %q, want %q", i, r.Source.EventID, expectedID)
		}
	}
}

func TestMapToEventInputs_DefaultLicense(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "lic-001",
				Summary: "License Test",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
			},
		},
	}

	opts := defaultMapperOpts()
	opts.License = "" // Should default to CC0-1.0.

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].License != "CC0-1.0" {
		t.Errorf("License = %q, want %q", results[0].License, "CC0-1.0")
	}
}

func TestMapToEventInputs_ContextCancellation(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: make([]ParsedEvent, 100),
	}
	for i := range cal.Events {
		cal.Events[i] = ParsedEvent{
			UID:     fmt.Sprintf("ctx-%03d", i),
			Summary: fmt.Sprintf("Event %d", i),
			Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
			End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	results, _, err := MapToEventInputs(ctx, cal, defaultMapperOpts())
	if err == nil {
		t.Fatal("expected context.Canceled error")
	}
	// May have partial results.
	_ = results
}

func TestMapToEventInputs_NoLocationNoDefault(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "noloc-001",
				Summary: "No Location",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
			},
		},
	}

	opts := defaultMapperOpts()
	opts.DefaultLocation = nil

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Location != nil {
		t.Errorf("Location = %+v, want nil", results[0].Location)
	}
}

// containsHTML checks if a string contains HTML-like tags.
func containsHTML(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			for j := i + 1; j < len(s); j++ {
				if s[j] == '>' {
					return true
				}
			}
		}
	}
	return false
}

// TestMapToEventInputs_RRULECappedWarning verifies that a warning is emitted
// when the MaxOccurrences cap truncates an RRULE expansion.
func TestMapToEventInputs_RRULECappedWarning(t *testing.T) {
	t.Parallel()

	// Create a recurring event that produces many occurrences (daily, no COUNT/UNTIL).
	dtstart := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "rrule-cap-test",
				Summary: "Daily Event",
				Start:   dtstart,
				End:     dtstart.Add(time.Hour),
				RRULE:   "FREQ=DAILY", // infinite daily — will hit cap
			},
		},
	}

	opts := defaultMapperOpts()
	opts.HorizonDays = 365  // large window
	opts.MaxOccurrences = 5 // small cap to trigger warning

	results, warnings, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce exactly MaxOccurrences events.
	if len(results) != 5 {
		t.Fatalf("expected 5 events (capped), got %d", len(results))
	}

	// Should contain the capped warning.
	found := false
	for _, w := range warnings {
		if len(w) > 0 && w[0] == 'R' {
			// Check for "RRULE capped at 5 occurrences"
			if fmt.Sprintf("RRULE capped at %d occurrences (UID: %s)", 5, "rrule-cap-test") == w {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected 'RRULE capped' warning, got warnings: %v", warnings)
	}
}

func TestDeriveSeriesEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rrule   string
		wantNil bool
		want    string
	}{
		{
			name:    "empty string returns nil",
			rrule:   "",
			wantNil: true,
		},
		{
			name:  "UNTIL with Z suffix",
			rrule: "FREQ=WEEKLY;UNTIL=20260831T235959Z",
			want:  "2026-08-31T23:59:59Z",
		},
		{
			name:  "UNTIL date-only",
			rrule: "FREQ=WEEKLY;UNTIL=20260831",
			want:  "2026-08-31T00:00:00Z",
		},
		{
			name:  "UNTIL local time without Z",
			rrule: "FREQ=WEEKLY;UNTIL=20260831T235959",
			want:  "2026-08-31T23:59:59Z",
		},
		{
			name:  "UNTIL with RFC3339 format",
			rrule: "FREQ=WEEKLY;UNTIL=2026-08-31T23:59:59Z",
			want:  "2026-08-31T23:59:59Z",
		},
		{
			name:    "COUNT-only returns nil",
			rrule:   "FREQ=WEEKLY;COUNT=10",
			wantNil: true,
		},
		{
			name:    "FREQ-only infinite returns nil",
			rrule:   "FREQ=DAILY",
			wantNil: true,
		},
		{
			name:  "UNTIL with RRULE: prefix",
			rrule: "RRULE:FREQ=WEEKLY;UNTIL=20260831T235959Z",
			want:  "2026-08-31T23:59:59Z",
		},
		{
			name:  "UNTIL before COUNT",
			rrule: "FREQ=WEEKLY;UNTIL=20260831T235959Z;COUNT=10",
			want:  "2026-08-31T23:59:59Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveSeriesEnd(tt.rrule)
			if tt.wantNil {
				if got != nil {
					t.Errorf("deriveSeriesEnd(%q) = %v, want nil", tt.rrule, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("deriveSeriesEnd(%q) = nil, want %s", tt.rrule, tt.want)
			}
			if got.Format(time.RFC3339) != tt.want {
				t.Errorf("deriveSeriesEnd(%q) = %v, want %s", tt.rrule, got.Format(time.RFC3339), tt.want)
			}
		})
	}
}

func TestMapToEventInputs_RecurrenceInput(t *testing.T) {
	t.Parallel()

	toronto, _ := time.LoadLocation("America/Toronto")
	masterStart := time.Date(2026, 7, 6, 19, 0, 0, 0, toronto)

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "weekly-workshop",
				Summary: "Weekly Workshop",
				Start:   masterStart,
				End:     masterStart.Add(2 * time.Hour),
				RRULE:   "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20260831T235959Z",
				ExDates: []time.Time{time.Date(2026, 7, 6, 19, 0, 0, 0, toronto)},
				TZID:    "America/Toronto",
			},
		},
	}

	opts := defaultMapperOpts()
	opts.HorizonDays = 90

	results, _, err := MapToEventInputs(context.Background(), cal, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	rec := results[0].Recurrence
	if rec == nil {
		t.Fatal("expected Recurrence to be non-nil for recurring event")
	}

	if rec.ExternalKey != "test-source:weekly-workshop" {
		t.Errorf("ExternalKey = %q, want %q", rec.ExternalKey, "test-source:weekly-workshop")
	}

	if rec.SeriesName != "Weekly Workshop" {
		t.Errorf("SeriesName = %q, want %q", rec.SeriesName, "Weekly Workshop")
	}

	if rec.RRule != "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20260831T235959Z" {
		t.Errorf("RRule = %q, want %q", rec.RRule, "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20260831T235959Z")
	}

	if rec.TZID != "America/Toronto" {
		t.Errorf("TZID = %q, want %q", rec.TZID, "America/Toronto")
	}

	if len(rec.ExDates) != 1 {
		t.Errorf("ExDates length = %d, want 1", len(rec.ExDates))
	}

	seriesStartYear := rec.SeriesStart.Year()
	if seriesStartYear != 2026 {
		t.Errorf("SeriesStart year = %d, want 2026", seriesStartYear)
	}

	if rec.SeriesEnd == nil {
		t.Fatal("SeriesEnd should not be nil when UNTIL is present")
	}
	if rec.SeriesEnd.Year() != 2026 {
		t.Errorf("SeriesEnd year = %d, want 2026", rec.SeriesEnd.Year())
	}

	// All occurrences should share the same RecurrenceInput pointer.
	for i := 1; i < len(results); i++ {
		if results[i].Recurrence != rec {
			t.Errorf("result %d: Recurrence is not the same pointer as result 0", i)
		}
	}
}

func TestMapToEventInputs_RecurrenceInput_CountOnly_NilEnd(t *testing.T) {
	t.Parallel()

	dtstart := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "count-only",
				Summary: "Count Only Series",
				Start:   dtstart,
				End:     dtstart.Add(time.Hour),
				RRULE:   "FREQ=WEEKLY;COUNT=5",
				TZID:    "UTC",
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	rec := results[0].Recurrence
	if rec == nil {
		t.Fatal("expected Recurrence to be non-nil")
	}

	if rec.SeriesEnd != nil {
		t.Errorf("SeriesEnd should be nil for COUNT-only series, got %v", rec.SeriesEnd)
	}
}

func TestMapToEventInputs_NonRecurring_NoRecurrence(t *testing.T) {
	t.Parallel()

	cal := &ParsedCalendar{
		Events: []ParsedEvent{
			{
				UID:     "single-001",
				Summary: "One-Off Event",
				Start:   time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				End:     time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC),
			},
		},
	}

	results, _, err := MapToEventInputs(context.Background(), cal, defaultMapperOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Recurrence != nil {
		t.Error("expected Recurrence to be nil for non-recurring event")
	}
}
