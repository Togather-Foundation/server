package events

import (
	"context"
	"errors"
	"testing"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/geoloc"
)

type stubLocationResolver struct {
	parts   geoloc.Parts
	ok      bool
	calls   int
	lastRaw string
}

func (s *stubLocationResolver) Resolve(_ context.Context, raw string) (geoloc.Parts, bool) {
	s.calls++
	s.lastRaw = raw
	return s.parts, s.ok
}

func torontoBoundary() config.GeographicBoundaryConfig {
	return config.GeographicBoundaryConfig{
		Regions:    []string{"Ontario"},
		Localities: []string{"Toronto"},
	}
}

// misparsedInput reproduces what the ICS mapper produced before the country fix:
// the region is the country and the locality is the leftover region+postal code.
func misparsedInput() EventInput {
	input := completeEventInput("Creeds Coffee Bar")
	input.Location = &PlaceInput{
		Name:            "Creeds Coffee Bar, 450 Dupont St, Toronto, ON M5R 3P3, Canada",
		StreetAddress:   "Creeds Coffee Bar, 450 Dupont St, Toronto, ON M5R 3P3, Canada",
		AddressLocality: "ON M5R 3P3",
		AddressRegion:   "Canada",
		AddressCountry:  "CA",
	}
	return input
}

func TestIngest_RejectsMisparsedLocationWithoutResolver(t *testing.T) {
	service := newTestService(NewMockRepository()).WithGeographicBoundaryConfig(torontoBoundary())

	_, err := service.Ingest(context.Background(), misparsedInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
}

func TestIngest_GeocodesLocationBeforeRejecting(t *testing.T) {
	resolver := &stubLocationResolver{
		ok: true,
		parts: geoloc.Parts{
			Locality:    "Toronto",
			Region:      "Ontario",
			Country:     "Canada",
			CountryCode: "ca",
			PostalCode:  "M5R 3P3",
		},
	}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	result, err := service.Ingest(context.Background(), misparsedInput())
	if err != nil {
		t.Fatalf("Ingest() error = %v, want the geocoder to place the event", err)
	}
	if result == nil || result.Event == nil {
		t.Fatal("expected an event")
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1", resolver.calls)
	}
	if resolver.lastRaw != "Creeds Coffee Bar, 450 Dupont St, Toronto, ON M5R 3P3, Canada" {
		t.Errorf("resolver raw = %q, want the location string", resolver.lastRaw)
	}
}

func TestIngest_StillRejectsWhenGeocoderConfirmsOutside(t *testing.T) {
	resolver := &stubLocationResolver{
		ok:    true,
		parts: geoloc.Parts{Locality: "Oakville", Region: "Ontario", Country: "Canada"},
	}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	_, err := service.Ingest(context.Background(), misparsedInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1", resolver.calls)
	}
}

func TestIngest_UnresolvedLocationKeepsTheOriginalDecision(t *testing.T) {
	resolver := &stubLocationResolver{ok: false}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	_, err := service.Ingest(context.Background(), misparsedInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
}

func TestApplyResolvedLocation(t *testing.T) {
	lat, lon := 43.67, -79.41
	loc := &PlaceInput{
		Name:            "Creeds Coffee Bar",
		AddressLocality: "ON M5R 3P3",
		AddressRegion:   "Canada",
		AddressCountry:  "",
	}
	applyResolvedLocation(loc, geoloc.Parts{
		Locality:    "Toronto",
		Region:      "Ontario",
		CountryCode: "ca",
		PostalCode:  "M5R 3P3",
		Street:      "450 Dupont St",
		Latitude:    &lat,
		Longitude:   &lon,
	})

	if loc.AddressLocality != "Toronto" || loc.AddressRegion != "Ontario" {
		t.Errorf("locality/region = %q/%q", loc.AddressLocality, loc.AddressRegion)
	}
	if loc.AddressCountry != "CA" {
		t.Errorf("country = %q, want CA from the country code", loc.AddressCountry)
	}
	if loc.PostalCode != "M5R 3P3" || loc.StreetAddress != "450 Dupont St" {
		t.Errorf("postal/street = %q/%q", loc.PostalCode, loc.StreetAddress)
	}
	if loc.Latitude != lat || loc.Longitude != lon {
		t.Errorf("coords = %v,%v", loc.Latitude, loc.Longitude)
	}
	if loc.Name != "Creeds Coffee Bar" {
		t.Errorf("name = %q, the feed's venue name must be kept", loc.Name)
	}
}

func TestApplyResolvedLocationKeepsFeedCoordinates(t *testing.T) {
	lat, lon := 43.67, -79.41
	loc := &PlaceInput{Latitude: 10, Longitude: 20}
	applyResolvedLocation(loc, geoloc.Parts{Locality: "Toronto", Latitude: &lat, Longitude: &lon})
	if loc.Latitude != 10 || loc.Longitude != 20 {
		t.Errorf("feed coordinates must win, got %v,%v", loc.Latitude, loc.Longitude)
	}
}
