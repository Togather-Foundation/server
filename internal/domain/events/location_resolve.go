package events

import (
	"context"
	"strings"

	"github.com/Togather-Foundation/server/internal/geoloc"
)

// resolveLocationOnBoundaryFailure is the "ask the world" tier of location
// handling. The offline parser (ICS comma splitting, JSON-LD fields) is cheap
// and covers the common shapes, but it cannot tell whether a string like
// "Toronto, ON M5R 3P3, Canada" means Toronto — and getting it wrong makes
// CheckGeographicBoundary reject a perfectly good event.
//
// Before rejecting, ask the configured resolver (the cached geocoder) to
// re-derive the address. If it places the event inside the boundary, apply the
// resolved components and accept it. Everything else — no resolver, resolver
// failure, still outside, no location to resolve — keeps the original decision.
//
// It mutates input.Location only when the resolved address passes the boundary,
// so a failed attempt cannot leave partially-applied data behind.
func (s *IngestService) resolveLocationOnBoundaryFailure(ctx context.Context, input *EventInput) bool {
	if s.locationResolver == nil || input == nil || input.Location == nil {
		return false
	}

	raw := strings.TrimSpace(input.Location.Name)
	if raw == "" {
		raw = strings.TrimSpace(input.Location.StreetAddress)
	}
	if raw == "" {
		return false
	}

	// A batch may carry a budget. Allow reserves one lookup when it returns
	// true, so concurrent batches never exceed their cap. When it is
	// exhausted, keep the offline decision instead of calling the geocoder.
	budget := geoloc.BudgetFromContext(ctx)
	if budget != nil && !budget.Allow() {
		budget.RecordSkipped()
		s.logger.Debug().
			Str("raw_location", raw).
			Msg("location resolution budget exhausted; keeping offline decision")
		return false
	}

	// Clamp the lookup to the budget's remaining window so a request started
	// just before the deadline cannot keep the batch alive past it.
	resolveCtx := ctx
	if budget != nil {
		if deadline, ok := budget.Deadline(); ok {
			var cancel context.CancelFunc
			resolveCtx, cancel = context.WithDeadline(ctx, deadline)
			defer cancel()
		}
	}

	parts, ok := s.locationResolver.Resolve(resolveCtx, raw)
	if !ok {
		budget.RecordUnresolved()
		return false
	}

	// Work on a copy so a rejected resolution does not partially rewrite the
	// event's address (the caller still returns the original error).
	candidate := *input.Location
	applyResolvedLocation(&candidate, parts)

	trial := *input
	trial.Location = &candidate
	if err := CheckGeographicBoundary(trial, s.geoBoundaryConfig); err != nil {
		// The geocoder answered but still could not place the event inside the
		// boundary; that is an unresolved lookup, not a skipped one.
		budget.RecordUnresolved()
		return false
	}

	input.Location = &candidate
	budget.RecordResolved()
	s.logger.Info().
		Str("raw_location", raw).
		Str("resolved_locality", candidate.AddressLocality).
		Str("resolved_region", candidate.AddressRegion).
		Str("resolved_country", candidate.AddressCountry).
		Msg("geographic boundary satisfied by geocoding the location string")
	return true
}

// LocationResolverEnabled reports whether the boundary geocoder fallback is
// wired. Callers use it to decide whether per-batch budgeting and reporting is
// needed at all; when false, behaviour is exactly the pre-fallback path.
func (s *IngestService) LocationResolverEnabled() bool {
	return s != nil && s.locationResolver != nil
}

// applyResolvedLocation copies resolved components onto a PlaceInput. Resolved
// locality/region/country replace the offline parse because that parse is
// exactly what failed. Everything else only fills gaps.
func applyResolvedLocation(loc *PlaceInput, parts geoloc.Parts) {
	if loc == nil {
		return
	}
	if parts.Locality != "" {
		loc.AddressLocality = parts.Locality
	}
	if parts.Region != "" {
		loc.AddressRegion = parts.Region
	}
	if parts.Country != "" {
		loc.AddressCountry = parts.Country
	} else if parts.CountryCode != "" {
		loc.AddressCountry = strings.ToUpper(parts.CountryCode)
	}
	if parts.PostalCode != "" {
		loc.PostalCode = parts.PostalCode
	}
	if parts.Street != "" && strings.TrimSpace(loc.StreetAddress) == "" {
		loc.StreetAddress = parts.Street
	}
	if loc.Latitude == 0 && loc.Longitude == 0 {
		if parts.Latitude != nil {
			loc.Latitude = *parts.Latitude
		}
		if parts.Longitude != nil {
			loc.Longitude = *parts.Longitude
		}
	}
}
