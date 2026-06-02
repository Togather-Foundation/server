package ical

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/sanitize"
	"github.com/bojanz/address"
)

var (
	defaultLocationPatternsOnce sync.Once
	defaultLocationPatterns     []LocationPattern
)

// LocationPattern pairs a named regex with a human-readable label used for debugging
// and error reporting during location extraction.
type LocationPattern struct {
	Name string
	Re   *regexp.Regexp
}

func DefaultLocationPatterns() []LocationPattern {
	defaultLocationPatternsOnce.Do(func() {
		defaultLocationPatterns = []LocationPattern{
			{Name: "meetup-location-label", Re: regexp.MustCompile(`(?i)meetup\s+location\s*:\s*([^\n\r]+)`)},
			{Name: "meetup-point-label", Re: regexp.MustCompile(`(?i)meet\s*up\s+point\s*:\s*([^\n\r]+)`)},
			{Name: "location-label", Re: regexp.MustCompile(`(?m)(?i)^location\s*:\s*([^\n\r]+)`)},
			{Name: "venue-label", Re: regexp.MustCompile(`(?m)(?i)^venue\s*:\s*([^\n\r]+)`)},
			{Name: "address-label", Re: regexp.MustCompile(`(?m)(?i)^address\s*:\s*([^\n\r]+)`)},
			{Name: "meet-at-near", Re: regexp.MustCompile(`(?i)\bmeet\s+(?:at|near|in front of|outside|inside)\s+([^.\n\r]+)`)},
			{Name: "starting-point", Re: regexp.MustCompile(`(?i)starting\s+point\s*:\s*([^\n\r]+)`)},
			{Name: "start-location", Re: regexp.MustCompile(`(?i)start\s+location\s*:\s*([^\n\r]+)`)},
		}
	})
	return defaultLocationPatterns
}

var virtualSignals = []string{
	"zoom",
	"virtual",
	"online",
	"webinar",
	"livestream",
	"live stream",
	"microsoft teams",
	"google meet",
	"teams meeting",
	"zoom meeting",
}

// IsVirtualDescription returns true if the description contains known virtual event
// signals (zoom, virtual, online, webinar, etc.), indicating the event has no
// physical venue.
func IsVirtualDescription(desc string) bool {
	lower := strings.ToLower(desc)
	for _, signal := range virtualSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}

// ExtractLocationFromDescription scans the description using DefaultLocationPatterns
// and returns the first match. Returns ("", false) if no pattern matches.
func ExtractLocationFromDescription(desc string) (string, bool) {
	return ExtractLocationWithPatterns(desc, DefaultLocationPatterns())
}

// ExtractLocationWithPatterns scans the description using the provided patterns
// (in order) and returns the first match. Returns ("", false) if no pattern matches.
func ExtractLocationWithPatterns(desc string, patterns []LocationPattern) (string, bool) {
	for _, p := range patterns {
		match := p.Re.FindStringSubmatch(desc)
		if len(match) > 1 {
			extracted := strings.TrimSpace(match[1])
			extracted = sanitize.Text(extracted)
			if extracted != "" {
				return extracted, true
			}
		}
	}
	return "", false
}

// DecomposeOpts controls how a raw location name is decomposed into structured
// PlaceInput fields. All fields are optional; DecomposeLocation falls back to
// defaults for any missing address components.
type DecomposeOpts struct {
	CountryCode     string
	DefaultLocality string
	DefaultRegion   string
	DefaultCountry  string
}

// DecomposeLocation converts a raw location name string into a PlaceInput with
// structured address components using opts defaults to fill missing fields.
// StreetAddress is set to the name when no structured decomposition is available.
func DecomposeLocation(name string, opts DecomposeOpts) events.PlaceInput {
	addr := address.Address{
		Line1: name,
	}

	pi := events.PlaceInput{
		Name: sanitize.Text(name),
	}

	streetAddr := addr.Line1
	if addr.Line2 != "" {
		streetAddr += ", " + addr.Line2
	}
	if addr.Line3 != "" {
		streetAddr += ", " + addr.Line3
	}
	if streetAddr != "" {
		pi.StreetAddress = sanitize.Text(streetAddr)
	}

	if addr.Locality != "" {
		pi.AddressLocality = sanitize.Text(addr.Locality)
	} else if opts.DefaultLocality != "" {
		pi.AddressLocality = opts.DefaultLocality
	}

	if addr.Region != "" {
		pi.AddressRegion = sanitize.Text(addr.Region)
	} else if opts.DefaultRegion != "" {
		pi.AddressRegion = opts.DefaultRegion
	}

	if addr.PostalCode != "" {
		pi.PostalCode = sanitize.Text(addr.PostalCode)
	}

	if addr.CountryCode != "" && address.CheckCountryCode(addr.CountryCode) {
		pi.AddressCountry = addr.CountryCode
	} else if opts.DefaultCountry != "" {
		pi.AddressCountry = opts.DefaultCountry
	}

	return pi
}
