package scraper

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// testSource is a reusable SourceConfig for tests.
var testSource = SourceConfig{
	Name:    "Test Source",
	URL:     "https://example.com",
	License: "CC0-1.0",
}

// mustJSON is a test helper that marshals v to json.RawMessage or panics.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}

// TestNormalizeJSONLDEvent covers the main NormalizeJSONLDEvent function.
func TestNormalizeJSONLDEvent(t *testing.T) {
	t.Parallel()

	boolTrue := true
	boolFalse := false

	tests := []struct {
		name       string
		raw        string
		source     SourceConfig
		wantErr    bool
		wantErrMsg string
		check      func(t *testing.T, got events.EventInput)
	}{
		{
			name: "complete event with Place location",
			raw: `{
				"@type": "Event",
				"@id": "https://example.com/events/1",
				"name": "Jazz Night",
				"description": "A wonderful jazz evening",
				"startDate": "2026-03-15T20:00:00",
				"endDate": "2026-03-15T23:00:00",
				"doorTime": "2026-03-15T19:30:00",
				"location": {
					"@type": "Place",
					"name": "Massey Hall",
					"address": {
						"@type": "PostalAddress",
						"streetAddress": "178 Victoria St",
						"addressLocality": "Toronto",
						"addressRegion": "ON",
						"postalCode": "M5B 1T7",
						"addressCountry": "CA"
					},
					"geo": {
						"@type": "GeoCoordinates",
						"latitude": 43.6535,
						"longitude": -79.3796
					}
				},
				"organizer": {
					"@type": "Organization",
					"name": "Live Nation",
					"url": "https://livenation.com"
				},
				"image": "https://example.com/image.jpg",
				"url": "https://example.com/events/1",
				"offers": {
					"@type": "Offer",
					"price": "25",
					"priceCurrency": "CAD",
					"url": "https://example.com/tickets/1"
				},
				"keywords": ["jazz", "live music"],
				"inLanguage": "en",
				"isAccessibleForFree": false,
				"sameAs": "https://eventbrite.com/e/123"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Name != "Jazz Night" {
					t.Errorf("Name = %q, want %q", got.Name, "Jazz Night")
				}
				if got.Description != "A wonderful jazz evening" {
					t.Errorf("Description = %q", got.Description)
				}
				if got.StartDate != "2026-03-15T20:00:00" {
					t.Errorf("StartDate = %q", got.StartDate)
				}
				if got.EndDate != "2026-03-15T23:00:00" {
					t.Errorf("EndDate = %q", got.EndDate)
				}
				if got.DoorTime != "2026-03-15T19:30:00" {
					t.Errorf("DoorTime = %q", got.DoorTime)
				}
				if got.Type != "Event" {
					t.Errorf("Type = %q, want Event", got.Type)
				}
				if got.Image != "https://example.com/image.jpg" {
					t.Errorf("Image = %q", got.Image)
				}
				if got.URL != "https://example.com/events/1" {
					t.Errorf("URL = %q", got.URL)
				}
				if got.Location == nil {
					t.Fatal("Location is nil")
				}
				if got.Location.Name != "Massey Hall" {
					t.Errorf("Location.Name = %q", got.Location.Name)
				}
				if got.Location.StreetAddress != "178 Victoria St" {
					t.Errorf("Location.StreetAddress = %q", got.Location.StreetAddress)
				}
				if got.Location.AddressLocality != "Toronto" {
					t.Errorf("Location.AddressLocality = %q", got.Location.AddressLocality)
				}
				if got.Location.AddressRegion != "ON" {
					t.Errorf("Location.AddressRegion = %q", got.Location.AddressRegion)
				}
				if got.Location.PostalCode != "M5B 1T7" {
					t.Errorf("Location.PostalCode = %q", got.Location.PostalCode)
				}
				if got.Location.AddressCountry != "CA" {
					t.Errorf("Location.AddressCountry = %q", got.Location.AddressCountry)
				}
				if got.Location.Latitude != 43.6535 {
					t.Errorf("Location.Latitude = %v", got.Location.Latitude)
				}
				if got.Location.Longitude != -79.3796 {
					t.Errorf("Location.Longitude = %v", got.Location.Longitude)
				}
				if got.Organizer == nil {
					t.Fatal("Organizer is nil")
				}
				if got.Organizer.Name != "Live Nation" {
					t.Errorf("Organizer.Name = %q", got.Organizer.Name)
				}
				if got.Organizer.URL != "https://livenation.com" {
					t.Errorf("Organizer.URL = %q", got.Organizer.URL)
				}
				if got.Offers == nil {
					t.Fatal("Offers is nil")
				}
				if got.Offers.Price != "25" {
					t.Errorf("Offers.Price = %q", got.Offers.Price)
				}
				if got.Offers.PriceCurrency != "CAD" {
					t.Errorf("Offers.PriceCurrency = %q", got.Offers.PriceCurrency)
				}
				if got.Offers.URL != "https://example.com/tickets/1" {
					t.Errorf("Offers.URL = %q", got.Offers.URL)
				}
				if len(got.Keywords) != 2 || got.Keywords[0] != "jazz" {
					t.Errorf("Keywords = %v", got.Keywords)
				}
				if len(got.InLanguage) != 1 || got.InLanguage[0] != "en" {
					t.Errorf("InLanguage = %v", got.InLanguage)
				}
				if got.IsAccessibleForFree == nil || *got.IsAccessibleForFree != false {
					t.Errorf("IsAccessibleForFree = %v", got.IsAccessibleForFree)
				}
				if len(got.SameAs) != 1 || got.SameAs[0] != "https://eventbrite.com/e/123" {
					t.Errorf("SameAs = %v", got.SameAs)
				}
				if got.License != "CC0-1.0" {
					t.Errorf("License = %q", got.License)
				}
				if got.Source == nil {
					t.Fatal("Source is nil")
				}
				if got.Source.URL != testSource.URL {
					t.Errorf("Source.URL = %q", got.Source.URL)
				}
				if got.Source.Name != testSource.Name {
					t.Errorf("Source.Name = %q", got.Source.Name)
				}
			},
		},
		{
			name: "string location",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"location": "Massey Hall"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Location == nil {
					t.Fatal("Location is nil")
				}
				if got.Location.Name != "Massey Hall" {
					t.Errorf("Location.Name = %q, want %q", got.Location.Name, "Massey Hall")
				}
			},
		},
		{
			name: "offers as array - first element used",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"offers": [
					{"@type": "Offer", "price": "10", "priceCurrency": "CAD"},
					{"@type": "Offer", "price": "20", "priceCurrency": "CAD"}
				]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Offers == nil {
					t.Fatal("Offers is nil")
				}
				if got.Offers.Price != "10" {
					t.Errorf("Offers.Price = %q, want %q", got.Offers.Price, "10")
				}
			},
		},
		{
			name: "offers as single object",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"offers": {"@type": "Offer", "price": "15", "priceCurrency": "USD"}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Offers == nil {
					t.Fatal("Offers is nil")
				}
				if got.Offers.Price != "15" {
					t.Errorf("Offers.Price = %q", got.Offers.Price)
				}
				if got.Offers.PriceCurrency != "USD" {
					t.Errorf("Offers.PriceCurrency = %q", got.Offers.PriceCurrency)
				}
			},
		},
		{
			name: "keywords as string",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"keywords": "jazz"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Keywords) != 1 || got.Keywords[0] != "jazz" {
					t.Errorf("Keywords = %v, want [\"jazz\"]", got.Keywords)
				}
			},
		},
		{
			name: "keywords as array",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"keywords": ["jazz", "blues", "soul"]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Keywords) != 3 {
					t.Errorf("Keywords length = %d, want 3", len(got.Keywords))
				}
				if got.Keywords[1] != "blues" {
					t.Errorf("Keywords[1] = %q, want blues", got.Keywords[1])
				}
			},
		},
		{
			name: "isAccessibleForFree as bool true",
			raw: `{
				"@type": "Event",
				"name": "Free Concert",
				"startDate": "2026-04-01",
				"isAccessibleForFree": true
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.IsAccessibleForFree == nil {
					t.Fatal("IsAccessibleForFree is nil")
				}
				if *got.IsAccessibleForFree != true {
					t.Errorf("IsAccessibleForFree = %v, want true", *got.IsAccessibleForFree)
				}
			},
		},
		{
			name: "isAccessibleForFree as string True",
			raw: `{
				"@type": "Event",
				"name": "Free Concert",
				"startDate": "2026-04-01",
				"isAccessibleForFree": "True"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.IsAccessibleForFree == nil {
					t.Fatal("IsAccessibleForFree is nil")
				}
				if *got.IsAccessibleForFree != true {
					t.Errorf("IsAccessibleForFree = %v, want true", *got.IsAccessibleForFree)
				}
			},
		},
		{
			name: "missing name returns error",
			raw: `{
				"@type": "Event",
				"startDate": "2026-04-01"
			}`,
			source:     testSource,
			wantErr:    true,
			wantErrMsg: "event has no name",
		},
		{
			name: "missing startDate returns error",
			raw: `{
				"@type": "Event",
				"name": "Concert"
			}`,
			source:     testSource,
			wantErr:    true,
			wantErrMsg: "event has no startDate",
		},
		{
			name: "date as plain string",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-03-15"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.StartDate != "2026-03-15" {
					t.Errorf("StartDate = %q, want %q", got.StartDate, "2026-03-15")
				}
			},
		},
		{
			name: "date as typed @value object",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": {"@type": "Date", "@value": "2026-03-15"}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.StartDate != "2026-03-15" {
					t.Errorf("StartDate = %q, want %q", got.StartDate, "2026-03-15")
				}
			},
		},
		{
			name: "image as string",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"image": "https://example.com/img.jpg"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Image != "https://example.com/img.jpg" {
					t.Errorf("Image = %q", got.Image)
				}
			},
		},
		{
			name: "image as ImageObject with url",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"image": {"@type": "ImageObject", "url": "https://example.com/img.jpg"}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Image != "https://example.com/img.jpg" {
					t.Errorf("Image = %q, want https://example.com/img.jpg", got.Image)
				}
			},
		},
		{
			name: "organizer as object",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"organizer": {
					"@type": "Organization",
					"name": "Jazz Society",
					"url": "https://jazzso.org",
					"email": "info@jazzso.org",
					"telephone": "+1-416-555-0100"
				}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Organizer == nil {
					t.Fatal("Organizer is nil")
				}
				if got.Organizer.Name != "Jazz Society" {
					t.Errorf("Organizer.Name = %q", got.Organizer.Name)
				}
				if got.Organizer.URL != "https://jazzso.org" {
					t.Errorf("Organizer.URL = %q", got.Organizer.URL)
				}
				if got.Organizer.Email != "info@jazzso.org" {
					t.Errorf("Organizer.Email = %q", got.Organizer.Email)
				}
				if got.Organizer.Telephone != "+1-416-555-0100" {
					t.Errorf("Organizer.Telephone = %q", got.Organizer.Telephone)
				}
			},
		},
		{
			name: "organizer as array - first element used",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"organizer": [
					{"@type": "Organization", "name": "Primary Org"},
					{"@type": "Organization", "name": "Secondary Org"}
				]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Organizer == nil {
					t.Fatal("Organizer is nil")
				}
				if got.Organizer.Name != "Primary Org" {
					t.Errorf("Organizer.Name = %q, want Primary Org", got.Organizer.Name)
				}
			},
		},
		{
			name: "source attribution populated from SourceConfig",
			raw: `{
				"@type": "Event",
				"@id": "https://example.com/events/42",
				"name": "Concert",
				"startDate": "2026-04-01"
			}`,
			source: SourceConfig{
				Name:    "My Source",
				URL:     "https://my-source.com",
				License: "CC-BY-4.0",
			},
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Source == nil {
					t.Fatal("Source is nil")
				}
				if got.Source.URL != "https://my-source.com" {
					t.Errorf("Source.URL = %q", got.Source.URL)
				}
				if got.Source.Name != "My Source" {
					t.Errorf("Source.Name = %q", got.Source.Name)
				}
				if got.Source.License != "CC-BY-4.0" {
					t.Errorf("Source.License = %q", got.Source.License)
				}
				if got.Source.EventID != "https://example.com/events/42" {
					t.Errorf("Source.EventID = %q, want https://example.com/events/42", got.Source.EventID)
				}
				if got.License != "CC-BY-4.0" {
					t.Errorf("License = %q", got.License)
				}
			},
		},
		{
			name: "isAccessibleForFree as bool false",
			raw: `{
				"@type": "Event",
				"name": "Paid Concert",
				"startDate": "2026-04-01",
				"isAccessibleForFree": false
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.IsAccessibleForFree == nil {
					t.Fatal("IsAccessibleForFree is nil")
				}
				if *got.IsAccessibleForFree != false {
					t.Errorf("IsAccessibleForFree = %v, want false", *got.IsAccessibleForFree)
				}
			},
		},
		{
			name: "isAccessibleForFree as string false",
			raw: `{
				"@type": "Event",
				"name": "Paid Concert",
				"startDate": "2026-04-01",
				"isAccessibleForFree": "False"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.IsAccessibleForFree == nil {
					t.Fatal("IsAccessibleForFree is nil")
				}
				if *got.IsAccessibleForFree != false {
					t.Errorf("IsAccessibleForFree = %v, want false", *got.IsAccessibleForFree)
				}
			},
		},
		{
			name: "place with flat address fields",
			raw: `{
				"@type": "Event",
				"name": "Festival",
				"startDate": "2026-07-04",
				"location": {
					"@type": "Place",
					"name": "City Park",
					"streetAddress": "100 Main St",
					"addressLocality": "Ottawa",
					"addressRegion": "ON",
					"addressCountry": "CA"
				}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Location == nil {
					t.Fatal("Location is nil")
				}
				if got.Location.StreetAddress != "100 Main St" {
					t.Errorf("Location.StreetAddress = %q", got.Location.StreetAddress)
				}
				if got.Location.AddressLocality != "Ottawa" {
					t.Errorf("Location.AddressLocality = %q", got.Location.AddressLocality)
				}
			},
		},
		{
			name: "event ID falls back to url when @id absent",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"url": "https://example.com/events/fallback"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Source == nil {
					t.Fatal("Source is nil")
				}
				if got.Source.EventID != "https://example.com/events/fallback" {
					t.Errorf("Source.EventID = %q", got.Source.EventID)
				}
			},
		},
		{
			name: "event ID falls back to source.URL when no @id or url field",
			raw: `{
				"@type": "Event",
				"name": "Second City Show",
				"startDate": "2026-05-01T19:30:00"
			}`,
			source: SourceConfig{
				Name:    "second-city-toronto",
				URL:     "https://secondcity.com/shows/toronto/the-show",
				License: "CC0-1.0",
			},
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Source == nil {
					t.Fatal("Source is nil")
				}
				if got.Source.EventID != "https://secondcity.com/shows/toronto/the-show" {
					t.Errorf("Source.EventID = %q, want source.URL fallback", got.Source.EventID)
				}
			},
		},
		{
			name: "image as ImageObject with contentUrl",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"image": {"@type": "ImageObject", "contentUrl": "https://example.com/content.jpg"}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Image != "https://example.com/content.jpg" {
					t.Errorf("Image = %q", got.Image)
				}
			},
		},
		{
			name: "sameAs as array",
			raw: `{
				"@type": "Event",
				"name": "Concert",
				"startDate": "2026-04-01",
				"sameAs": ["https://eventbrite.com/e/1", "https://facebook.com/events/2"]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.SameAs) != 2 {
					t.Errorf("SameAs length = %d, want 2", len(got.SameAs))
				}
			},
		},
		{
			name:    "invalid JSON returns error",
			raw:     `{not valid json`,
			source:  testSource,
			wantErr: true,
		},
		{
			name: "EventSeries with subEvent array",
			raw: `{
				"@type": "EventSeries",
				"name": "Jazz Festival 2026",
				"startDate": "2026-07-10T20:00:00",
				"subEvent": [
					{"@type": "Event", "name": "Night 1", "startDate": "2026-07-10T20:00:00", "endDate": "2026-07-10T23:00:00"},
					{"@type": "Event", "name": "Night 2", "startDate": "2026-07-11T19:00:00", "endDate": "2026-07-11T22:00:00"},
					{"@type": "Event", "name": "Night 3", "startDate": "2026-07-12T20:00:00", "endDate": "2026-07-12T23:30:00"}
				]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Occurrences) != 3 {
					t.Fatalf("Occurrences len = %d, want 3", len(got.Occurrences))
				}
				if got.Occurrences[0].StartDate != "2026-07-10T20:00:00" {
					t.Errorf("Occurrences[0].StartDate = %q", got.Occurrences[0].StartDate)
				}
				if got.Occurrences[1].StartDate != "2026-07-11T19:00:00" {
					t.Errorf("Occurrences[1].StartDate = %q", got.Occurrences[1].StartDate)
				}
				if got.Occurrences[2].EndDate != "2026-07-12T23:30:00" {
					t.Errorf("Occurrences[2].EndDate = %q", got.Occurrences[2].EndDate)
				}
				if got.Type != "EventSeries" {
					t.Errorf("Type = %q, want EventSeries", got.Type)
				}
			},
		},
		{
			name: "Event with single subEvent object",
			raw: `{
				"@type": "Event",
				"name": "Workshop Series",
				"startDate": "2026-08-01T10:00:00",
				"subEvent": {
					"@type": "Event",
					"name": "Session 1",
					"startDate": "2026-08-01T10:00:00",
					"endDate": "2026-08-01T12:00:00"
				}
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Occurrences) != 1 {
					t.Fatalf("Occurrences len = %d, want 1", len(got.Occurrences))
				}
				if got.Occurrences[0].StartDate != "2026-08-01T10:00:00" {
					t.Errorf("Occurrences[0].StartDate = %q", got.Occurrences[0].StartDate)
				}
				if got.Occurrences[0].EndDate != "2026-08-01T12:00:00" {
					t.Errorf("Occurrences[0].EndDate = %q", got.Occurrences[0].EndDate)
				}
			},
		},
		{
			name: "subEvent with URL-only references skipped",
			raw: `{
				"@type": "EventSeries",
				"name": "Mixed Series",
				"startDate": "2026-09-01T10:00:00",
				"subEvent": [
					"https://example.com/events/1",
					{"@type": "Event", "name": "Inline Event", "startDate": "2026-09-01T10:00:00"},
					"https://example.com/events/2"
				]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Occurrences) != 1 {
					t.Fatalf("Occurrences len = %d, want 1 (URL references skipped)", len(got.Occurrences))
				}
				if got.Occurrences[0].StartDate != "2026-09-01T10:00:00" {
					t.Errorf("Occurrences[0].StartDate = %q", got.Occurrences[0].StartDate)
				}
			},
		},
		{
			name: "subEvent without startDate skipped",
			raw: `{
				"@type": "EventSeries",
				"name": "Partial Series",
				"startDate": "2026-10-01T10:00:00",
				"subEvent": [
					{"@type": "Event", "name": "Has Date", "startDate": "2026-10-01T10:00:00"},
					{"@type": "Event", "name": "No Date"},
					{"@type": "Event", "name": "Also Has Date", "startDate": "2026-10-02T10:00:00"}
				]
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Occurrences) != 2 {
					t.Fatalf("Occurrences len = %d, want 2 (no-date sub-event skipped)", len(got.Occurrences))
				}
				if got.Occurrences[0].StartDate != "2026-10-01T10:00:00" {
					t.Errorf("Occurrences[0].StartDate = %q", got.Occurrences[0].StartDate)
				}
				if got.Occurrences[1].StartDate != "2026-10-02T10:00:00" {
					t.Errorf("Occurrences[1].StartDate = %q", got.Occurrences[1].StartDate)
				}
			},
		},
		{
			name: "event without subEvent unchanged",
			raw: `{
				"@type": "Event",
				"name": "Simple Concert",
				"startDate": "2026-11-01T20:00:00",
				"endDate": "2026-11-01T22:00:00"
			}`,
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if len(got.Occurrences) != 0 {
					t.Errorf("Occurrences len = %d, want 0 for event without subEvent", len(got.Occurrences))
				}
				if got.StartDate != "2026-11-01T20:00:00" {
					t.Errorf("StartDate = %q", got.StartDate)
				}
			},
		},
	}

	// Suppress unused variable warnings for boolTrue/boolFalse — they are
	// used indirectly through test closures above.
	_ = boolTrue
	_ = boolFalse

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeJSONLDEvent(json.RawMessage(tc.raw), tc.source)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result: %+v)", got)
				}
				if tc.wantErrMsg != "" && err.Error() != tc.wantErrMsg {
					t.Errorf("error = %q, want %q", err.Error(), tc.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestParseSubEvents covers the parseSubEvents helper function.
func TestParseSubEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantNil   bool
	}{
		{
			name:    "nil input",
			raw:     ``,
			wantNil: true,
		},
		{
			name:    "null string",
			raw:     `null`,
			wantNil: true,
		},
		{
			name:      "single object",
			raw:       `{"@type":"Event","startDate":"2026-06-01"}`,
			wantCount: 1,
		},
		{
			name:      "array of three objects",
			raw:       `[{"@type":"Event","startDate":"2026-06-01"},{"@type":"Event","startDate":"2026-06-02"},{"@type":"Event","startDate":"2026-06-03"}]`,
			wantCount: 3,
		},
		{
			name:      "array with mixed objects and URL strings",
			raw:       `["https://example.com/1",{"@type":"Event","startDate":"2026-06-01"},"https://example.com/2",{"@type":"Event","startDate":"2026-06-02"}]`,
			wantCount: 2,
		},
		{
			name:    "single string URL reference",
			raw:     `"https://example.com/events/1"`,
			wantNil: true,
		},
		{
			name:    "malformed JSON",
			raw:     `[{not valid`,
			wantNil: true,
		},
		{
			name:    "empty array",
			raw:     `[]`,
			wantNil: true,
		},
		{
			name:      "array with all URL strings filtered out yields nil result",
			raw:       `["https://example.com/1","https://example.com/2"]`,
			wantCount: 0,
			wantNil:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseSubEvents(json.RawMessage(tc.raw))
			if tc.wantNil {
				if got != nil {
					t.Errorf("parseSubEvents(%s) = %v (len %d), want nil", tc.raw, got, len(got))
				}
				return
			}
			if len(got) != tc.wantCount {
				t.Errorf("parseSubEvents(%s) len = %d, want %d", tc.raw, len(got), tc.wantCount)
			}
		})
	}
}

// TestNormalizeRawEvent covers the NormalizeRawEvent function.
func TestNormalizeRawEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        RawEvent
		source     SourceConfig
		wantErr    bool
		wantErrMsg string
		check      func(t *testing.T, got events.EventInput)
	}{
		{
			name: "valid event",
			raw: RawEvent{
				Name:        "Film Screening",
				StartDate:   "2026-05-10T19:00:00",
				EndDate:     "2026-05-10T21:00:00",
				Location:    "TIFF Bell Lightbox",
				Description: "An award-winning documentary",
				URL:         "https://example.com/film",
				Image:       "https://example.com/poster.jpg",
			},
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Name != "Film Screening" {
					t.Errorf("Name = %q", got.Name)
				}
				if got.Type != "Event" {
					t.Errorf("Type = %q, want Event", got.Type)
				}
				// Partial ISO 8601 inputs now get timezone applied; check prefix only.
				if !hasPrefix(got.StartDate, "2026-05-10T19:00:00-") {
					t.Errorf("StartDate = %q, want prefix 2026-05-10T19:00:00-", got.StartDate)
				}
				if !hasPrefix(got.EndDate, "2026-05-10T21:00:00-") {
					t.Errorf("EndDate = %q, want prefix 2026-05-10T21:00:00-", got.EndDate)
				}
				if got.Description != "An award-winning documentary" {
					t.Errorf("Description = %q", got.Description)
				}
				if got.Image != "https://example.com/poster.jpg" {
					t.Errorf("Image = %q", got.Image)
				}
				if got.Location == nil {
					t.Fatal("Location is nil")
				}
				if got.Location.Name != "TIFF Bell Lightbox" {
					t.Errorf("Location.Name = %q", got.Location.Name)
				}
				if got.License != testSource.License {
					t.Errorf("License = %q", got.License)
				}
				if got.Source == nil {
					t.Fatal("Source is nil")
				}
				if got.Source.EventID != "https://example.com/film" {
					t.Errorf("Source.EventID = %q", got.Source.EventID)
				}
				if got.Source.URL != testSource.URL {
					t.Errorf("Source.URL = %q", got.Source.URL)
				}
				if got.Source.Name != testSource.Name {
					t.Errorf("Source.Name = %q", got.Source.Name)
				}
			},
		},
		{
			name: "missing name returns error",
			raw: RawEvent{
				StartDate: "2026-05-10",
			},
			source:     testSource,
			wantErr:    true,
			wantErrMsg: "raw event has no name",
		},
		{
			name: "missing startDate returns error",
			raw: RawEvent{
				Name: "Film Screening",
			},
			source:     testSource,
			wantErr:    true,
			wantErrMsg: "raw event has no startDate",
		},
		{
			name: "location string creates PlaceInput",
			raw: RawEvent{
				Name:      "Concert",
				StartDate: "2026-06-01",
				Location:  "Roy Thomson Hall",
			},
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Location == nil {
					t.Fatal("Location is nil")
				}
				if got.Location.Name != "Roy Thomson Hall" {
					t.Errorf("Location.Name = %q, want Roy Thomson Hall", got.Location.Name)
				}
			},
		},
		{
			name: "empty location results in nil PlaceInput",
			raw: RawEvent{
				Name:      "Concert",
				StartDate: "2026-06-01",
			},
			source: testSource,
			check: func(t *testing.T, got events.EventInput) {
				t.Helper()
				if got.Location != nil {
					t.Errorf("Location = %+v, want nil", got.Location)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeRawEvent(tc.raw, tc.source)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result: %+v)", got)
				}
				if tc.wantErrMsg != "" && err.Error() != tc.wantErrMsg {
					t.Errorf("error = %q, want %q", err.Error(), tc.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// TestParseDate tests the date parsing helper directly.
func TestParseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", ``, ""},
		{"null", `null`, ""},
		{"plain string", `"2026-03-15"`, "2026-03-15"},
		{"datetime string", `"2026-03-15T20:00:00"`, "2026-03-15T20:00:00"},
		{"@value object", `{"@value":"2026-03-15"}`, "2026-03-15"},
		{"typed Date object", `{"@type":"Date","@value":"2026-03-15"}`, "2026-03-15"},
		{"typed DateTime object", `{"@type":"DateTime","@value":"2026-03-15T20:00:00Z"}`, "2026-03-15T20:00:00Z"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseDate(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("parseDate(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestParseBool tests boolean parsing including string variants.
func TestParseBool(t *testing.T) {
	t.Parallel()

	bTrue := true
	bFalse := false

	tests := []struct {
		name string
		raw  string
		want *bool
	}{
		{"null", `null`, nil},
		{"empty", ``, nil},
		{"bool true", `true`, &bTrue},
		{"bool false", `false`, &bFalse},
		{"string True", `"True"`, &bTrue},
		{"string False", `"False"`, &bFalse},
		{"string true lowercase", `"true"`, &bTrue},
		{"string false lowercase", `"false"`, &bFalse},
		{"unknown string", `"maybe"`, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseBool(json.RawMessage(tc.raw))
			if tc.want == nil {
				if got != nil {
					t.Errorf("parseBool(%s) = %v, want nil", tc.raw, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseBool(%s) = nil, want %v", tc.raw, *tc.want)
			}
			if *got != *tc.want {
				t.Errorf("parseBool(%s) = %v, want %v", tc.raw, *got, *tc.want)
			}
		})
	}
}

// TestParseStringOrArray tests the string-or-array helper.
func TestParseStringOrArray(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"null", `null`, nil},
		{"empty", ``, nil},
		{"empty string", `""`, nil},
		{"single string", `"jazz"`, []string{"jazz"}},
		{"array", `["a","b","c"]`, []string{"a", "b", "c"}},
		{"empty array", `[]`, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseStringOrArray(json.RawMessage(tc.raw))
			if len(got) != len(tc.want) {
				t.Fatalf("parseStringOrArray(%s) = %v (len %d), want %v (len %d)", tc.raw, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestParseImage tests image URL extraction.
func TestParseImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"null", `null`, ""},
		{"plain string", `"https://example.com/img.jpg"`, "https://example.com/img.jpg"},
		{"ImageObject url", `{"@type":"ImageObject","url":"https://example.com/img.jpg"}`, "https://example.com/img.jpg"},
		{"ImageObject contentUrl", `{"@type":"ImageObject","contentUrl":"https://example.com/content.jpg"}`, "https://example.com/content.jpg"},
		{"array first element", `[{"url":"https://example.com/first.jpg"},{"url":"https://example.com/second.jpg"}]`, "https://example.com/first.jpg"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseImage(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("parseImage(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestExtractEventID tests event ID extraction priority.
func TestExtractEventID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"@id wins", `{"@id":"https://ex.com/1","url":"https://ex.com/2"}`, "https://ex.com/1"},
		{"url fallback", `{"url":"https://ex.com/url"}`, "https://ex.com/url"},
		{"identifier string", `{"identifier":"ext-id-123"}`, "ext-id-123"},
		{"identifier @value object", `{"identifier":{"@value":"ext-id-456"}}`, "ext-id-456"},
		{"nothing", `{}`, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractEventID(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("extractEventID = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEventIDForJSONLD tests the eventIDForJSONLD fallback chain.
func TestEventIDForJSONLD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		urlStr  string
		pageURL string
		want    string
	}{
		{
			name:    "@id wins over urlStr and pageURL",
			raw:     `{"@id":"https://ex.com/1","url":"https://ex.com/url"}`,
			urlStr:  "https://ex.com/url",
			pageURL: "https://ex.com/page",
			want:    "https://ex.com/1",
		},
		{
			name:    "falls back to urlStr when no @id",
			raw:     `{"name":"Concert"}`,
			urlStr:  "https://ex.com/url",
			pageURL: "https://ex.com/page",
			want:    "https://ex.com/url",
		},
		{
			name:    "falls back to pageURL when no @id or urlStr",
			raw:     `{"name":"Concert"}`,
			urlStr:  "",
			pageURL: "https://ex.com/page",
			want:    "https://ex.com/page",
		},
		{
			name:    "all empty returns empty",
			raw:     `{}`,
			urlStr:  "",
			pageURL: "",
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := eventIDForJSONLD(json.RawMessage(tc.raw), tc.urlStr, tc.pageURL)
			if got != tc.want {
				t.Errorf("eventIDForJSONLD = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestHasTruncatedDescription verifies detection of ellipsis-truncated descriptions.
func TestHasTruncatedDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		desc string
		want bool
	}{
		{"ellipsis unicode suffix", "This is a long description\u2026", true},
		{"ellipsis literal suffix", "This is a long description…", true},
		{"three dots suffix", "This is a long description...", true},
		{"three dots with space", "This is a long description ... ", true},
		{"empty string", "", false},
		{"full sentence no ellipsis", "This is a complete description.", false},
		{"ends with period not dots", "A description ending in period.", false},
		{"only ellipsis", "…", true},
		{"only three dots", "...", true},
		{"ellipsis mid-string", "Truncated… here is more text", false},
		{"whitespace trimmed ellipsis", "Some text…   ", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := HasTruncatedDescription(tc.desc)
			if got != tc.want {
				t.Errorf("HasTruncatedDescription(%q) = %v, want %v", tc.desc, got, tc.want)
			}
		})
	}
}

// TestMustJSONHelper ensures the test helper itself works correctly.
func TestMustJSONHelper(t *testing.T) {
	raw := mustJSON("hello")
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || s != "hello" {
		t.Errorf("mustJSON round-trip failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// normalizeRawEvents helper tests
// ---------------------------------------------------------------------------

// makeRaw is a shorthand for building a RawEvent in tests.
func makeRaw(name, url, startDate string) RawEvent {
	return RawEvent{
		Name:      name,
		URL:       url,
		StartDate: startDate,
	}
}

// TestNormalizeRawEvents_MultiRowGrouping verifies that RawEvents sharing the
// same URL+Name are consolidated into a single EventInput with Occurrences,
// while a RawEvent with a distinct URL+Name becomes its own EventInput.
func TestNormalizeRawEvents_MultiRowGrouping(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	raws := []RawEvent{
		makeRaw("Hamlet", "https://example.com/hamlet", "2026-06-01T20:00:00"),
		makeRaw("Hamlet", "https://example.com/hamlet", "2026-06-02T20:00:00"),
		makeRaw("Hamlet", "https://example.com/hamlet", "2026-06-03T20:00:00"),
		makeRaw("Oak Tree", "https://example.com/oak-tree", "2026-06-05T19:30:00"),
	}

	valid, skipped := normalizeRawEvents(raws, src, 0, logger)

	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if len(valid) != 2 {
		t.Fatalf("expected 2 EventInputs, got %d", len(valid))
	}

	// Find the Hamlet event (3 occurrences).
	var hamlet, oakTree *events.EventInput
	for i := range valid {
		switch valid[i].Name {
		case "Hamlet":
			hamlet = &valid[i]
		case "Oak Tree":
			oakTree = &valid[i]
		}
	}

	if hamlet == nil {
		t.Fatal("Hamlet EventInput not found")
	}
	if oakTree == nil {
		t.Fatal("Oak Tree EventInput not found")
	}
	if len(hamlet.Occurrences) != 3 {
		t.Errorf("expected Hamlet to have 3 occurrences, got %d", len(hamlet.Occurrences))
	}
	if len(oakTree.Occurrences) != 0 {
		t.Errorf("expected Oak Tree to have 0 occurrences (single-row), got %d", len(oakTree.Occurrences))
	}
}

// TestNormalizeRawEvents_SingleEvents verifies that RawEvents all with distinct
// URL+Name keys each produce a separate EventInput with no Occurrences set.
func TestNormalizeRawEvents_SingleEvents(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	raws := []RawEvent{
		makeRaw("Event A", "https://example.com/a", "2026-06-01T20:00:00"),
		makeRaw("Event B", "https://example.com/b", "2026-06-02T20:00:00"),
		makeRaw("Event C", "https://example.com/c", "2026-06-03T20:00:00"),
	}

	valid, skipped := normalizeRawEvents(raws, src, 0, logger)

	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if len(valid) != 3 {
		t.Fatalf("expected 3 EventInputs, got %d", len(valid))
	}
	for _, ev := range valid {
		if len(ev.Occurrences) != 0 {
			t.Errorf("event %q: expected no occurrences for single-row, got %d", ev.Name, len(ev.Occurrences))
		}
	}
}

// TestNormalizeRawEvents_Limit verifies that the limit parameter caps the
// number of returned EventInputs.
func TestNormalizeRawEvents_Limit(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	raws := []RawEvent{
		makeRaw("Event A", "https://example.com/a", "2026-06-01T20:00:00"),
		makeRaw("Event B", "https://example.com/b", "2026-06-02T20:00:00"),
		makeRaw("Event C", "https://example.com/c", "2026-06-03T20:00:00"),
		makeRaw("Event D", "https://example.com/d", "2026-06-04T20:00:00"),
		makeRaw("Event E", "https://example.com/e", "2026-06-05T20:00:00"),
	}

	valid, _ := normalizeRawEvents(raws, src, 2, logger)

	if len(valid) != 2 {
		t.Errorf("expected 2 EventInputs with limit=2, got %d", len(valid))
	}
}

// TestNormalizeRawEvents_SkipsInvalidDates verifies that within a multi-row
// group, rows with empty DateParts (no parseable date) are skipped from the
// occurrence list rather than failing the whole group.
func TestNormalizeRawEvents_SkipsInvalidDates(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	// Three rows for the same event: two have valid dates, one has empty DateParts.
	raws := []RawEvent{
		makeRaw("Show", "https://example.com/show", "2026-06-01T20:00:00"),
		// Empty StartDate and no DateParts → normalizeStartDate returns "".
		{Name: "Show", URL: "https://example.com/show"},
		makeRaw("Show", "https://example.com/show", "2026-06-03T20:00:00"),
	}

	valid, skipped := normalizeRawEvents(raws, src, 0, logger)

	// The group as a whole should succeed (2 valid occurrences).
	if skipped != 0 {
		t.Errorf("expected 0 skipped groups, got %d", skipped)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 EventInput, got %d", len(valid))
	}
	if len(valid[0].Occurrences) != 2 {
		t.Errorf("expected 2 occurrences (bad row skipped), got %d", len(valid[0].Occurrences))
	}
}

// TestNormalizeRawEvents_EmptyURLNoCollision verifies that RawEvents with empty
// URLs and the same Name are NOT merged. Without the disambiguation guard each
// would collide on the key "|||SameName" and be incorrectly treated as
// multi-occurrence rows of one event.
func TestNormalizeRawEvents_EmptyURLNoCollision(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	raws := []RawEvent{
		makeRaw("Same Name", "", "2026-06-01T20:00:00"),
		makeRaw("Same Name", "", "2026-06-02T20:00:00"),
		makeRaw("Same Name", "", "2026-06-03T20:00:00"),
	}

	valid, skipped := normalizeRawEvents(raws, src, 0, logger)

	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	// Each row should produce its own EventInput — they must NOT be merged.
	if len(valid) != 3 {
		t.Errorf("expected 3 separate EventInputs for empty-URL rows with same name, got %d", len(valid))
	}
}

// makeJSONLDRaw builds a minimal JSON-LD Event json.RawMessage for testing.
func makeJSONLDRaw(name, url, startDate string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"@type":"Event","name":%q,"url":%q,"startDate":%q}`, name, url, startDate))
}

