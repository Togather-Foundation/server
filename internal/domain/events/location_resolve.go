package events

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/Togather-Foundation/server/internal/geoloc"
	"github.com/rs/zerolog"
)

// locationCoordinateConflictKm is the distance beyond which a resolution that
// supplies coordinates disagrees with coordinates the extractor already had.
// 50 km is deliberately loose: it tolerates a genuine venue correction within a
// metropolitan area (e.g. a suburb placed at the city centre) while separating
// two different cities (Vancouver and Toronto are ~3,300 km apart).
const locationCoordinateConflictKm = 50

// earthRadiusKm is the mean Earth radius used by the great-circle distance.
const earthRadiusKm = 6371

// resolveLocationOnBoundaryFailure is the "ask the world" tier of location
// handling. The offline parser (ICS comma splitting, JSON-LD fields) is cheap
// and covers the common shapes, but it cannot tell whether a string like
// "Toronto, ON M5R 3P3, Canada" means Toronto — and getting it wrong makes
// CheckGeographicBoundary reject a perfectly good event.
//
// Before rejecting, ask the configured resolver (the cached geocoder) to
// re-derive the address. The query is built from the strongest evidence the
// extractor supplied (see locationResolutionQuery), and the answer is only
// accepted when it does not contradict evidence already in hand (see
// locationEvidenceConflict). If it places the event inside the boundary, apply
// the resolved components and coordinates together and accept it. Everything
// else — no resolver, resolver failure, a contradicted answer, still outside,
// no location to resolve — keeps the original decision.
//
// It mutates input.Location only when the resolved address passes the boundary,
// so a failed attempt cannot leave partially-applied data behind.
func (s *IngestService) resolveLocationOnBoundaryFailure(ctx context.Context, input *EventInput) bool {
	if s.locationResolver == nil || input == nil || input.Location == nil {
		return false
	}

	query, queryKind, ok := locationResolutionQuery(input.Location)
	if !ok {
		return false
	}

	// A batch may carry a budget. Allow reserves one lookup when it returns
	// true, so concurrent batches never exceed their cap. When it is
	// exhausted, keep the offline decision instead of calling the geocoder.
	budget := geoloc.BudgetFromContext(ctx)
	if budget != nil && !budget.Allow() {
		budget.RecordSkipped()
		s.logger.Debug().
			Str("query", query).
			Str("query_kind", queryKind).
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

	// Exactly one resolver attempt per event: the resolver itself may retry
	// once on an honest no-result, but this path never issues a second,
	// different query. "Name-only last" in locationResolutionQuery means the
	// name-only query is chosen only when no stronger evidence exists, not that
	// weaker queries are tried after a stronger one fails.
	parts, resolved := s.locationResolver.Resolve(resolveCtx, query)
	if !resolved {
		budget.RecordUnresolved()
		s.logBoundaryResolution(zerolog.DebugLevel, query, queryKind, "rejected", "", "no_result", parts)
		return false
	}

	if conflict := locationEvidenceConflict(input.Location, parts, s.locationConflictKm); conflict != "" {
		// A resolver answer that flatly contradicts the supplied address is a
		// data conflict, not a correction: a name-only match must never
		// override a country, region, locality or coordinate pair that already
		// says something else. Keep the offline decision. Routing to review
		// instead would trade one data bug for another — this path only runs
		// for events already outside the boundary, and a review item carrying a
		// contradicted address would still be wrong if approved.
		budget.RecordUnresolved()
		s.logBoundaryResolution(zerolog.WarnLevel, query, queryKind, "rejected", conflict, "conflict", parts)
		return false
	}

	// Work on a copy so a rejected resolution does not partially rewrite the
	// event's address (the caller still returns the original error).
	candidate := *input.Location
	if applied, reason := applyResolvedLocation(&candidate, parts); !applied {
		// The resolution cannot be applied consistently: it carries
		// administrative components but no coordinates while the supplied
		// place has coordinates, or it supplies coordinates but omits a
		// component the supplied place already has. Either way, applying it
		// would leave data from two different places, so apply neither and keep
		// the offline decision.
		budget.RecordUnresolved()
		s.logBoundaryResolution(zerolog.DebugLevel, query, queryKind, "rejected", "", reason, parts)
		return false
	}

	trial := *input
	trial.Location = &candidate
	if err := CheckGeographicBoundary(trial, s.geoBoundaryConfig); err != nil {
		// The geocoder answered but still could not place the event inside the
		// boundary; that is an unresolved lookup, not a skipped one.
		budget.RecordUnresolved()
		s.logBoundaryResolution(zerolog.DebugLevel, query, queryKind, "rejected", "", "outside_boundary", parts)
		return false
	}

	input.Location = &candidate
	budget.RecordResolved()
	s.logBoundaryResolution(zerolog.InfoLevel, query, queryKind, "accepted", "", "", parts)
	return true
}

// locationResolutionQuery builds the single geocoder query for a place from the
// strongest evidence the extractor supplied:
//
//  1. structured_address — the street address plus any other components;
//  2. venue_and_region — the venue name plus locality/region/country;
//  3. name_only — the venue name alone, used only when nothing else exists.
//
// The weaker tiers are only used when the stronger evidence is missing: this
// path makes exactly one resolver attempt per event, so it must not fall back
// from one query to another. Components already present in the accumulated
// query are skipped so the ICS shape (Name/StreetAddress hold the whole feed
// string while a misparsed locality/region is also set) does not produce a
// franken-address such as "Venue, 450 Dupont St, Toronto, ON M5R 3P3, Canada,
// ON M5R 3P3".
func locationResolutionQuery(loc *PlaceInput) (query string, kind string, ok bool) {
	if loc == nil {
		return "", "", false
	}

	if street := strings.TrimSpace(loc.StreetAddress); street != "" {
		components := []string{
			street,
			loc.AddressLocality,
			loc.AddressRegion,
			loc.PostalCode,
			loc.AddressCountry,
		}
		built := joinAddressComponents(components)
		return built, "structured_address", built != ""
	}

	if name := strings.TrimSpace(loc.Name); name != "" {
		components := []string{
			name,
			loc.AddressLocality,
			loc.AddressRegion,
			loc.AddressCountry,
		}
		acc := name
		appended := false
		for _, component := range components[1:] {
			component = strings.TrimSpace(component)
			if component == "" {
				continue
			}
			if addressComponentPresent(acc, component) {
				continue
			}
			acc += ", " + component
			appended = true
		}
		if appended {
			return acc, "venue_and_region", true
		}
		return acc, "name_only", true
	}

	return "", "", false
}

// joinAddressComponents joins the non-empty components in order, skipping any
// component already contained in the accumulated string (case-insensitively
// and ignoring punctuation).
func joinAddressComponents(components []string) string {
	acc := ""
	for _, component := range components {
		component = strings.TrimSpace(component)
		if component == "" {
			continue
		}
		if acc == "" {
			acc = component
			continue
		}
		if addressComponentPresent(acc, component) {
			continue
		}
		acc += ", " + component
	}
	return acc
}

// addressComponentPresent reports whether component already appears in the
// accumulated address. Comparison is case-insensitive and punctuation is a
// separator, so a multi-word component matches when its token sequence occurs
// contiguously. Token matching (rather than a bare substring test) keeps "ON"
// from being considered present because "Toronto" contains those letters.
func addressComponentPresent(acc, component string) bool {
	if tokenSequencePresent(acc, component) {
		return true
	}
	// A country code and its full name are the same component: skip "CA" when
	// the accumulated address already contains "Canada" (and vice versa).
	code := countryValueCode(component)
	if code == "" {
		return false
	}
	if tokenSequencePresent(acc, code) {
		return true
	}
	for name, candidate := range countryCodeForName {
		if candidate == code && tokenSequencePresent(acc, name) {
			return true
		}
	}
	return false
}

// tokenSequencePresent reports whether component's letter/digit tokens occur
// contiguously in acc.
func tokenSequencePresent(acc, component string) bool {
	tokens := addressTokens(component)
	if len(tokens) == 0 {
		return true
	}
	haystack := addressTokens(acc)
	for i := 0; i+len(tokens) <= len(haystack); i++ {
		match := true
		for j := range tokens {
			if haystack[i+j] != tokens[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// addressTokens lowercases s and splits it into letter/digit tokens; every other
// rune is a separator.
func addressTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// locationEvidenceConflict compares a resolver result against the evidence the
// extractor already supplied and returns a short description of the first
// disagreement, or "" when the answer is compatible. Only evidence that is
// trustworthy is compared:
//
//   - country: only when both sides name one;
//   - region: only when both sides name one, and the supplied region is not
//     really the country (the ICS misparse leaves region="Canada" country="CA");
//   - locality: only when the supplied locality looks like a locality (the ICS
//     misparse leaves locality="ON M5R 3P3", which contains digits);
//   - coordinates: only when both sides have a non-zero pair, using a 50 km
//     threshold so a real venue correction is not treated as a contradiction.
func locationEvidenceConflict(supplied *PlaceInput, parts geoloc.Parts, conflictKm float64) string {
	if supplied == nil {
		return ""
	}
	if conflictKm <= 0 {
		conflictKm = locationCoordinateConflictKm
	}

	if country := strings.TrimSpace(supplied.AddressCountry); country != "" {
		if !countryValuesMatch(country, parts.CountryCode, parts.Country) {
			resolved := strings.TrimSpace(parts.Country)
			if code := strings.ToUpper(strings.TrimSpace(parts.CountryCode)); code != "" {
				resolved = code
			}
			return fmt.Sprintf("country: supplied %s, resolved %s", country, resolved)
		}
	}

	if region := strings.TrimSpace(supplied.AddressRegion); region != "" && strings.TrimSpace(parts.Region) != "" {
		// The ICS mapper can leave the country in addressRegion while
		// addressCountry holds its code. That value is not region evidence and
		// must be ignored here: without this exception the fallback would
		// reject the very events it exists to repair.
		if !suppliedRegionIsCountry(region, supplied.AddressCountry) &&
			normalizeRegion(region) != normalizeRegion(parts.Region) {
			return fmt.Sprintf("region: supplied %s, resolved %s", region, parts.Region)
		}
	}

	if locality := strings.TrimSpace(supplied.AddressLocality); locality != "" && looksLikeLocality(locality) {
		if resolved := strings.TrimSpace(parts.Locality); resolved != "" &&
			normalizeLocationName(locality) != normalizeLocationName(resolved) {
			return fmt.Sprintf("locality: supplied %s, resolved %s", locality, resolved)
		}
	}

	if supplied.Latitude != 0 && supplied.Longitude != 0 && parts.Latitude != nil && parts.Longitude != nil {
		distance := haversineKm(supplied.Latitude, supplied.Longitude, *parts.Latitude, *parts.Longitude)
		if distance > conflictKm {
			return fmt.Sprintf("coordinates: supplied %g,%g, resolved %g,%g",
				supplied.Latitude, supplied.Longitude, *parts.Latitude, *parts.Longitude)
		}
	}

	return ""
}

// looksLikeLocality reports whether s can be trusted as a locality name. The
// only disqualifier is a digit: postal codes and the ICS misparse
// ("ON M5R 3P3") contain them, while real localities may legitimately contain
// periods, apostrophes, ampersands and dashes ("St. Catharines",
// "St. John's", "Washington, D.C."). A value with no letters is not a name.
func looksLikeLocality(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.IndexFunc(s, unicode.IsDigit) >= 0 {
		return false
	}
	return strings.IndexFunc(s, unicode.IsLetter) >= 0
}

// suppliedRegionIsCountry reports whether the region field actually names a
// country. AddressCountry may be a code ("CA") or a name ("Canada"), and the
// ICS parser can leave the country's full name in addressRegion. When no
// country was supplied at all, a recognised country name in the region field is
// still not region evidence — a JSON-LD event can carry region="Canada" with no
// addressCountry.
func suppliedRegionIsCountry(region, country string) bool {
	region = strings.TrimSpace(region)
	country = strings.TrimSpace(country)
	if region == "" {
		return false
	}
	if country != "" {
		if strings.EqualFold(region, country) {
			return true
		}
		regionCode := countryNameCode(region)
		countryCode := countryValueCode(country)
		return regionCode != "" && regionCode == countryCode
	}
	return countryNameCode(region) != ""
}

// countryValuesMatch reports whether the supplied country and the resolved
// country agree. An unrecognised supplied value is never evidence of a
// conflict: this comparison is defence-in-depth, so unknown must not mean
// wrong. When the resolver supplies no country at all, there is nothing to
// contradict.
func countryValuesMatch(supplied, resolvedCode, resolvedName string) bool {
	supplied = strings.TrimSpace(supplied)
	if supplied == "" {
		return true
	}
	suppliedCode := countryValueCode(supplied)
	if suppliedCode == "" {
		return true
	}

	resolvedCode = strings.ToUpper(strings.TrimSpace(resolvedCode))
	resolvedName = strings.TrimSpace(resolvedName)
	if resolvedCode == "" && resolvedName == "" {
		return true
	}
	resolved := resolvedCode
	if resolved == "" {
		resolved = countryValueCode(resolvedName)
	}
	if resolved == "" {
		return strings.EqualFold(supplied, resolvedName)
	}
	return suppliedCode == resolved
}

// countryNameCode returns the ISO 3166-1 alpha-2 code for a recognised country
// *name* (or alias). Unlike countryValueCode it does not treat an arbitrary
// two-letter value as a country code, which is what lets the region check tell
// "Canada" (a country misplaced into the region field) from "BC" (a province).
func countryNameCode(value string) string {
	return countryCodeForName[strings.ToUpper(strings.TrimSpace(value))]
}

// countryValueCode returns the ISO 3166-1 alpha-2 code a country value names, or
// "" when it is not recognised. Bare two-letter values are codes; the name map
// also carries the common alpha-3 and colloquial forms.
func countryValueCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if code, ok := countryCodeForName[strings.ToUpper(value)]; ok {
		return code
	}
	if len(value) == 2 && isAlphaString(value) {
		return strings.ToUpper(value)
	}
	return ""
}

func isAlphaString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// haversineKm returns the great-circle distance between two points in
// kilometres.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const toRadians = math.Pi / 180
	dLat := (lat2 - lat1) * toRadians
	dLon := (lon2 - lon1) * toRadians
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*toRadians)*math.Cos(lat2*toRadians)*math.Sin(dLon/2)*math.Sin(dLon/2)
	if a > 1 {
		// Floating-point overshoot for near-antipodal points would make Asin
		// return NaN, and a NaN comparison silently misses the conflict.
		a = 1
	}
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
}

// countryCodeForName maps country names, aliases and alpha-3 codes to ISO
// 3166-1 alpha-2 codes. It mirrors the set the ICS mapper recognises as trailing
// country components (`internal/ical`), plus the alpha-3 forms feeds commonly
// put in `addressCountry`; it is used only to treat a code and its name as the
// same evidence — never to invent a country for an address. Keep it aligned
// with the ICS list when that list changes.
var countryCodeForName = map[string]string{
	"CANADA":                   "CA",
	"CAN":                      "CA",
	"UNITED STATES":            "US",
	"UNITED STATES OF AMERICA": "US",
	"USA":                      "US",
	"UNITED KINGDOM":           "GB",
	"GREAT BRITAIN":            "GB",
	"GBR":                      "GB",
	"UK":                       "GB",
	"ENGLAND":                  "GB",
	"SCOTLAND":                 "GB",
	"WALES":                    "GB",
	"IRELAND":                  "IE",
	"IRL":                      "IE",
	"FRANCE":                   "FR",
	"FRA":                      "FR",
	"GERMANY":                  "DE",
	"DEU":                      "DE",
	"MEXICO":                   "MX",
	"MEX":                      "MX",
	"AUSTRALIA":                "AU",
	"AUS":                      "AU",
	"NEW ZEALAND":              "NZ",
	"NZL":                      "NZ",
	"INDIA":                    "IN",
	"IND":                      "IN",
	"CHINA":                    "CN",
	"CHN":                      "CN",
	"JAPAN":                    "JP",
	"JPN":                      "JP",
	"BRAZIL":                   "BR",
	"BRA":                      "BR",
	"NETHERLANDS":              "NL",
	"NLD":                      "NL",
	"SPAIN":                    "ES",
	"ESP":                      "ES",
	"ITALY":                    "IT",
	"ITA":                      "IT",
	"PORTUGAL":                 "PT",
	"PRT":                      "PT",
}

// logBoundaryResolution emits exactly one structured line for an attempt:
// the query used, the decision, the conflict/reason when there is one, and the
// resolved components.
func (s *IngestService) logBoundaryResolution(
	level zerolog.Level,
	query, queryKind, decision, conflict, reason string,
	parts geoloc.Parts,
) {
	event := s.logger.WithLevel(level).
		Str("query", query).
		Str("query_kind", queryKind).
		Str("decision", decision)
	if conflict != "" {
		event = event.Str("conflict", conflict)
	}
	if reason != "" {
		event = event.Str("reason", reason)
	}
	if parts.Locality != "" {
		event = event.Str("resolved_locality", parts.Locality)
	}
	if parts.Region != "" {
		event = event.Str("resolved_region", parts.Region)
	}
	if parts.Country != "" {
		event = event.Str("resolved_country", parts.Country)
	}
	if parts.Latitude != nil && parts.Longitude != nil {
		event = event.
			Float64("resolved_latitude", *parts.Latitude).
			Float64("resolved_longitude", *parts.Longitude)
	}
	event.Msg("boundary location resolution")
}

// LocationResolverEnabled reports whether the boundary geocoder fallback is
// wired. Callers use it to decide whether per-batch budgeting and reporting is
// needed at all; when false, behaviour is exactly the pre-fallback path.
func (s *IngestService) LocationResolverEnabled() bool {
	return s != nil && s.locationResolver != nil
}

// applyResolvedLocation copies resolved components onto a PlaceInput and reports
// whether the resolution could be applied consistently (plus a machine-readable
// reason when it could not). Resolved locality/region/country replace the
// offline parse because that parse is exactly what failed. Everything else only
// fills gaps.
//
// Administrative components and coordinates must always describe the same
// place. A resolution is therefore refused when applying it would pair new
// coordinates with a stale component from the supplied place:
//   - it supplies no coordinate pair while the supplied place has one;
//   - it supplies coordinates but omits the locality or region the supplied
//     place already carries (a POI result often has no city).
//
// In both cases nothing is applied, so the caller keeps the offline decision.
func applyResolvedLocation(loc *PlaceInput, parts geoloc.Parts) (bool, string) {
	if loc == nil {
		return false, "coordinates_unavailable"
	}

	hasCoordinates := parts.Latitude != nil && parts.Longitude != nil
	if !hasCoordinates && (loc.Latitude != 0 || loc.Longitude != 0) {
		return false, "coordinates_unavailable"
	}
	if hasCoordinates {
		if strings.TrimSpace(loc.AddressLocality) != "" && strings.TrimSpace(parts.Locality) == "" {
			return false, "components_unavailable"
		}
		if strings.TrimSpace(loc.AddressRegion) != "" && strings.TrimSpace(parts.Region) == "" {
			return false, "components_unavailable"
		}
	}

	if parts.Locality != "" {
		loc.AddressLocality = parts.Locality
	}
	if parts.Region != "" {
		loc.AddressRegion = parts.Region
	}
	if parts.CountryCode != "" {
		// Prefer the ISO code: stored addresses use it, and the resolver
		// supplies both when it knows the country.
		loc.AddressCountry = strings.ToUpper(parts.CountryCode)
	} else if parts.Country != "" {
		loc.AddressCountry = parts.Country
	}
	if parts.PostalCode != "" {
		loc.PostalCode = parts.PostalCode
	}
	if parts.Street != "" && strings.TrimSpace(loc.StreetAddress) == "" {
		loc.StreetAddress = parts.Street
	}
	if hasCoordinates {
		loc.Latitude = *parts.Latitude
		loc.Longitude = *parts.Longitude
	}
	return true, ""
}
