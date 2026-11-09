package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/Togather-Foundation/server/internal/geoloc"
	"github.com/rs/zerolog"
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

func float64Ptr(v float64) *float64 { return &v }

// torontoCoordinates are the coordinates of Toronto City Hall.
var (
	torontoLat   = 43.6535
	torontoLon   = -79.3841
	vancouverLat = 49.2827
	vancouverLon = -123.1207
	oakvilleLat  = 43.4675
	oakvilleLon  = -79.6877
)

func torontoResolvedParts() geoloc.Parts {
	return geoloc.Parts{
		Locality:    "Toronto",
		Region:      "Ontario",
		Country:     "Canada",
		CountryCode: "ca",
		PostalCode:  "M5R 3P3",
		Latitude:    float64Ptr(torontoLat),
		Longitude:   float64Ptr(torontoLon),
	}
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

// vancouverCityHallInput is a real out-of-boundary event whose name also exists
// in Toronto. It carries enough evidence that a name-only match must not win.
func vancouverCityHallInput() EventInput {
	input := completeEventInput("City Hall")
	input.Location = &PlaceInput{
		Name:            "City Hall",
		AddressLocality: "Vancouver",
		AddressRegion:   "BC",
		AddressCountry:  "CA",
		Latitude:        vancouverLat,
		Longitude:       vancouverLon,
	}
	return input
}

// newTestServiceWithLog creates an ingest service whose logger writes JSON to
// buf, so tests can assert the structured boundary-resolution decision line.
func newTestServiceWithLog(repo *MockRepository, buf *bytes.Buffer) *IngestService {
	logger := zerolog.New(buf).Level(zerolog.DebugLevel)
	return NewIngestService(repo, "https://test.com", "America/Toronto", defaultValidationConfig(), logger).
		WithDedupConfig(defaultDedupConfig())
}

// boundaryResolutionLogLine returns the single structured decision line emitted
// by resolveLocationOnBoundaryFailure.
func boundaryResolutionLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var found map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["message"] == "boundary location resolution" {
			if found != nil {
				t.Fatalf("expected exactly one boundary location resolution line, got more:\n%s", buf.String())
			}
			found = entry
		}
	}
	if found == nil {
		t.Fatalf("no boundary location resolution line in log:\n%s", buf.String())
	}
	return found
}

// assertStoredPlace checks the place ingest actually asked the repository to
// store: locality, region, country and coordinates together.
func assertStoredPlace(t *testing.T, repo *MockRepository, placeULID string,
	wantLocality, wantRegion, wantCountry string, wantLat, wantLon float64,
) {
	t.Helper()
	if placeULID == "" {
		t.Fatal("result.PlaceULID is empty; no place was stored")
	}
	params, ok := repo.PlaceParams(placeULID)
	if !ok {
		t.Fatalf("no stored place parameters for ULID %q", placeULID)
	}
	if params.AddressLocality != wantLocality {
		t.Errorf("stored locality = %q, want %q", params.AddressLocality, wantLocality)
	}
	if params.AddressRegion != wantRegion {
		t.Errorf("stored region = %q, want %q", params.AddressRegion, wantRegion)
	}
	if params.AddressCountry != wantCountry {
		t.Errorf("stored country = %q, want %q", params.AddressCountry, wantCountry)
	}
	gotLat, gotLon := 0.0, 0.0
	if params.Latitude != nil {
		gotLat = *params.Latitude
	}
	if params.Longitude != nil {
		gotLon = *params.Longitude
	}
	if gotLat != wantLat || gotLon != wantLon {
		t.Errorf("stored coordinates = %v,%v, want %v,%v", gotLat, gotLon, wantLat, wantLon)
	}
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
	resolver := &stubLocationResolver{ok: true, parts: torontoResolvedParts()}
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
		ok: true,
		parts: geoloc.Parts{
			Locality:  "Oakville",
			Region:    "Ontario",
			Country:   "Canada",
			Latitude:  float64Ptr(oakvilleLat),
			Longitude: float64Ptr(oakvilleLon),
		},
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

// TestIngest_CityHallNameMatchDoesNotOverrideVancouverEvidence is defect 1 and 3
// from the ticket: a resolver that matches only the venue name must not override
// a location that is correctly outside the boundary.
func TestIngest_CityHallNameMatchDoesNotOverrideVancouverEvidence(t *testing.T) {
	repo := NewMockRepository()
	var buf bytes.Buffer
	resolver := &stubLocationResolver{
		ok: true,
		parts: geoloc.Parts{
			Locality:    "Toronto",
			Region:      "Ontario",
			Country:     "Canada",
			CountryCode: "ca",
			Latitude:    float64Ptr(torontoLat),
			Longitude:   float64Ptr(torontoLon),
		},
	}
	service := newTestServiceWithLog(repo, &buf).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	input := vancouverCityHallInput()
	_, err := service.Ingest(context.Background(), input)

	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1", resolver.calls)
	}
	if want := "City Hall, Vancouver, BC, CA"; resolver.lastRaw != want {
		t.Errorf("resolver query = %q, want %q", resolver.lastRaw, want)
	}
	if len(repo.places) != 0 {
		t.Errorf("stored %d place(s), want none for a rejected location", len(repo.places))
	}
	// The supplied location must be untouched: a rejected resolution may not
	// partially rewrite the event's address.
	if input.Location.AddressLocality != "Vancouver" ||
		input.Location.AddressRegion != "BC" ||
		input.Location.Latitude != vancouverLat ||
		input.Location.Longitude != vancouverLon {
		t.Errorf("input location was mutated: %+v", input.Location)
	}

	line := boundaryResolutionLogLine(t, &buf)
	if line["decision"] != "rejected" {
		t.Errorf("decision = %v, want rejected", line["decision"])
	}
	if line["reason"] != "conflict" {
		t.Errorf("reason = %v, want conflict", line["reason"])
	}
	conflict, _ := line["conflict"].(string)
	if !strings.Contains(conflict, "region: supplied BC") {
		t.Errorf("conflict = %q, want the region disagreement", conflict)
	}
	if line["query_kind"] != "venue_and_region" {
		t.Errorf("query_kind = %v, want venue_and_region", line["query_kind"])
	}
}

