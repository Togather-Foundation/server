package geocoding

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
)

type stubGeocoder struct {
	result    *GeocodeResult
	err       error
	calls     int
	lastQuery string
}

func (s *stubGeocoder) Geocode(_ context.Context, query string, _ string) (*GeocodeResult, error) {
	s.calls++
	s.lastQuery = query
	return s.result, s.err
}

func TestLocationResolver_Resolve(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("returns structured components", func(t *testing.T) {
		lat, lon := 43.67, -79.41
		geocoder := &stubGeocoder{result: &GeocodeResult{
			Latitude: lat, Longitude: lon,
			Address: AddressComponents{
				Road: "Dupont St", City: "Toronto", County: "Toronto",
				State: "Ontario", Postcode: "M5R 3P3", Country: "Canada", CountryCode: "ca",
			},
		}}

		parts, ok := NewLocationResolver(geocoder, "ca", logger).Resolve(context.Background(), "  Creeds Coffee Bar, Toronto  ")
		if !ok {
			t.Fatal("expected ok")
		}
		if parts.Locality != "Toronto" || parts.Region != "Ontario" || parts.Country != "Canada" {
			t.Errorf("components = %+v", parts)
		}
		if parts.PostalCode != "M5R 3P3" || parts.Street != "Dupont St" {
			t.Errorf("address parts = %+v", parts)
		}
		if parts.Latitude == nil || *parts.Latitude != lat {
			t.Errorf("latitude = %v", parts.Latitude)
		}
		if geocoder.lastQuery != "Creeds Coffee Bar, Toronto" {
			t.Errorf("query = %q, want trimmed input", geocoder.lastQuery)
		}
	})

	t.Run("county fills a missing city", func(t *testing.T) {
		geocoder := &stubGeocoder{result: &GeocodeResult{Address: AddressComponents{County: "Peel", State: "Ontario"}}}
		parts, ok := NewLocationResolver(geocoder, "", logger).Resolve(context.Background(), "somewhere")
		if !ok || parts.Locality != "Peel" {
			t.Fatalf("parts = %+v, ok = %v", parts, ok)
		}
	})

	t.Run("coordinates only still resolves", func(t *testing.T) {
		geocoder := &stubGeocoder{result: &GeocodeResult{Latitude: 43.6, Longitude: -79.4}}
		parts, ok := NewLocationResolver(geocoder, "", logger).Resolve(context.Background(), "somewhere")
		if !ok || parts.Latitude == nil {
			t.Fatalf("parts = %+v, ok = %v", parts, ok)
		}
	})

	t.Run("failure keeps the caller's answer", func(t *testing.T) {
		geocoder := &stubGeocoder{err: errors.New("timeout")}
		if _, ok := NewLocationResolver(geocoder, "", logger).Resolve(context.Background(), "somewhere"); ok {
			t.Fatal("expected ok=false on error")
		}
	})

	t.Run("empty result is not a resolution", func(t *testing.T) {
		geocoder := &stubGeocoder{result: &GeocodeResult{}}
		if _, ok := NewLocationResolver(geocoder, "", logger).Resolve(context.Background(), "somewhere"); ok {
			t.Fatal("expected ok=false for an empty result")
		}
	})

	t.Run("blank input is not looked up", func(t *testing.T) {
		geocoder := &stubGeocoder{}
		resolver := NewLocationResolver(geocoder, "", logger)
		if _, ok := resolver.Resolve(context.Background(), "   "); ok {
			t.Fatal("expected ok=false")
		}
		if geocoder.calls != 0 {
			t.Errorf("geocoder calls = %d, want 0", geocoder.calls)
		}
	})

	t.Run("nil resolver and nil geocoder are safe", func(t *testing.T) {
		var resolver *LocationResolver
		if _, ok := resolver.Resolve(context.Background(), "x"); ok {
			t.Fatal("expected ok=false")
		}
		if _, ok := NewLocationResolver(nil, "", logger).Resolve(context.Background(), "x"); ok {
			t.Fatal("expected ok=false")
		}
	})

	t.Run("address is recovered from a cached raw response", func(t *testing.T) {
		raw := []byte(`{"lat":"43.6","lon":"-79.4","address":{"city":"Toronto","state":"Ontario","country":"Canada","country_code":"ca","postcode":"M5R 3P3"}}`)
		got := addressComponentsFromRaw(raw)
		if got.City != "Toronto" || got.State != "Ontario" || got.CountryCode != "ca" || got.Postcode != "M5R 3P3" {
			t.Errorf("addressComponentsFromRaw = %+v", got)
		}
		if empty := addressComponentsFromRaw([]byte("not json")); empty.City != "" {
			t.Errorf("malformed raw should yield the zero value, got %+v", empty)
		}
	})
}

type queryGeocoder struct {
	byQuery map[string]*GeocodeResult
	calls   []string
}

func (q *queryGeocoder) Geocode(_ context.Context, query string, _ string) (*GeocodeResult, error) {
	q.calls = append(q.calls, query)
	if result, ok := q.byQuery[query]; ok {
		return result, nil
	}
	return nil, ErrNoResults
}

