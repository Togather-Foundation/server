package ical

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/sanitize"
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

// DecomposeOpts controls how a raw location name is mapped into structured
// PlaceInput fields. All fields are optional; DecomposeLocation fills missing
// address components from these defaults.
type DecomposeOpts struct {
	DefaultLocality string
	DefaultRegion   string
	DefaultCountry  string
}

// DecomposeLocation converts a raw location name into a PlaceInput.
// For venue names extracted from ICS descriptions (e.g. "Finch Subway Station"),
// there are typically no structured address components to decompose. The name is
// used as StreetAddress, and any missing locality/region/country fields are filled
// from opts defaults (typically sourced from MapperOptions.DefaultLocation).
// Full address decomposition (e.g. "123 Main St, Toronto, ON M5V 1A1") is handled
// downstream by the geocoding pipeline.
func DecomposeLocation(name string, opts DecomposeOpts) events.PlaceInput {
	pi := events.PlaceInput{
		Name: sanitize.Text(name),
	}

	if name != "" {
		pi.StreetAddress = sanitize.Text(name)
	}

	if opts.DefaultLocality != "" {
		pi.AddressLocality = opts.DefaultLocality
	}
	if opts.DefaultRegion != "" {
		pi.AddressRegion = opts.DefaultRegion
	}
	if opts.DefaultCountry != "" {
		pi.AddressCountry = opts.DefaultCountry
	}

	return pi
}