func TestIngest_ConflictingCountryIsRejected(t *testing.T) {
	repo := NewMockRepository()
	var buf bytes.Buffer
	resolver := &stubLocationResolver{ok: true, parts: torontoResolvedParts()}
	service := newTestServiceWithLog(repo, &buf).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	input := completeEventInput("City Hall")
	input.Location = &PlaceInput{
		Name:            "City Hall",
		AddressLocality: "Seattle",
		AddressRegion:   "WA",
		AddressCountry:  "US",
		Latitude:        47.6062,
		Longitude:       -122.3321,
	}
	_, err := service.Ingest(context.Background(), input)
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
	if len(repo.places) != 0 {
		t.Errorf("stored %d place(s), want none for a conflicting country", len(repo.places))
	}

	line := boundaryResolutionLogLine(t, &buf)
	if line["decision"] != "rejected" || line["reason"] != "conflict" {
		t.Errorf("decision/reason = %v/%v, want rejected/conflict", line["decision"], line["reason"])
	}
	conflict, _ := line["conflict"].(string)
	if !strings.Contains(conflict, "country: supplied US, resolved CA") {
		t.Errorf("conflict = %q, want the country disagreement", conflict)
	}
}

// TestIngest_HonestNameMatchOutsideBoundaryIsNotAConflict proves the conflict
// check does not reject honest confirmations: the resolver agrees with every
// supplied component, the event is simply still outside.
func TestIngest_HonestNameMatchOutsideBoundaryIsNotAConflict(t *testing.T) {
	repo := NewMockRepository()
	var buf bytes.Buffer
	resolver := &stubLocationResolver{
		ok: true,
		parts: geoloc.Parts{
			Locality:    "Vancouver",
			Region:      "British Columbia",
			Country:     "Canada",
			CountryCode: "CA",
			Latitude:    float64Ptr(vancouverLat),
			Longitude:   float64Ptr(vancouverLon),
		},
	}
	service := newTestServiceWithLog(repo, &buf).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	_, err := service.Ingest(context.Background(), vancouverCityHallInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}

	line := boundaryResolutionLogLine(t, &buf)
	if line["decision"] != "rejected" {
		t.Errorf("decision = %v, want rejected", line["decision"])
	}
	if line["reason"] != "outside_boundary" {
		t.Errorf("reason = %v, want outside_boundary", line["reason"])
	}
	if _, ok := line["conflict"]; ok {
		t.Errorf("conflict = %v, want none for an honest confirmation", line["conflict"])
	}
}