func TestLocationResolver_RetriesWithoutVenueName(t *testing.T) {
	logger := zerolog.Nop()
	toronto := &GeocodeResult{Address: AddressComponents{City: "Toronto", State: "Ontario"}}

	geocoder := &queryGeocoder{byQuery: map[string]*GeocodeResult{
		"486 Front St W, Toronto, ON M5V 0V2, Canada": toronto,
	}}
	parts, ok := NewLocationResolver(geocoder, "ca", logger).
		Resolve(context.Background(), "Sweat and Tonic, 486 Front St W, Toronto, ON M5V 0V2, Canada")
	if !ok || parts.Locality != "Toronto" {
		t.Fatalf("parts = %+v, ok = %v", parts, ok)
	}
	if len(geocoder.calls) != 2 {
		t.Fatalf("calls = %v, want the full string then the address", geocoder.calls)
	}
	if geocoder.calls[1] != "486 Front St W, Toronto, ON M5V 0V2, Canada" {
		t.Errorf("retry query = %q", geocoder.calls[1])
	}
}

func TestLocationResolver_DoesNotShortenAStreetAddress(t *testing.T) {
	geocoder := &queryGeocoder{byQuery: map[string]*GeocodeResult{
		"123 Main St, Toronto, ON": {Address: AddressComponents{City: "Toronto", State: "Ontario"}},
	}}
	if _, ok := NewLocationResolver(geocoder, "", zerolog.Nop()).Resolve(context.Background(), "123 Main St, Toronto, ON"); !ok {
		t.Fatal("expected ok")
	}
	if len(geocoder.calls) != 1 {
		t.Fatalf("calls = %v, a street-first address must not be shortened", geocoder.calls)
	}
}

// funcGeocoder answers each call through fn, recording the queries, so tests
// can script "no results, then success" and count the attempts.
type funcGeocoder struct {
	calls []string
	fn    func(call int, query string) (*GeocodeResult, error)
}

func (g *funcGeocoder) Geocode(_ context.Context, query string, _ string) (*GeocodeResult, error) {
	g.calls = append(g.calls, query)
	return g.fn(len(g.calls), query)
}

func TestLocationResolver_RetriesOnlyOnNoResults(t *testing.T) {
	toronto := &GeocodeResult{Address: AddressComponents{City: "Toronto", State: "Ontario"}}
	geocoder := &funcGeocoder{fn: func(call int, query string) (*GeocodeResult, error) {
		if call == 1 {
			return nil, fmt.Errorf("%w for query: %s", ErrNoResults, query)
		}
		return toronto, nil
	}}

	parts, ok := NewLocationResolver(geocoder, "ca", zerolog.Nop()).
		Resolve(context.Background(), "Sweat and Tonic, 486 Front St W, Toronto, ON M5V 0V2, Canada")
	if !ok || parts.Locality != "Toronto" {
		t.Fatalf("parts = %+v, ok = %v", parts, ok)
	}
	if len(geocoder.calls) != 2 {
		t.Fatalf("calls = %v, want exactly two (full string, then address)", geocoder.calls)
	}
	if geocoder.calls[1] != "486 Front St W, Toronto, ON M5V 0V2, Canada" {
		t.Errorf("retry query = %q", geocoder.calls[1])
	}
	if parts.Name != "486 Front St W, Toronto, ON M5V 0V2, Canada" {
		t.Errorf("parts.Name = %q, want the query that matched", parts.Name)
	}
}

func TestLocationResolver_DoesNotRetryOnProviderFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"timeout", fmt.Errorf("%w: context deadline exceeded", ErrGeocodingFailed)},
		{"transport error", errors.New("connection reset by peer")},
		{"rate limited 429", fmt.Errorf("%w: rate limited (429)", ErrGeocodingFailed)},
		{"context deadline", context.DeadlineExceeded},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			geocoder := &funcGeocoder{fn: func(int, string) (*GeocodeResult, error) {
				return nil, tc.err
			}}

			if _, ok := NewLocationResolver(geocoder, "", zerolog.Nop()).
				Resolve(context.Background(), "Sweat and Tonic, 486 Front St W, Toronto, ON"); ok {
				t.Fatal("expected ok=false")
			}
			if len(geocoder.calls) != 1 {
				t.Fatalf("calls = %v, want exactly one (no shortened retry on %v)", geocoder.calls, tc.err)
			}
		})
	}
}

func TestAddressWithoutVenueName(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"Sweat and Tonic, 486 Front St W, Toronto, ON M5V 0V2, Canada", "486 Front St W, Toronto, ON M5V 0V2, Canada", true},
		{"MOLA CAFE/MARKET, 822 Dundas St W, Toronto, ON M6J 1V3, Canada", "822 Dundas St W, Toronto, ON M6J 1V3, Canada", true},
		{"123 Main St, Toronto, ON", "", false},
		{"Toronto, ON", "", false},
		{"Some Venue", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, ok := addressWithoutVenueName(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Errorf("addressWithoutVenueName(%q) = %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}
