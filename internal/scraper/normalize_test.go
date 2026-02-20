package scraper

import (
	"encoding/json"
	"testing"

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
				if got.StartDate != "2026-05-10T19:00:00" {
					t.Errorf("StartDate = %q", got.StartDate)
				}
				if got.EndDate != "2026-05-10T21:00:00" {
					t.Errorf("EndDate = %q", got.EndDate)
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

// TestMustJSONHelper ensures the test helper itself works correctly.
func TestMustJSONHelper(t *testing.T) {
	raw := mustJSON("hello")
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || s != "hello" {
		t.Errorf("mustJSON round-trip failed: %v", err)
	}
}
