package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- PlaceInput UnmarshalJSON tests ---

func TestPlaceInput_UnmarshalJSON_FlatFormat(t *testing.T) {
	data := `{
		"name": "The Venue",
		"streetAddress": "123 Main St",
		"addressLocality": "Toronto",
		"addressRegion": "ON",
		"postalCode": "M5V 1A1",
		"addressCountry": "CA",
		"latitude": 43.6532,
		"longitude": -79.3832
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "The Venue", place.Name)
	assert.Equal(t, "123 Main St", place.StreetAddress)
	assert.Equal(t, "Toronto", place.AddressLocality)
	assert.Equal(t, "ON", place.AddressRegion)
	assert.Equal(t, "M5V 1A1", place.PostalCode)
	assert.Equal(t, "CA", place.AddressCountry)
	assert.InDelta(t, 43.6532, place.Latitude, 0.0001)
	assert.InDelta(t, -79.3832, place.Longitude, 0.0001)
}

func TestPlaceInput_UnmarshalJSON_NestedSchemaOrg(t *testing.T) {
	data := `{
		"@type": "Place",
		"name": "The Blue Note",
		"address": {
			"@type": "PostalAddress",
			"streetAddress": "131 W 3rd St",
			"addressLocality": "New York",
			"addressRegion": "NY",
			"postalCode": "10012",
			"addressCountry": "US"
		},
		"geo": {
			"@type": "GeoCoordinates",
			"latitude": 40.7306,
			"longitude": -73.9996
		}
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "The Blue Note", place.Name)
	assert.Equal(t, "131 W 3rd St", place.StreetAddress)
	assert.Equal(t, "New York", place.AddressLocality)
	assert.Equal(t, "NY", place.AddressRegion)
	assert.Equal(t, "10012", place.PostalCode)
	assert.Equal(t, "US", place.AddressCountry)
	assert.InDelta(t, 40.7306, place.Latitude, 0.0001)
	assert.InDelta(t, -73.9996, place.Longitude, 0.0001)
}

func TestPlaceInput_UnmarshalJSON_AddressAsString(t *testing.T) {
	data := `{
		"name": "Some Place",
		"address": "123 Main St, Toronto, ON"
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "Some Place", place.Name)
	assert.Equal(t, "123 Main St, Toronto, ON", place.StreetAddress)
}

func TestPlaceInput_UnmarshalJSON_NestedOverridesFlat(t *testing.T) {
	// If both flat and nested are present, nested should win
	data := `{
		"name": "The Venue",
		"streetAddress": "Flat Street",
		"addressLocality": "Flat City",
		"address": {
			"streetAddress": "Nested Street",
			"addressLocality": "Nested City"
		}
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "Nested Street", place.StreetAddress)
	assert.Equal(t, "Nested City", place.AddressLocality)
}

func TestPlaceInput_UnmarshalJSON_IDOnly(t *testing.T) {
	data := `{"@id": "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV"}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV", place.ID)
}

func TestPlaceInput_UnmarshalJSON_StringLatLon(t *testing.T) {
	data := `{
		"name": "String Coords Venue",
		"latitude": "43.6532",
		"longitude": "-79.3832"
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "String Coords Venue", place.Name)
	assert.InDelta(t, 43.6532, place.Latitude, 0.0001)
	assert.InDelta(t, -79.3832, place.Longitude, 0.0001)
}