// TestGroupJSONLDEvents covers the groupJSONLDEvents function.
func TestGroupJSONLDEvents(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	tests := []struct {
		name        string
		raws        []json.RawMessage
		limit       int
		wantCount   int
		wantSkipped int
		check       func(t *testing.T, valid []events.EventInput)
	}{
		{
			name: "three events same URL+Name → one EventInput with 3 Occurrences",
			raws: []json.RawMessage{
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-01T20:00:00"),
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-02T20:00:00"),
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-03T20:00:00"),
			},
			wantCount:   1,
			wantSkipped: 0,
			check: func(t *testing.T, valid []events.EventInput) {
				t.Helper()
				if valid[0].Name != "Hamlet" {
					t.Errorf("expected Name=Hamlet, got %q", valid[0].Name)
				}
				if len(valid[0].Occurrences) != 3 {
					t.Errorf("expected 3 Occurrences, got %d", len(valid[0].Occurrences))
				}
			},
		},
		{
			name: "different shows not grouped",
			raws: []json.RawMessage{
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-01T20:00:00"),
				makeJSONLDRaw("Macbeth", "https://example.com/macbeth", "2026-06-02T20:00:00"),
			},
			wantCount:   2,
			wantSkipped: 0,
			check: func(t *testing.T, valid []events.EventInput) {
				t.Helper()
				if len(valid[0].Occurrences) != 0 {
					t.Errorf("expected 0 Occurrences for first event, got %d", len(valid[0].Occurrences))
				}
				if len(valid[1].Occurrences) != 0 {
					t.Errorf("expected 0 Occurrences for second event, got %d", len(valid[1].Occurrences))
				}
			},
		},
		{
			name: "mixed — some grouped, some standalone",
			raws: []json.RawMessage{
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-01T20:00:00"),
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-02T20:00:00"),
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-03T20:00:00"),
				makeJSONLDRaw("The Tempest", "https://example.com/tempest", "2026-06-05T19:30:00"),
			},
			wantCount:   2,
			wantSkipped: 0,
			check: func(t *testing.T, valid []events.EventInput) {
				t.Helper()
				// First output = Hamlet (3 occurrences).
				if valid[0].Name != "Hamlet" {
					t.Errorf("expected first event Name=Hamlet, got %q", valid[0].Name)
				}
				if len(valid[0].Occurrences) != 3 {
					t.Errorf("expected 3 Occurrences for Hamlet, got %d", len(valid[0].Occurrences))
				}
				// Second output = The Tempest (no occurrences).
				if valid[1].Name != "The Tempest" {
					t.Errorf("expected second event Name=The Tempest, got %q", valid[1].Name)
				}
				if len(valid[1].Occurrences) != 0 {
					t.Errorf("expected 0 Occurrences for The Tempest, got %d", len(valid[1].Occurrences))
				}
			},
		},
		{
			name: "events without URL not grouped",
			raws: []json.RawMessage{
				makeJSONLDRaw("Same Name", "", "2026-06-01T20:00:00"),
				makeJSONLDRaw("Same Name", "", "2026-06-02T20:00:00"),
			},
			wantCount:   2,
			wantSkipped: 0,
			check: func(t *testing.T, valid []events.EventInput) {
				t.Helper()
				if len(valid[0].Occurrences) != 0 {
					t.Errorf("expected 0 Occurrences for first no-URL event, got %d", len(valid[0].Occurrences))
				}
				if len(valid[1].Occurrences) != 0 {
					t.Errorf("expected 0 Occurrences for second no-URL event, got %d", len(valid[1].Occurrences))
				}
			},
		},
		{
			name: "limit caps output EventInputs",
			raws: []json.RawMessage{
				makeJSONLDRaw("Event A", "https://example.com/a", "2026-06-01T20:00:00"),
				makeJSONLDRaw("Event B", "https://example.com/b", "2026-06-02T20:00:00"),
				makeJSONLDRaw("Event C", "https://example.com/c", "2026-06-03T20:00:00"),
				makeJSONLDRaw("Event D", "https://example.com/d", "2026-06-04T20:00:00"),
				makeJSONLDRaw("Event E", "https://example.com/e", "2026-06-05T20:00:00"),
			},
			limit:       2,
			wantCount:   2,
			wantSkipped: 0,
		},
		{
			name: "invalid event in group skipped",
			raws: []json.RawMessage{
				// Missing name → NormalizeJSONLDEvent will fail.
				json.RawMessage(`{"@type":"Event","url":"https://example.com/x","startDate":"2026-06-01T20:00:00"}`),
			},
			wantCount:   0,
			wantSkipped: 1,
		},
		{
			name: "single event passthrough",
			raws: []json.RawMessage{
				makeJSONLDRaw("Solo Show", "https://example.com/solo", "2026-06-01T20:00:00"),
			},
			wantCount:   1,
			wantSkipped: 0,
			check: func(t *testing.T, valid []events.EventInput) {
				t.Helper()
				if len(valid[0].Occurrences) != 0 {
					t.Errorf("expected 0 Occurrences for single event, got %d", len(valid[0].Occurrences))
				}
			},
		},
		{
			name: "multi-event group preserves first event's metadata",
			raws: []json.RawMessage{
				json.RawMessage(`{"@type":"Event","name":"Hamlet","url":"https://example.com/hamlet","startDate":"2026-06-01T20:00:00","description":"First night","location":{"@type":"Place","name":"Royal Theatre"},"image":"https://example.com/hamlet.jpg"}`),
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-02T20:00:00"),
				makeJSONLDRaw("Hamlet", "https://example.com/hamlet", "2026-06-03T20:00:00"),
			},
			wantCount:   1,
			wantSkipped: 0,
			check: func(t *testing.T, valid []events.EventInput) {
				t.Helper()
				evt := valid[0]
				if evt.Description != "First night" {
					t.Errorf("expected Description=%q from first event, got %q", "First night", evt.Description)
				}
				if evt.Location == nil || evt.Location.Name != "Royal Theatre" {
					vn := "<nil>"
					if evt.Location != nil {
						vn = evt.Location.Name
					}
					t.Errorf("expected Location.Name=Royal Theatre, got %q", vn)
				}
				if len(evt.Occurrences) != 3 {
					t.Errorf("expected 3 Occurrences, got %d", len(evt.Occurrences))
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid, skipped := groupJSONLDEvents(tc.raws, src, tc.limit, logger)

			if len(valid) != tc.wantCount {
				t.Errorf("expected %d valid EventInputs, got %d", tc.wantCount, len(valid))
			}
			if skipped != tc.wantSkipped {
				t.Errorf("expected %d skipped, got %d", tc.wantSkipped, skipped)
			}
			if tc.check != nil && len(valid) > 0 {
				tc.check(t, valid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SubEvent + Grouping interaction tests
// ---------------------------------------------------------------------------

// TestGroupJSONLDEvents_SubEventPreserved verifies that when an EventSeries
// with subEvent-derived Occurrences lands in a multi-event group, the richer
// subEvent Occurrences (which include endDate and doorTime) are preserved
// rather than being overwritten by the group's top-level startDates.
func TestGroupJSONLDEvents_SubEventPreserved(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	// An EventSeries with subEvents appears twice with the same URL+Name
	// (this could happen if a page duplicates the JSON-LD block).
	seriesJSON := json.RawMessage(`{
		"@type": "EventSeries",
		"name": "Jazz Festival",
		"url": "https://example.com/jazz",
		"startDate": "2026-07-10T20:00:00",
		"subEvent": [
			{"@type": "Event", "startDate": "2026-07-10T20:00:00", "endDate": "2026-07-10T23:00:00", "doorTime": "2026-07-10T19:00:00"},
			{"@type": "Event", "startDate": "2026-07-11T19:00:00", "endDate": "2026-07-11T22:00:00"},
			{"@type": "Event", "startDate": "2026-07-12T20:00:00", "endDate": "2026-07-12T23:30:00"}
		]
	}`)

	// Two copies of the same EventSeries → forms a multi-event group.
	raws := []json.RawMessage{seriesJSON, seriesJSON}

	valid, skipped := groupJSONLDEvents(raws, src, 0, logger)

	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 EventInput, got %d", len(valid))
	}

	evt := valid[0]
	// The subEvent-derived Occurrences from NormalizeJSONLDEvent should be
	// preserved (3 occurrences with endDate+doorTime), not overwritten by
	// the 2-item group's top-level startDates.
	if len(evt.Occurrences) != 3 {
		t.Fatalf("expected 3 Occurrences from subEvents, got %d", len(evt.Occurrences))
	}
	// Verify richer metadata is preserved.
	if evt.Occurrences[0].EndDate != "2026-07-10T23:00:00" {
		t.Errorf("Occurrences[0].EndDate = %q, want 2026-07-10T23:00:00", evt.Occurrences[0].EndDate)
	}
	if evt.Occurrences[0].DoorTime != "2026-07-10T19:00:00" {
		t.Errorf("Occurrences[0].DoorTime = %q, want 2026-07-10T19:00:00", evt.Occurrences[0].DoorTime)
	}
}

// TestGroupJSONLDEvents_SingleEventWithSubEvents verifies that a single-group
// EventSeries with subEvents correctly preserves the subEvent Occurrences.
func TestGroupJSONLDEvents_SingleEventWithSubEvents(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	raws := []json.RawMessage{
		json.RawMessage(`{
			"@type": "EventSeries",
			"name": "Workshop Series",
			"url": "https://example.com/workshop",
			"startDate": "2026-08-01T10:00:00",
			"subEvent": [
				{"@type": "Event", "startDate": "2026-08-01T10:00:00", "endDate": "2026-08-01T12:00:00"},
				{"@type": "Event", "startDate": "2026-08-02T10:00:00", "endDate": "2026-08-02T12:00:00"}
			]
		}`),
	}

	valid, skipped := groupJSONLDEvents(raws, src, 0, logger)

	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 EventInput, got %d", len(valid))
	}

	evt := valid[0]
	if len(evt.Occurrences) != 2 {
		t.Errorf("expected 2 Occurrences from subEvents, got %d", len(evt.Occurrences))
	}
	if evt.Occurrences[0].EndDate != "2026-08-01T12:00:00" {
		t.Errorf("Occurrences[0].EndDate = %q, want 2026-08-01T12:00:00", evt.Occurrences[0].EndDate)
	}
}

// TestNormalizeRawEvents_AllInvalidDatesGroup verifies that when ALL rows in a
// multi-row group have unparseable/empty dates, consolidateOccurrences returns
// an error ("no valid dates found") and normalizeRawEvents correctly increments
// skipped and continues to the next group.
func TestNormalizeRawEvents_AllInvalidDatesGroup(t *testing.T) {
	t.Parallel()

	src := SourceConfig{Name: "Test", URL: "https://example.com", License: "CC0-1.0"}
	logger := zerolog.Nop()

	raws := []RawEvent{
		// Group 1: all rows have no valid dates → should be skipped
		{Name: "Bad Show", URL: "https://example.com/bad"},
		{Name: "Bad Show", URL: "https://example.com/bad"},
		{Name: "Bad Show", URL: "https://example.com/bad"},
		// Group 2: valid single event
		makeRaw("Good Show", "https://example.com/good", "2026-06-01T20:00:00"),
	}

	valid, skipped := normalizeRawEvents(raws, src, 0, logger)

	if skipped != 1 {
		t.Errorf("expected 1 skipped group (all-invalid-dates), got %d", skipped)
	}
	if len(valid) != 1 {
		t.Fatalf("expected 1 valid EventInput, got %d", len(valid))
	}
	if valid[0].Name != "Good Show" {
		t.Errorf("expected surviving event to be 'Good Show', got %q", valid[0].Name)
	}
}