// TestIngest_MatchingResolutionIsAppliedAsAWhole is the repair case: the
// misparsed ICS address is corrected, and the stored place carries the
// resolver's administrative components and coordinates together.
func TestIngest_MatchingResolutionIsAppliedAsAWhole(t *testing.T) {
	repo := NewMockRepository()
	var buf bytes.Buffer
	resolver := &stubLocationResolver{ok: true, parts: torontoResolvedParts()}
	service := newTestServiceWithLog(repo, &buf).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	result, err := service.Ingest(context.Background(), misparsedInput())
	if err != nil {
		t.Fatalf("Ingest() error = %v, want the resolver to place the event", err)
	}
	if result == nil {
		t.Fatal("expected an ingest result")
	}
	assertStoredPlace(t, repo, result.PlaceULID, "Toronto", "ON", "CA", torontoLat, torontoLon)

	line := boundaryResolutionLogLine(t, &buf)
	if line["decision"] != "accepted" {
		t.Errorf("decision = %v, want accepted", line["decision"])
	}
	if line["query_kind"] != "structured_address" {
		t.Errorf("query_kind = %v, want structured_address", line["query_kind"])
	}
	if want := misparsedInput().Location.StreetAddress; line["query"] != want {
		t.Errorf("query = %v, want %q without the misparsed duplicate components", line["query"], want)
	}
}

// TestIngest_CoordinatesPresentButResolutionWithoutCoordinatesIsRefused covers
// the both-or-neither rule at the ingest boundary: new administrative components
// without coordinates would leave coordinates from another place, so nothing is
// applied and the offline decision stands.
func TestIngest_CoordinatesPresentButResolutionWithoutCoordinatesIsRefused(t *testing.T) {
	repo := NewMockRepository()
	var buf bytes.Buffer
	resolver := &stubLocationResolver{
		ok: true,
		parts: geoloc.Parts{
			Locality: "Toronto",
			Region:   "Ontario",
			Country:  "Canada",
		},
	}
	service := newTestServiceWithLog(repo, &buf).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	input := misparsedInput()
	input.Location.Latitude = 43.67
	input.Location.Longitude = -79.41

	_, err := service.Ingest(context.Background(), input)
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want the offline decision to stand", err)
	}
	if len(repo.places) != 0 {
		t.Errorf("stored %d place(s), want none when the resolution has no coordinates", len(repo.places))
	}

	line := boundaryResolutionLogLine(t, &buf)
	if line["decision"] != "rejected" || line["reason"] != "coordinates_unavailable" {
		t.Errorf("decision/reason = %v/%v, want rejected/coordinates_unavailable", line["decision"], line["reason"])
	}
}

// TestIngest_ResolutionMissingLocalityIsRefused proves the both-or-neither rule
// also covers a coordinate-bearing resolution that omits a component the event
// already carries: applying it would store the supplied locality beside the
// resolver's coordinates.
func TestIngest_ResolutionMissingLocalityIsRefused(t *testing.T) {
	repo := NewMockRepository()
	var buf bytes.Buffer
	resolver := &stubLocationResolver{
		ok: true,
		parts: geoloc.Parts{
			Region:      "Ontario",
			Country:     "Canada",
			CountryCode: "CA",
			Latitude:    float64Ptr(torontoLat),
			Longitude:   float64Ptr(torontoLon),
		},
	}
	service := newTestServiceWithLog(repo, &buf).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	input := completeEventInput("City Hall")
	input.Location = &PlaceInput{
		Name:            "City Hall",
		AddressLocality: "Mississauga",
		AddressRegion:   "ON",
		AddressCountry:  "CA",
	}
	_, err := service.Ingest(context.Background(), input)
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want the offline decision to stand", err)
	}
	if len(repo.places) != 0 {
		t.Errorf("stored %d place(s), want none when the resolution omits the supplied locality", len(repo.places))
	}
	line := boundaryResolutionLogLine(t, &buf)
	if line["decision"] != "rejected" || line["reason"] != "components_unavailable" {
		t.Errorf("decision/reason = %v/%v, want rejected/components_unavailable", line["decision"], line["reason"])
	}
}