func TestPlaceInput_UnmarshalJSON_NestedGeoStringLatLon(t *testing.T) {
	data := `{
		"@type": "Place",
		"name": "String Geo Venue",
		"geo": {
			"@type": "GeoCoordinates",
			"latitude": "40.7306",
			"longitude": "-73.9996"
		}
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, "String Geo Venue", place.Name)
	assert.InDelta(t, 40.7306, place.Latitude, 0.0001)
	assert.InDelta(t, -73.9996, place.Longitude, 0.0001)
}

func TestPlaceInput_UnmarshalJSON_EmptyStringLatLon(t *testing.T) {
	data := `{
		"name": "Empty Coords Venue",
		"latitude": "",
		"longitude": ""
	}`

	var place PlaceInput
	err := json.Unmarshal([]byte(data), &place)
	require.NoError(t, err)
	assert.Equal(t, float64(0), place.Latitude)
	assert.Equal(t, float64(0), place.Longitude)
}

// --- OrganizationInput tests ---

func TestOrganizationInput_WithEmailTelephone(t *testing.T) {
	data := `{
		"@type": "Organization",
		"name": "Arts Foundation",
		"url": "https://arts.org",
		"email": "info@arts.org",
		"telephone": "+1-555-0123"
	}`

	var org OrganizationInput
	err := json.Unmarshal([]byte(data), &org)
	require.NoError(t, err)
	assert.Equal(t, "Arts Foundation", org.Name)
	assert.Equal(t, "https://arts.org", org.URL)
	assert.Equal(t, "info@arts.org", org.Email)
	assert.Equal(t, "+1-555-0123", org.Telephone)
}

// --- OfferInput UnmarshalJSON tests ---

func TestOfferInput_UnmarshalJSON_PriceAsString(t *testing.T) {
	data := `{"price": "25.00", "priceCurrency": "CAD"}`

	var offer OfferInput
	err := json.Unmarshal([]byte(data), &offer)
	require.NoError(t, err)
	assert.Equal(t, "25.00", offer.Price)
	assert.Equal(t, "CAD", offer.PriceCurrency)
}

func TestOfferInput_UnmarshalJSON_PriceAsNumber(t *testing.T) {
	data := `{"price": 35.00, "priceCurrency": "USD"}`

	var offer OfferInput
	err := json.Unmarshal([]byte(data), &offer)
	require.NoError(t, err)
	assert.Equal(t, "35", offer.Price)
	assert.Equal(t, "USD", offer.PriceCurrency)
}

func TestOfferInput_UnmarshalJSON_PriceAsDecimalNumber(t *testing.T) {
	data := `{"price": 25.50, "priceCurrency": "CAD"}`

	var offer OfferInput
	err := json.Unmarshal([]byte(data), &offer)
	require.NoError(t, err)
	assert.Equal(t, "25.5", offer.Price)
}

func TestOfferInput_UnmarshalJSON_WithURL(t *testing.T) {
	data := `{"url": "https://tickets.example.com", "price": 10, "priceCurrency": "CAD"}`

	var offer OfferInput
	err := json.Unmarshal([]byte(data), &offer)
	require.NoError(t, err)
	assert.Equal(t, "https://tickets.example.com", offer.URL)
	assert.Equal(t, "10", offer.Price)
}

func TestOfferInput_UnmarshalJSON_ZeroPrice(t *testing.T) {
	data := `{"price": 0, "priceCurrency": "USD"}`

	var offer OfferInput
	err := json.Unmarshal([]byte(data), &offer)
	require.NoError(t, err)
	assert.Equal(t, "0", offer.Price)
}

// --- EventInput UnmarshalJSON tests ---

func TestEventInput_UnmarshalJSON_WithContext(t *testing.T) {
	data := `{
		"@context": "https://schema.org",
		"@type": "Event",
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Test Venue"}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "Test Event", event.Name)
	assert.Equal(t, "Event", event.Type)
	assert.NotNil(t, event.Context) // Context is captured but not used
}

func TestEventInput_UnmarshalJSON_LocationAsText(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": "The Venue, 123 Main St, Toronto"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.NotNil(t, event.Location)
	assert.Equal(t, "The Venue, 123 Main St, Toronto", event.Location.Name)
}

func TestEventInput_UnmarshalJSON_LocationAsObject(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {
			"name": "The Venue",
			"streetAddress": "123 Main St"
		}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.NotNil(t, event.Location)
	assert.Equal(t, "The Venue", event.Location.Name)
	assert.Equal(t, "123 Main St", event.Location.StreetAddress)
}

func TestEventInput_UnmarshalJSON_ImageAsString(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"image": "https://example.com/photo.jpg"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/photo.jpg", event.Image)
}

func TestEventInput_UnmarshalJSON_ImageAsObject(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"image": {
			"@type": "ImageObject",
			"url": "https://example.com/photo.jpg"
		}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/photo.jpg", event.Image)
}

func TestEventInput_UnmarshalJSON_ImageAsObjectContentUrl(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"image": {
			"@type": "ImageObject",
			"contentUrl": "https://example.com/photo.jpg"
		}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/photo.jpg", event.Image)
}

func TestEventInput_UnmarshalJSON_OrganizerAsString(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"organizer": "https://example.com/organizations/ABC123"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.NotNil(t, event.Organizer)
	assert.Equal(t, "https://example.com/organizations/ABC123", event.Organizer.ID)
}

func TestEventInput_UnmarshalJSON_OrganizerAsObject(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"organizer": {
			"@type": "Organization",
			"name": "Arts Corp",
			"url": "https://arts.org"
		}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.NotNil(t, event.Organizer)
	assert.Equal(t, "Arts Corp", event.Organizer.Name)
	assert.Equal(t, "https://arts.org", event.Organizer.URL)
}

func TestEventInput_UnmarshalJSON_InLanguageAsString(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"inLanguage": "en"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, []string{"en"}, event.InLanguage)
}

