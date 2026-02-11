package schema

import (
	"encoding/json"
	"testing"
)

func TestEventMarshal(t *testing.T) {
	free := true
	e := NewEvent("Jazz Night")
	e.Context = "https://schema.org"
	e.ID = "https://example.com/events/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	e.Description = "A night of jazz"
	e.StartDate = "2025-06-15T20:00:00Z"
	e.EndDate = "2025-06-15T23:00:00Z"
	e.DoorTime = "2025-06-15T19:30:00Z"
	e.Image = "https://example.com/jazz.jpg"
	e.URL = "https://example.com/jazz-night"
	e.Keywords = []string{"jazz", "music"}
	e.InLanguage = []string{"en"}
	e.IsAccessibleForFree = &free
	e.EventStatus = "https://schema.org/EventScheduled"
	e.EventAttendanceMode = "https://schema.org/OfflineEventAttendanceMode"
	e.License = "https://creativecommons.org/publicdomain/zero/1.0/"
	e.SameAs = []string{"https://other.example.com/events/123"}
	e.Location = NewPlace("The Jazz Club")
	e.Organizer = NewOrganization("Jazz Society")

	price := 25.0
	offer := NewOffer()
	offer.Price = &price
	offer.PriceCurrency = "CAD"
	offer.URL = "https://example.com/tickets"
	e.Offers = []Offer{*offer}

	e.SubEvents = []EventSummary{
		*NewEventSummary("Set 1"),
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal Event: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	// Verify schema.org property names
	checks := map[string]any{
		"@context":            "https://schema.org",
		"@type":               "Event",
		"@id":                 "https://example.com/events/01ARZ3NDEKTSV4RRFFQ69G5FAV",
		"name":                "Jazz Night",
		"description":         "A night of jazz",
		"startDate":           "2025-06-15T20:00:00Z",
		"endDate":             "2025-06-15T23:00:00Z",
		"doorTime":            "2025-06-15T19:30:00Z",
		"image":               "https://example.com/jazz.jpg",
		"url":                 "https://example.com/jazz-night",
		"eventStatus":         "https://schema.org/EventScheduled",
		"eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
		"license":             "https://creativecommons.org/publicdomain/zero/1.0/",
		"isAccessibleForFree": true,
	}
	for key, want := range checks {
		got, ok := m[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		// JSON numbers are float64
		switch w := want.(type) {
		case bool:
			if got != w {
				t.Errorf("key %q: got %v, want %v", key, got, w)
			}
		default:
			if got != want {
				t.Errorf("key %q: got %v, want %v", key, got, want)
			}
		}
	}

	// Check arrays
	kw, ok := m["keywords"].([]any)
	if !ok || len(kw) != 2 {
		t.Errorf("keywords: got %v", m["keywords"])
	}

	sa, ok := m["sameAs"].([]any)
	if !ok || len(sa) != 1 {
		t.Errorf("sameAs: got %v", m["sameAs"])
	}

	offers, ok := m["offers"].([]any)
	if !ok || len(offers) != 1 {
		t.Errorf("offers: got %v", m["offers"])
	}

	subs, ok := m["subEvent"].([]any)
	if !ok || len(subs) != 1 {
		t.Errorf("subEvent: got %v", m["subEvent"])
	}

	// Check nested location type
	loc, ok := m["location"].(map[string]any)
	if !ok {
		t.Fatalf("location not a map: %v", m["location"])
	}
	if loc["@type"] != "Place" {
		t.Errorf("location @type: got %v, want Place", loc["@type"])
	}

	// Check nested organizer type
	org, ok := m["organizer"].(map[string]any)
	if !ok {
		t.Fatalf("organizer not a map: %v", m["organizer"])
	}
	if org["@type"] != "Organization" {
		t.Errorf("organizer @type: got %v, want Organization", org["@type"])
	}
}

func TestPlaceMarshal(t *testing.T) {
	p := NewPlace("The Jazz Club")
	p.Context = "https://schema.org"
	p.ID = "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	p.Description = "A cozy jazz club"
	p.Address = NewPostalAddress("123 Main St", "Toronto", "ON", "M5V 1A1", "CA")
	p.Geo = NewGeoCoordinates(43.6532, -79.3832)
	p.Telephone = "+1-416-555-0100"
	p.Email = "info@jazzclub.example.com"
	p.URL = "https://jazzclub.example.com"
	p.MaximumAttendeeCapacity = 200

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal Place: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if m["@type"] != "Place" {
		t.Errorf("@type: got %v, want Place", m["@type"])
	}
	if m["name"] != "The Jazz Club" {
		t.Errorf("name: got %v", m["name"])
	}
	if m["telephone"] != "+1-416-555-0100" {
		t.Errorf("telephone: got %v", m["telephone"])
	}
	if m["maximumAttendeeCapacity"] != float64(200) {
		t.Errorf("maximumAttendeeCapacity: got %v", m["maximumAttendeeCapacity"])
	}

	// Check nested address
	addr, ok := m["address"].(map[string]any)
	if !ok {
		t.Fatalf("address not a map: %v", m["address"])
	}
	if addr["@type"] != "PostalAddress" {
		t.Errorf("address @type: got %v", addr["@type"])
	}
	if addr["streetAddress"] != "123 Main St" {
		t.Errorf("streetAddress: got %v", addr["streetAddress"])
	}
	if addr["addressLocality"] != "Toronto" {
		t.Errorf("addressLocality: got %v", addr["addressLocality"])
	}
	if addr["addressRegion"] != "ON" {
		t.Errorf("addressRegion: got %v", addr["addressRegion"])
	}
	if addr["postalCode"] != "M5V 1A1" {
		t.Errorf("postalCode: got %v", addr["postalCode"])
	}
	if addr["addressCountry"] != "CA" {
		t.Errorf("addressCountry: got %v", addr["addressCountry"])
	}

	// Check nested geo
	geo, ok := m["geo"].(map[string]any)
	if !ok {
		t.Fatalf("geo not a map: %v", m["geo"])
	}
	if geo["@type"] != "GeoCoordinates" {
		t.Errorf("geo @type: got %v", geo["@type"])
	}
	if geo["latitude"] != 43.6532 {
		t.Errorf("latitude: got %v", geo["latitude"])
	}
	if geo["longitude"] != -79.3832 {
		t.Errorf("longitude: got %v", geo["longitude"])
	}
}

func TestOrganizationMarshal(t *testing.T) {
	o := NewOrganization("Jazz Society")
	o.Context = "https://schema.org"
	o.ID = "https://example.com/organizations/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	o.LegalName = "Toronto Jazz Society Inc."
	o.Description = "Promoting jazz in Toronto"
	o.URL = "https://jazzsociety.example.com"
	o.Email = "contact@jazzsociety.example.com"
	o.Telephone = "+1-416-555-0200"
	o.Address = NewPostalAddress("456 Queen St", "Toronto", "ON", "M5V 2B2", "CA")

	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal Organization: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if m["@type"] != "Organization" {
		t.Errorf("@type: got %v, want Organization", m["@type"])
	}
	if m["name"] != "Jazz Society" {
		t.Errorf("name: got %v", m["name"])
	}
	if m["legalName"] != "Toronto Jazz Society Inc." {
		t.Errorf("legalName: got %v", m["legalName"])
	}
	if m["description"] != "Promoting jazz in Toronto" {
		t.Errorf("description: got %v", m["description"])
	}
	if m["email"] != "contact@jazzsociety.example.com" {
		t.Errorf("email: got %v", m["email"])
	}

	addr, ok := m["address"].(map[string]any)
	if !ok {
		t.Fatalf("address not a map: %v", m["address"])
	}
	if addr["@type"] != "PostalAddress" {
		t.Errorf("address @type: got %v", addr["@type"])
	}
}

func TestOmitEmptyFields(t *testing.T) {
	// Event with only required fields
	e := NewEvent("Minimal Event")
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// These should be present
	if m["@type"] != "Event" {
		t.Errorf("@type missing or wrong")
	}
	if m["name"] != "Minimal Event" {
		t.Errorf("name missing or wrong")
	}

	// These should be omitted
	omitted := []string{
		"@context", "@id", "description", "startDate", "endDate", "doorTime",
		"location", "organizer", "image", "url", "keywords", "inLanguage",
		"isAccessibleForFree", "eventStatus", "eventAttendanceMode",
		"offers", "license", "sameAs", "subEvent",
	}
	for _, key := range omitted {
		if _, ok := m[key]; ok {
			t.Errorf("expected %q to be omitted, but it was present", key)
		}
	}

	// Place with only required fields
	p := NewPlace("Minimal Place")
	data, err = json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal Place: %v", err)
	}

	m = map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	placeOmitted := []string{
		"@context", "@id", "description", "address", "geo",
		"telephone", "email", "url", "maximumAttendeeCapacity",
	}
	for _, key := range placeOmitted {
		if _, ok := m[key]; ok {
			t.Errorf("expected %q to be omitted from Place, but it was present", key)
		}
	}
}

func TestNewPostalAddressNilWhenAllEmpty(t *testing.T) {
	addr := NewPostalAddress("", "", "", "", "")
	if addr != nil {
		t.Error("expected nil when all fields empty")
	}
}

func TestNewPostalAddressNonNil(t *testing.T) {
	addr := NewPostalAddress("", "Toronto", "", "", "")
	if addr == nil {
		t.Fatal("expected non-nil when locality provided")
	}
	if addr.Type != "PostalAddress" {
		t.Errorf("Type: got %q, want PostalAddress", addr.Type)
	}
	if addr.AddressLocality != "Toronto" {
		t.Errorf("AddressLocality: got %q", addr.AddressLocality)
	}
}

func TestNewGeoCoordinatesNilWhenZero(t *testing.T) {
	geo := NewGeoCoordinates(0, 0)
	if geo != nil {
		t.Error("expected nil when lat/lng are zero")
	}
}

func TestNewGeoCoordinatesNonNil(t *testing.T) {
	geo := NewGeoCoordinates(43.6532, -79.3832)
	if geo == nil {
		t.Fatal("expected non-nil for valid coordinates")
	}
	if geo.Type != "GeoCoordinates" {
		t.Errorf("Type: got %q, want GeoCoordinates", geo.Type)
	}
	if *geo.Latitude != 43.6532 {
		t.Errorf("Latitude: got %v", *geo.Latitude)
	}
	if *geo.Longitude != -79.3832 {
		t.Errorf("Longitude: got %v", *geo.Longitude)
	}
}

func TestListResponseMarshal(t *testing.T) {
	events := []EventSummary{
		*NewEventSummary("Event A"),
		*NewEventSummary("Event B"),
	}

	lr := NewListResponse(events)
	lr.Context = "https://schema.org"
	lr.NextCursor = "cursor123"
	lr.TotalItems = 42

	data, err := json.Marshal(lr)
	if err != nil {
		t.Fatalf("marshal ListResponse: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if m["@type"] != "ItemList" {
		t.Errorf("@type: got %v, want ItemList", m["@type"])
	}
	if m["@context"] != "https://schema.org" {
		t.Errorf("@context: got %v", m["@context"])
	}
	if m["nextCursor"] != "cursor123" {
		t.Errorf("nextCursor: got %v", m["nextCursor"])
	}
	if m["totalItems"] != float64(42) {
		t.Errorf("totalItems: got %v", m["totalItems"])
	}

	items, ok := m["itemListElement"].([]any)
	if !ok {
		t.Fatalf("itemListElement not an array: %v", m["itemListElement"])
	}
	if len(items) != 2 {
		t.Errorf("itemListElement length: got %d, want 2", len(items))
	}
}

func TestListResponseOmitsEmptyOptionals(t *testing.T) {
	lr := NewListResponse([]EventSummary{})

	data, err := json.Marshal(lr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := m["nextCursor"]; ok {
		t.Error("expected nextCursor to be omitted")
	}
	if _, ok := m["totalItems"]; ok {
		t.Error("expected totalItems to be omitted")
	}
	// itemListElement should always be present (not omitempty)
	if _, ok := m["itemListElement"]; !ok {
		t.Error("expected itemListElement to be present")
	}
}

func TestVirtualLocationMarshal(t *testing.T) {
	vl := NewVirtualLocation("https://zoom.us/j/123456")
	vl.Name = "Zoom Meeting"

	data, err := json.Marshal(vl)
	if err != nil {
		t.Fatalf("marshal VirtualLocation: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if m["@type"] != "VirtualLocation" {
		t.Errorf("@type: got %v, want VirtualLocation", m["@type"])
	}
	if m["url"] != "https://zoom.us/j/123456" {
		t.Errorf("url: got %v", m["url"])
	}
	if m["name"] != "Zoom Meeting" {
		t.Errorf("name: got %v", m["name"])
	}
}

func TestVirtualLocationOmitsEmptyName(t *testing.T) {
	vl := NewVirtualLocation("https://zoom.us/j/123456")

	data, err := json.Marshal(vl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := m["name"]; ok {
		t.Error("expected name to be omitted when empty")
	}
}

func TestBuildEventURI(t *testing.T) {
	uri := BuildEventURI("example.com", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	want := "https://example.com/events/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if uri != want {
		t.Errorf("BuildEventURI: got %q, want %q", uri, want)
	}
}

func TestBuildPlaceURI(t *testing.T) {
	uri := BuildPlaceURI("example.com", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	want := "https://example.com/places/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if uri != want {
		t.Errorf("BuildPlaceURI: got %q, want %q", uri, want)
	}
}

func TestBuildOrganizationURI(t *testing.T) {
	uri := BuildOrganizationURI("example.com", "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	want := "https://example.com/organizations/01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if uri != want {
		t.Errorf("BuildOrganizationURI: got %q, want %q", uri, want)
	}
}

func TestBuildURIInvalidULID(t *testing.T) {
	uri := BuildEventURI("example.com", "not-a-ulid")
	if uri != "" {
		t.Errorf("expected empty string for invalid ULID, got %q", uri)
	}
}

func TestGeoCoordinatesZeroValues(t *testing.T) {
	// Directly construct GeoCoordinates for the rare zero-zero case
	lat, lng := 0.0, 0.0
	geo := &GeoCoordinates{
		Type:      "GeoCoordinates",
		Latitude:  &lat,
		Longitude: &lng,
	}

	data, err := json.Marshal(geo)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// With pointer fields, zero values should be serialized
	if m["latitude"] != 0.0 {
		t.Errorf("latitude: got %v, want 0", m["latitude"])
	}
	if m["longitude"] != 0.0 {
		t.Errorf("longitude: got %v, want 0", m["longitude"])
	}
}

func TestOfferPriceZero(t *testing.T) {
	// Explicitly set price to zero (free event)
	price := 0.0
	o := NewOffer()
	o.Price = &price
	o.PriceCurrency = "CAD"

	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Price=0 should be present (pointer is non-nil)
	if m["price"] != 0.0 {
		t.Errorf("price: got %v, want 0", m["price"])
	}
}

func TestOfferPriceOmittedWhenNil(t *testing.T) {
	o := NewOffer()
	o.PriceCurrency = "CAD"

	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := m["price"]; ok {
		t.Error("expected price to be omitted when nil")
	}
}