// TestIngest_ConflictCountsAsUnresolved pins the per-batch accounting for a
// conflict: the lookup ran and was rejected, so it is unresolved, not skipped.
func TestIngest_ConflictCountsAsUnresolved(t *testing.T) {
	resolver := &stubLocationResolver{ok: true, parts: torontoResolvedParts()}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	budget := geoloc.NewBudget(5, 0)
	ctx := geoloc.WithBudget(context.Background(), budget)
	if _, err := service.Ingest(ctx, vancouverCityHallInput()); err == nil {
		t.Fatal("Ingest() error = nil, want the contradicted resolution to be rejected")
	}
	if got, want := budget.Stats(), (geoloc.BatchStats{Unresolved: 1}); got != want {
		t.Errorf("budget stats = %+v, want %+v", got, want)
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

func TestIngest_LocationResolverDisabledByDefault(t *testing.T) {
	service := newTestService(NewMockRepository()).WithGeographicBoundaryConfig(torontoBoundary())
	if service.LocationResolverEnabled() {
		t.Error("LocationResolverEnabled() = true, want false without WithLocationResolver")
	}

	_, err := service.Ingest(context.Background(), misparsedInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
}

func TestIngest_BoundaryBudgetExhaustedKeepsOfflineDecision(t *testing.T) {
	resolver := &stubLocationResolver{ok: true, parts: torontoResolvedParts()}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)
	if !service.LocationResolverEnabled() {
		t.Fatal("LocationResolverEnabled() = false, want true")
	}

	budget := geoloc.NewBudget(0, 0)
	ctx := geoloc.WithBudget(context.Background(), budget)

	_, err := service.Ingest(ctx, misparsedInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
	if resolver.calls != 0 {
		t.Errorf("resolver calls = %d, want 0 when the budget is exhausted", resolver.calls)
	}
	if got, want := budget.Stats(), (geoloc.BatchStats{Skipped: 1}); got != want {
		t.Errorf("budget stats = %+v, want %+v", got, want)
	}
}

func TestIngest_BoundaryBudgetCapsLookupsPerBatch(t *testing.T) {
	resolver := &stubLocationResolver{ok: false}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	// One lookup shared by two out-of-boundary events: the first attempt is
	// made (and fails), the second keeps its offline decision without calling
	// the resolver.
	budget := geoloc.NewBudget(1, 0)
	ctx := geoloc.WithBudget(context.Background(), budget)

	for i := 0; i < 2; i++ {
		_, err := service.Ingest(ctx, misparsedInput())
		var boundaryErr *ErrOutsideGeographicBoundary
		if !errors.As(err, &boundaryErr) {
			t.Fatalf("event %d: err = %v, want ErrOutsideGeographicBoundary", i, err)
		}
	}

	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1", resolver.calls)
	}
	if got, want := budget.Stats(), (geoloc.BatchStats{Unresolved: 1, Skipped: 1}); got != want {
		t.Errorf("budget stats = %+v, want %+v", got, want)
	}
}

func TestIngest_BoundaryBudgetRecordsResolved(t *testing.T) {
	resolver := &stubLocationResolver{ok: true, parts: torontoResolvedParts()}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	budget := geoloc.NewBudget(5, 0)
	ctx := geoloc.WithBudget(context.Background(), budget)

	if _, err := service.Ingest(ctx, misparsedInput()); err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if got, want := budget.Stats(), (geoloc.BatchStats{Resolved: 1}); got != want {
		t.Errorf("budget stats = %+v, want %+v", got, want)
	}
}

func TestIngest_BoundaryBudgetRecordsStillOutsideAsUnresolved(t *testing.T) {
	resolver := &stubLocationResolver{
		ok: true,
		parts: geoloc.Parts{
			Locality:  "Oakville",
			Region:    "Ontario",
			Country:   "Canada",
			Latitude:  float64Ptr(oakvilleLat),
			Longitude: float64Ptr(oakvilleLon),
		},
	}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	budget := geoloc.NewBudget(5, 0)
	ctx := geoloc.WithBudget(context.Background(), budget)

	_, err := service.Ingest(ctx, misparsedInput())
	var boundaryErr *ErrOutsideGeographicBoundary
	if !errors.As(err, &boundaryErr) {
		t.Fatalf("err = %v, want ErrOutsideGeographicBoundary", err)
	}
	if got, want := budget.Stats(), (geoloc.BatchStats{Unresolved: 1}); got != want {
		t.Errorf("budget stats = %+v, want %+v", got, want)
	}
}

type deadlineRecordingResolver struct {
	calls       int
	hasDeadline bool
	deadline    time.Time
}

func (r *deadlineRecordingResolver) Resolve(ctx context.Context, _ string) (geoloc.Parts, bool) {
	r.calls++
	r.deadline, r.hasDeadline = ctx.Deadline()
	return geoloc.Parts{}, false
}

func TestIngest_BoundaryBudgetClampsLookupToWindow(t *testing.T) {
	resolver := &deadlineRecordingResolver{}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	budget := geoloc.NewBudget(5, time.Minute)
	ctx := geoloc.WithBudget(context.Background(), budget)

	_, _ = service.Ingest(ctx, misparsedInput())
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if !resolver.hasDeadline {
		t.Error("resolver context has no deadline; an in-flight lookup could outlive the batch window")
	}
	// The deadline must be the budget's own window, not an unrelated one.
	if remaining := time.Until(resolver.deadline); remaining > time.Minute || remaining < 0 {
		t.Errorf("resolver deadline is %v away, want within the 1m budget", remaining)
	}
}

func TestIngest_NoBudgetDoesNotAddADeadline(t *testing.T) {
	resolver := &deadlineRecordingResolver{}
	service := newTestService(NewMockRepository()).
		WithGeographicBoundaryConfig(torontoBoundary()).
		WithLocationResolver(resolver)

	_, _ = service.Ingest(context.Background(), misparsedInput())
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
	if resolver.hasDeadline {
		t.Error("resolver context has a deadline without a budget")
	}
}

func TestApplyResolvedLocation(t *testing.T) {
	loc := &PlaceInput{
		Name:            "Creeds Coffee Bar",
		AddressLocality: "ON M5R 3P3",
		AddressRegion:   "Canada",
		AddressCountry:  "",
	}
	applied, reason := applyResolvedLocation(loc, geoloc.Parts{
		Locality:    "Toronto",
		Region:      "Ontario",
		CountryCode: "ca",
		PostalCode:  "M5R 3P3",
		Street:      "450 Dupont St",
		Latitude:    float64Ptr(torontoLat),
		Longitude:   float64Ptr(torontoLon),
	})
	if !applied {
		t.Fatalf("applyResolvedLocation() = false (%s), want true", reason)
	}

	if loc.AddressLocality != "Toronto" || loc.AddressRegion != "Ontario" {
		t.Errorf("locality/region = %q/%q", loc.AddressLocality, loc.AddressRegion)
	}
	if loc.AddressCountry != "CA" {
		t.Errorf("country = %q, want CA from the country code", loc.AddressCountry)
	}
	if loc.PostalCode != "M5R 3P3" || loc.StreetAddress != "450 Dupont St" {
		t.Errorf("postal/street = %q/%q", loc.PostalCode, loc.StreetAddress)
	}
	if loc.Latitude != torontoLat || loc.Longitude != torontoLon {
		t.Errorf("coords = %v,%v", loc.Latitude, loc.Longitude)
	}
	if loc.Name != "Creeds Coffee Bar" {
		t.Errorf("name = %q, the feed's venue name must be kept", loc.Name)
	}
}

// TestApplyResolvedLocationReplacesFeedCoordinates replaces the old "feed
// coordinates always win" expectation: coordinates and administrative
// components must come from the same source.
func TestApplyResolvedLocationReplacesFeedCoordinates(t *testing.T) {
	loc := &PlaceInput{Latitude: 10, Longitude: 20}
	applied, reason := applyResolvedLocation(loc, geoloc.Parts{
		Locality:  "Toronto",
		Latitude:  float64Ptr(torontoLat),
		Longitude: float64Ptr(torontoLon),
	})
	if !applied {
		t.Fatalf("applyResolvedLocation() = false (%s), want true", reason)
	}
	if loc.Latitude != torontoLat || loc.Longitude != torontoLon {
		t.Errorf("coordinates = %v,%v, want the resolution's %v,%v", loc.Latitude, loc.Longitude, torontoLat, torontoLon)
	}
}

func TestApplyResolvedLocationRefusesComponentsWithoutCoordinates(t *testing.T) {
	loc := &PlaceInput{Latitude: 10, Longitude: 20, AddressLocality: "Vancouver"}
	applied, reason := applyResolvedLocation(loc, geoloc.Parts{Locality: "Toronto"})
	if applied {
		t.Fatal("applyResolvedLocation() = true, want false: components without coordinates would mix places")
	}
	if reason != "coordinates_unavailable" {
		t.Errorf("reason = %q, want coordinates_unavailable", reason)
	}
	if loc.AddressLocality != "Vancouver" || loc.Latitude != 10 || loc.Longitude != 20 {
		t.Errorf("location was partially applied: %+v", loc)
	}
}

func TestApplyResolvedLocationTreatsHalfACoordinatePairAsNoCoordinates(t *testing.T) {
	loc := &PlaceInput{Latitude: 10, Longitude: 20}
	applied, _ := applyResolvedLocation(loc, geoloc.Parts{Locality: "Toronto", Latitude: float64Ptr(torontoLat)})
	if applied {
		t.Fatal("applyResolvedLocation() = true, want false for a partial coordinate pair")
	}
	if loc.AddressLocality == "Toronto" {
		t.Error("locality was applied with an unusable coordinate pair")
	}
}

func TestApplyResolvedLocationAppliesComponentsWhenSuppliedCoordinatesAreZero(t *testing.T) {
	loc := &PlaceInput{AddressLocality: "ON M5R 3P3"}
	applied, reason := applyResolvedLocation(loc, geoloc.Parts{Locality: "Toronto", Region: "Ontario"})
	if !applied {
		t.Fatalf("applyResolvedLocation() = false (%s), want true when neither side has coordinates", reason)
	}
	if loc.AddressLocality != "Toronto" || loc.AddressRegion != "Ontario" {
		t.Errorf("locality/region = %q/%q", loc.AddressLocality, loc.AddressRegion)
	}
}

func TestLocationResolutionQuery(t *testing.T) {
	tests := []struct {
		name      string
		loc       *PlaceInput
		wantQuery string
		wantKind  string
		wantOK    bool
	}{
		{
			name: "ICS franken address is deduplicated",
			loc: &PlaceInput{
				Name:            "Creeds Coffee Bar, 450 Dupont St, Toronto, ON M5R 3P3, Canada",
				StreetAddress:   "Creeds Coffee Bar, 450 Dupont St, Toronto, ON M5R 3P3, Canada",
				AddressLocality: "ON M5R 3P3",
				AddressRegion:   "Canada",
				AddressCountry:  "CA",
			},
			wantQuery: "Creeds Coffee Bar, 450 Dupont St, Toronto, ON M5R 3P3, Canada",
			wantKind:  "structured_address",
			wantOK:    true,
		},
		{
			name: "street address is the strongest evidence",
			loc: &PlaceInput{
				Name:            "Creeds Coffee Bar",
				StreetAddress:   "450 Dupont St",
				AddressLocality: "Toronto",
				AddressRegion:   "ON",
				PostalCode:      "M5R 3P3",
				AddressCountry:  "CA",
			},
			wantQuery: "450 Dupont St, Toronto, ON, M5R 3P3, CA",
			wantKind:  "structured_address",
			wantOK:    true,
		},
		{
			name: "venue plus region when there is no street address",
			loc: &PlaceInput{
				Name:            "City Hall",
				AddressLocality: "Vancouver",
				AddressRegion:   "BC",
				AddressCountry:  "CA",
			},
			wantQuery: "City Hall, Vancouver, BC, CA",
			wantKind:  "venue_and_region",
			wantOK:    true,
		},
		{
			name:      "name alone is the last resort",
			loc:       &PlaceInput{Name: "City Hall"},
			wantQuery: "City Hall",
			wantKind:  "name_only",
			wantOK:    true,
		},
		{
			name: "duplicate locality collapses to name only",
			loc: &PlaceInput{
				Name:            "City Hall, Vancouver",
				AddressLocality: "Vancouver",
			},
			wantQuery: "City Hall, Vancouver",
			wantKind:  "name_only",
			wantOK:    true,
		},
		{
			name:   "nothing to resolve",
			loc:    &PlaceInput{},
			wantOK: false,
		},
		{
			name:   "nil place",
			loc:    nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, kind, ok := locationResolutionQuery(tt.loc)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if query != tt.wantQuery {
				t.Errorf("query = %q, want %q", query, tt.wantQuery)
			}
			if kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}
		})
	}
}