func TestEventInput_UnmarshalJSON_InLanguageAsArray(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"inLanguage": ["en", "fr"]
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, []string{"en", "fr"}, event.InLanguage)
}

func TestEventInput_UnmarshalJSON_KeywordsAsString(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"keywords": "music, jazz, live performance"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, []string{"music", "jazz", "live performance"}, event.Keywords)
}

func TestEventInput_UnmarshalJSON_KeywordsAsArray(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"keywords": ["music", "jazz"]
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, []string{"music", "jazz"}, event.Keywords)
}

func TestEventInput_UnmarshalJSON_OffersSingle(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"offers": {"price": 25.00, "priceCurrency": "CAD"}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.NotNil(t, event.Offers)
	assert.Equal(t, "25", event.Offers.Price)
	assert.Equal(t, "CAD", event.Offers.PriceCurrency)
}

func TestEventInput_UnmarshalJSON_OffersArray(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"offers": [
			{"price": 25.00, "priceCurrency": "CAD"},
			{"price": 50.00, "priceCurrency": "CAD"}
		]
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.NotNil(t, event.Offers)
	assert.Equal(t, "25", event.Offers.Price)
}

// --- Event subtype mapping tests ---

func TestNormalizeEventInput_EventSubtypeDomain(t *testing.T) {
	tests := []struct {
		name       string
		inputType  string
		wantDomain string
	}{
		{"MusicEvent maps to music", "MusicEvent", "music"},
		{"DanceEvent maps to arts", "DanceEvent", "arts"},
		{"SportsEvent maps to sports", "SportsEvent", "sports"},
		{"EducationEvent maps to education", "EducationEvent", "education"},
		{"generic Event has no mapping", "Event", ""},
		{"unknown type has no mapping", "CustomEvent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := EventInput{
				Name:      "Test Event",
				Type:      tt.inputType,
				StartDate: "2026-02-01T10:00:00Z",
			}
			result := NormalizeEventInput(input)
			assert.Equal(t, tt.wantDomain, result.EventDomain)
		})
	}
}

func TestNormalizeEventInput_EventSubtypeDomainNoOverride(t *testing.T) {
	// If EventDomain is already set, don't override from @type
	input := EventInput{
		Name:        "Test Event",
		Type:        "MusicEvent",
		EventDomain: "community",
		StartDate:   "2026-02-01T10:00:00Z",
	}
	result := NormalizeEventInput(input)
	assert.Equal(t, "community", result.EventDomain)
}

// --- Full schema.org round-trip test ---

