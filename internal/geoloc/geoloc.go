// Package geoloc turns free-text locations into structured address parts.
//
// It defines only the contract; implementations live with the data source that
// provides them (e.g. internal/geocoding for the cached Nominatim resolver, and
// the ICS mapper's built-in heuristic as the offline fallback). Keeping the
// contract here lets every extractor and every validator share one type without
// importing each other.
package geoloc

import "context"

// Parts is a structured location resolved from free text. Zero values mean
// "not known"; callers keep whatever they already had for missing fields.
type Parts struct {
	// Name is the venue or place name when the resolver can separate it from
	// the address. Callers should prefer their own name when present.
	Name       string
	Street     string
	Locality   string
	Region     string
	PostalCode string
	County     string
	Country    string
	// CountryCode is an ISO 3166-1 alpha-2 code when known.
	CountryCode string
	Latitude    *float64
	Longitude   *float64
}

// Resolver resolves a free-text location. ok=false means "could not resolve",
// and callers must preserve their existing result rather than guessing.
//
// Implementations may be offline (heuristics) or backed by a geocoder. They
// must be safe for concurrent use and should be cheap on repeat inputs.
type Resolver interface {
	Resolve(ctx context.Context, raw string) (Parts, bool)
}
