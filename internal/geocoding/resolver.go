package geocoding

import (
	"context"
	"errors"
	"strings"

	"github.com/Togather-Foundation/server/internal/geoloc"
	"github.com/rs/zerolog"
)

// ForwardGeocoder is the subset of GeocodingService a LocationResolver needs.
// Declared as an interface so the resolver can be tested without a cache or a
// network, and so a different provider can be substituted later.
type ForwardGeocoder interface {
	Geocode(ctx context.Context, query string, countryCodes string) (*GeocodeResult, error)
}

// LocationResolver implements geoloc.Resolver on top of the cached geocoder.
//
// It is the "ask the world" tier: callers use it only when their offline
// heuristic cannot place a location confidently (typically when the parsed
// result falls outside the node's geographic boundary). Volume is therefore
// small, and the geocoding cache makes repeated venues free.
type LocationResolver struct {
	geocoder     ForwardGeocoder
	countryCodes string
	logger       zerolog.Logger
}

// NewLocationResolver builds a resolver. countryCodes is passed to the geocoder
// (e.g. "ca"); empty means "use the geocoder's default".
func NewLocationResolver(geocoder ForwardGeocoder, countryCodes string, logger zerolog.Logger) *LocationResolver {
	return &LocationResolver{geocoder: geocoder, countryCodes: countryCodes, logger: logger}
}

// Resolve implements geoloc.Resolver. It returns ok=false on any failure or
// empty result so the caller keeps its existing (heuristic) answer.
//
// It makes at most two geocoder calls: the input as given, and — only when the
// first call honestly found nothing (geocoding.ErrNoResults) — the same string
// without its leading venue name. A timeout, transport error, rate limit (429)
// or any other failure stops immediately: retrying a shortened query after a
// provider problem would double the load without improving the odds.
func (r *LocationResolver) Resolve(ctx context.Context, raw string) (geoloc.Parts, bool) {
	if r == nil || r.geocoder == nil {
		return geoloc.Parts{}, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return geoloc.Parts{}, false
	}

	// Free-form Nominatim search often returns nothing for
	// "<venue name>, <street>, <city>, <region>, <country>" — the venue name is
	// not an address component. So if (and only if) the first query reports no
	// results, try once without the leading name (see addressWithoutVenueName).
	query := raw
	result, err := r.geocoder.Geocode(ctx, query, r.countryCodes)
	if err != nil || result == nil {
		if errors.Is(err, ErrNoResults) {
			if stripped, ok := addressWithoutVenueName(raw); ok {
				query = stripped
				result, err = r.geocoder.Geocode(ctx, query, r.countryCodes)
			}
		}
		if err != nil || result == nil {
			if err != nil {
				r.logger.Debug().Err(err).Str("location", raw).Msg("geoloc: resolver could not geocode location")
			}
			return geoloc.Parts{}, false
		}
	}

	parts := geoloc.Parts{
		Name:        query,
		Street:      result.Address.Road,
		Locality:    result.Address.City,
		Region:      result.Address.State,
		PostalCode:  result.Address.Postcode,
		County:      result.Address.County,
		Country:     result.Address.Country,
		CountryCode: result.Address.CountryCode,
	}
	// Fall back to the county only when the city is unknown; a suburb is a
	// neighbourhood ("Kensington Market"), not a locality.
	if parts.Locality == "" {
		parts.Locality = result.Address.County
	}
	if result.Latitude != 0 || result.Longitude != 0 {
		lat, lon := result.Latitude, result.Longitude
		parts.Latitude = &lat
		parts.Longitude = &lon
	}

	if parts.Locality == "" && parts.Region == "" && parts.Country == "" && parts.Latitude == nil {
		return geoloc.Parts{}, false
	}
	return parts, true
}

// addressWithoutVenueName drops the leading venue name from a location string so
// the remainder can be geocoded as an address:
//
//	"Sweat and Tonic, 486 Front St W, Toronto, ON M5V 0V2, Canada"
//	-> "486 Front St W, Toronto, ON M5V 0V2, Canada"
//
// It declines when the leading component looks like a street address itself
// (starts with a digit), or when the remainder is not still an address (no comma
// of its own), so a well-formed address is never shortened into just a city.
func addressWithoutVenueName(raw string) (string, bool) {
	first, rest, found := strings.Cut(raw, ",")
	if !found {
		return "", false
	}
	first = strings.TrimSpace(first)
	rest = strings.TrimSpace(rest)
	if first == "" || rest == "" || !strings.Contains(rest, ",") {
		return "", false
	}
	digits := strings.IndexFunc(first, func(r rune) bool { return r >= '0' && r <= '9' })
	if digits == 0 {
		return "", false
	}
	return rest, true
}