func TestEventInput_UnmarshalJSON_FullSchemaOrgPayload(t *testing.T) {
	data := `{
		"@context": "https://schema.org",
		"@type": "MusicEvent",
		"name": "Jazz Night",
		"startDate": "2025-06-15T20:00:00-04:00",
		"endDate": "2025-06-15T23:00:00-04:00",
		"location": {
			"@type": "Place",
			"name": "The Blue Note",
			"address": {
				"@type": "PostalAddress",
				"streetAddress": "131 W 3rd St",
				"addressLocality": "New York",
				"addressRegion": "NY",
				"postalCode": "10012",
				"addressCountry": "US"
			},
			"geo": {
				"@type": "GeoCoordinates",
				"latitude": 40.7306,
				"longitude": -73.9996
			}
		},
		"organizer": {
			"@type": "Organization",
			"name": "Blue Note Entertainment",
			"url": "https://bluenotejazz.com",
			"email": "info@bluenotejazz.com"
		},
		"offers": {
			"@type": "Offer",
			"price": 35.00,
			"priceCurrency": "USD",
			"url": "https://bluenotejazz.com/tickets"
		},
		"image": "https://bluenotejazz.com/images/jazz-night.jpg",
		"description": "An evening of classic jazz performances",
		"keywords": "jazz, music, live performance",
		"inLanguage": "en",
		"isAccessibleForFree": false
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)

	// Event fields
	assert.Equal(t, "MusicEvent", event.Type)
	assert.Equal(t, "Jazz Night", event.Name)
	assert.Equal(t, "2025-06-15T20:00:00-04:00", event.StartDate)
	assert.Equal(t, "2025-06-15T23:00:00-04:00", event.EndDate)
	assert.Equal(t, "An evening of classic jazz performances", event.Description)
	assert.NotNil(t, event.Context)
	assert.Equal(t, false, *event.IsAccessibleForFree)

	// Location (nested schema.org format)
	require.NotNil(t, event.Location)
	assert.Equal(t, "The Blue Note", event.Location.Name)
	assert.Equal(t, "131 W 3rd St", event.Location.StreetAddress)
	assert.Equal(t, "New York", event.Location.AddressLocality)
	assert.Equal(t, "NY", event.Location.AddressRegion)
	assert.Equal(t, "10012", event.Location.PostalCode)
	assert.Equal(t, "US", event.Location.AddressCountry)
	assert.InDelta(t, 40.7306, event.Location.Latitude, 0.0001)
	assert.InDelta(t, -73.9996, event.Location.Longitude, 0.0001)

	// Organizer (with email)
	require.NotNil(t, event.Organizer)
	assert.Equal(t, "Blue Note Entertainment", event.Organizer.Name)
	assert.Equal(t, "https://bluenotejazz.com", event.Organizer.URL)
	assert.Equal(t, "info@bluenotejazz.com", event.Organizer.Email)

	// Offers (price as number)
	require.NotNil(t, event.Offers)
	assert.Equal(t, "35", event.Offers.Price)
	assert.Equal(t, "USD", event.Offers.PriceCurrency)
	assert.Equal(t, "https://bluenotejazz.com/tickets", event.Offers.URL)

	// Image (string)
	assert.Equal(t, "https://bluenotejazz.com/images/jazz-night.jpg", event.Image)

	// Keywords (comma-separated string)
	assert.Equal(t, []string{"jazz", "music", "live performance"}, event.Keywords)

	// InLanguage (single string)
	assert.Equal(t, []string{"en"}, event.InLanguage)

	// Normalization should map MusicEvent -> music domain
	normalized := NormalizeEventInput(event)
	assert.Equal(t, "music", normalized.EventDomain)
}

// --- Backward compatibility: flat format still works ---

func TestEventInput_UnmarshalJSON_FlatFormatBackwardCompatible(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {
			"name": "Test Venue",
			"addressLocality": "Toronto"
		},
		"keywords": ["music", "jazz"],
		"inLanguage": ["en"],
		"offers": {"price": "25.00", "priceCurrency": "CAD"},
		"image": "https://example.com/img.jpg"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "Test Event", event.Name)
	require.NotNil(t, event.Location)
	assert.Equal(t, "Test Venue", event.Location.Name)
	assert.Equal(t, "Toronto", event.Location.AddressLocality)
	assert.Equal(t, []string{"music", "jazz"}, event.Keywords)
	assert.Equal(t, []string{"en"}, event.InLanguage)
	require.NotNil(t, event.Offers)
	assert.Equal(t, "25.00", event.Offers.Price)
	assert.Equal(t, "https://example.com/img.jpg", event.Image)
}

func TestEventInput_UnmarshalJSON_NilOptionalFields(t *testing.T) {
	// Minimal event — all optional flexible fields absent
	data := `{
		"name": "Minimal Event",
		"startDate": "2026-02-01T10:00:00Z"
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "Minimal Event", event.Name)
	assert.Nil(t, event.Location)
	assert.Nil(t, event.Organizer)
	assert.Nil(t, event.Offers)
	assert.Empty(t, event.Image)
	assert.Nil(t, event.Keywords)
	assert.Nil(t, event.InLanguage)
}