func TestLocationEvidenceConflict(t *testing.T) {
	torontoParts := torontoResolvedParts()

	tests := []struct {
		name       string
		supplied   *PlaceInput
		parts      geoloc.Parts
		wantSubstr string
	}{
		{
			name: "agreement is not a conflict",
			supplied: &PlaceInput{
				AddressLocality: "Toronto",
				AddressRegion:   "ON",
				AddressCountry:  "CA",
				Latitude:        torontoLat,
				Longitude:       torontoLon,
			},
			parts: torontoParts,
		},
		{
			name: "region full name and code agree",
			supplied: &PlaceInput{
				AddressLocality: "Toronto",
				AddressRegion:   "Ontario",
				AddressCountry:  "Canada",
			},
			parts: torontoParts,
		},
		{
			name: "country code names the same country as the resolved name",
			supplied: &PlaceInput{
				AddressCountry: "CA",
			},
			parts: geoloc.Parts{Country: "Canada"},
		},
		{
			name: "conflicting country",
			supplied: &PlaceInput{
				AddressCountry: "US",
			},
			parts:      torontoParts,
			wantSubstr: "country: supplied US, resolved CA",
		},
		{
			name: "region that is really the country is ignored",
			supplied: &PlaceInput{
				AddressRegion:  "Canada",
				AddressCountry: "CA",
			},
			parts: geoloc.Parts{Region: "Ontario", Country: "Canada", CountryCode: "CA"},
		},
		{
			name: "locale with digits is not locality evidence",
			supplied: &PlaceInput{
				AddressLocality: "ON M5R 3P3",
			},
			parts: torontoParts,
		},
		{
			name: "conflicting locality",
			supplied: &PlaceInput{
				AddressLocality: "Vancouver",
			},
			parts:      torontoParts,
			wantSubstr: "locality: supplied Vancouver, resolved Toronto",
		},
		{
			name: "conflicting region",
			supplied: &PlaceInput{
				AddressRegion: "BC",
			},
			parts:      torontoParts,
			wantSubstr: "region: supplied BC, resolved Ontario",
		},
		{
			name: "distant coordinates conflict",
			supplied: &PlaceInput{
				Latitude:  vancouverLat,
				Longitude: vancouverLon,
			},
			parts:      torontoParts,
			wantSubstr: "coordinates: supplied",
		},
		{
			name: "nearby coordinates do not conflict",
			supplied: &PlaceInput{
				// ~20 km north of Toronto City Hall: a plausible venue
				// correction, not a different city.
				Latitude:  torontoLat + 0.18,
				Longitude: torontoLon,
			},
			parts: torontoParts,
		},
		{
			name: "punctuated locality is still evidence",
			supplied: &PlaceInput{
				AddressLocality: "St. Catharines",
			},
			parts:      torontoParts,
			wantSubstr: "locality: supplied St. Catharines, resolved Toronto",
		},
		{
			name: "locality with typographic apostrophe is still evidence",
			supplied: &PlaceInput{
				AddressLocality: "St. John\u2019s",
			},
			parts:      torontoParts,
			wantSubstr: "locality: supplied St. John\u2019s, resolved Toronto",
		},
		{
			name: "country name in the region field is ignored without a country",
			supplied: &PlaceInput{
				AddressRegion: "Canada",
			},
			parts: geoloc.Parts{Region: "Ontario", Country: "Canada", CountryCode: "CA"},
		},
		{
			name: "alpha-3 country code agrees with the resolved code",
			supplied: &PlaceInput{
				AddressCountry: "CAN",
			},
			parts: geoloc.Parts{Country: "Canada", CountryCode: "CA"},
		},
		{
			name: "uk alias agrees with GB",
			supplied: &PlaceInput{
				AddressCountry: "UK",
			},
			parts: geoloc.Parts{Country: "United Kingdom", CountryCode: "GB"},
		},
		{
			name: "unrecognised country is not evidence of a conflict",
			supplied: &PlaceInput{
				AddressCountry: "Côte d'Ivoire",
			},
			parts: geoloc.Parts{Country: "Canada", CountryCode: "CA"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := locationEvidenceConflict(tt.supplied, tt.parts, 0)
			if tt.wantSubstr == "" {
				if got != "" {
					t.Errorf("conflict = %q, want none", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("conflict = %q, want it to contain %q", got, tt.wantSubstr)
			}
		})
	}
}

func TestHaversineKm(t *testing.T) {
	// Vancouver to Toronto is roughly 3,350 km; allow a wide tolerance for the
	// spherical approximation.
	got := haversineKm(vancouverLat, vancouverLon, torontoLat, torontoLon)
	if got < 3000 || got > 3600 {
		t.Errorf("haversineKm(Vancouver, Toronto) = %v, want ~3,300 km", got)
	}
}