// --- Struct literal construction backward compatibility ---

func TestEventInput_StructLiteral_BackwardCompatible(t *testing.T) {
	// This tests that existing code constructing EventInput via struct literals
	// still works. The new fields (Context, EventDomain) have zero values.
	input := EventInput{
		Name:      "Test Event",
		StartDate: "2026-02-01T10:00:00Z",
		Location: &PlaceInput{
			Name:            "Test Venue",
			AddressLocality: "Toronto",
		},
	}
	assert.Equal(t, "Test Event", input.Name)
	assert.Equal(t, "", input.EventDomain)
	assert.Nil(t, input.Context)
}

func TestPlaceInput_StructLiteral_BackwardCompatible(t *testing.T) {
	// Struct literal construction must still work
	place := PlaceInput{
		Name:            "Test Venue",
		StreetAddress:   "123 Main St",
		AddressLocality: "Toronto",
		AddressRegion:   "ON",
		Latitude:        43.6532,
		Longitude:       -79.3832,
	}
	assert.Equal(t, "Test Venue", place.Name)
	assert.Equal(t, "123 Main St", place.StreetAddress)
}

func TestOrganizationInput_StructLiteral_BackwardCompatible(t *testing.T) {
	org := OrganizationInput{
		Name: "Test Org",
		URL:  "https://example.com",
	}
	assert.Equal(t, "Test Org", org.Name)
	assert.Equal(t, "", org.Email)
	assert.Equal(t, "", org.Telephone)
}

func TestOfferInput_StructLiteral_BackwardCompatible(t *testing.T) {
	offer := OfferInput{
		Price:         "25.00",
		PriceCurrency: "CAD",
	}
	assert.Equal(t, "25.00", offer.Price)
}

// --- Edge cases ---

func TestEventInput_UnmarshalJSON_ContextAsObject(t *testing.T) {
	// Schema.org context can be a string or object
	data := `{
		"@context": {"@vocab": "https://schema.org/"},
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"}
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, "Test Event", event.Name)
	assert.NotNil(t, event.Context)
}

func TestEventInput_UnmarshalJSON_EmptyOffersArray(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"offers": []
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	// Empty array → nil offers (unmarshalFlexibleOffer returns nil for empty array)
	assert.Nil(t, event.Offers)
}

func TestEventInput_UnmarshalJSON_KeywordsEmptyString(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"keywords": ""
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Empty(t, event.Keywords)
}

func TestUnmarshalFlexibleOffer_ArrayFirstElement(t *testing.T) {
	// Verify that when offers is an array, we use the first element
	data := `[
		{"price": "10", "priceCurrency": "CAD", "url": "https://first.com"},
		{"price": "20", "priceCurrency": "USD", "url": "https://second.com"}
	]`

	offer, err := unmarshalFlexibleOffer(json.RawMessage(data))
	require.NoError(t, err)
	require.NotNil(t, offer)
	assert.Equal(t, "10", offer.Price)
	assert.Equal(t, "https://first.com", offer.URL)
}

func TestEventInput_UnmarshalJSON_SameAsPreserved(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"sameAs": ["https://external.com/events/123"]
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://external.com/events/123"}, event.SameAs)
}

func TestEventInput_UnmarshalJSON_OccurrencesPreserved(t *testing.T) {
	data := `{
		"name": "Test Event",
		"startDate": "2026-02-01T10:00:00Z",
		"location": {"name": "Venue"},
		"occurrences": [
			{"startDate": "2026-02-01T10:00:00Z", "endDate": "2026-02-01T12:00:00Z"}
		]
	}`

	var event EventInput
	err := json.Unmarshal([]byte(data), &event)
	require.NoError(t, err)
	require.Len(t, event.Occurrences, 1)
	assert.Equal(t, "2026-02-01T10:00:00Z", event.Occurrences[0].StartDate)
}
