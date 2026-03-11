package events

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

// multiSessionPatterns are compiled once at package level for efficiency.
// They match title patterns indicating a multi-session course or recurring series.
var multiSessionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\(\d+\s+sessions?\)`),  // "(6 sessions)", "(1 session)"
	regexp.MustCompile(`(?i)\(\d+\s+weeks?\)`),     // "(4 weeks)", "(1 week)"
	regexp.MustCompile(`(?i)\(\d+\s+classes?\)`),   // "(8 classes)", "(1 class)"
	regexp.MustCompile(`(?i)\(\d+\s+workshops?\)`), // "(3 workshops)", "(1 workshop)"
	regexp.MustCompile(`(?i)workshop series`),      // "Workshop Series"
	regexp.MustCompile(`(?i)\bcourse\b`),           // "Course" (word boundary avoids "racecourse", "discourse")
	regexp.MustCompile(`(?i)\bweekly\b`),           // "Weekly" (word boundary)
}

// strictHTML is a bluemonday policy that strips all HTML tags.
// It is safe for concurrent use.
var strictHTML = bluemonday.StrictPolicy()

// cleanText strips all HTML tags and decodes HTML entities from s, then
// collapses internal whitespace. This repairs content from CMSes (e.g.
// WordPress) that embed HTML markup in JSON-LD name/description fields.
// Some sources double-encode their HTML (e.g. &lt;p&gt; instead of <p>),
// so we unescape entities first, then sanitize, to catch both forms.
func cleanText(s string) string {
	// First pass: decode entities so &lt;p&gt; becomes <p> before sanitizing.
	s = html.UnescapeString(s)
	// Strip all HTML tags (bluemonday uses a real parser — handles attributes,
	// comments, CDATA, etc.). StrictPolicy produces plain text with no tags.
	s = strictHTML.Sanitize(s)
	// Second pass: decode any entities that were inside tag attributes and
	// are now exposed after stripping (e.g. &amp; left in text nodes).
	s = html.UnescapeString(s)
	// Unescape WordPress/Tribe-Events-style backslash sequences that survive
	// JSON parsing as literal two-character sequences. These arise because
	// WordPress encodes apostrophes as \' and newlines as \n inside JSON-LD
	// <script> tags using double-backslashes (\\' and \\n in the raw HTML),
	// which are valid JSON escape sequences for a literal backslash followed
	// by the next character. After JSON decoding we get the literal pair.
	s = unescapeBackslashSequences(s)
	// Collapse runs of whitespace (newlines, tabs, multiple spaces).
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// unescapeBackslashSequences converts literal two-character backslash sequences
// left by WordPress/Tribe Events JSON-LD encoding into their intended characters:
//   - \' → '  (apostrophe)
//   - \n → newline (collapsed to space by the caller's strings.Fields)
//   - \r → carriage return (collapsed to space)
//   - \t → tab (collapsed to space)
//   - \\ → \ (double-backslash → single backslash)
func unescapeBackslashSequences(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '\'':
				b.WriteByte('\'')
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// normalizeURL fixes common URL issues in external data sources:
// - Adds https:// prefix to URLs starting with "www."
// - Adds https:// to domain-like strings without protocol
// - Strips query parameters and fragments from http/https URLs (per SEL spec)
// - Normalizes social media shorthand (@username patterns)
// - Preserves mailto: and other non-http schemes unchanged
// - Returns empty string if input is empty
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Preserve mailto: and other non-http schemes unchanged (no query stripping)
	if strings.HasPrefix(trimmed, "mailto:") {
		return trimmed
	}

	// Social media shorthand: @username -> don't try to normalize
	// These should be caught by validation if they're in a URL field
	if strings.HasPrefix(trimmed, "@") {
		return trimmed // Let validation handle @mentions in URL fields
	}

	rawURL := trimmed

	// Check if URL has protocol
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		// Starts with www. or looks like a domain -> add https://
		if strings.HasPrefix(trimmed, "www.") {
			rawURL = "https://" + trimmed
		} else if len(trimmed) > 0 && !strings.ContainsAny(trimmed[:1], "/@#") {
			// Looks like a domain without protocol (has dot and doesn't start with special chars)
			if strings.Contains(trimmed, ".") && !strings.Contains(trimmed, " ") {
				rawURL = "https://" + trimmed
			} else {
				// Not a URL-like string, return as-is
				return trimmed
			}
		} else {
			// Not a URL-like string, return as-is
			return trimmed
		}
	}

	// Strip query parameters and fragments from http/https URLs
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, return original (validation will catch invalid URLs)
		return trimmed
	}

	// Clear query string and fragment
	parsed.RawQuery = ""
	parsed.Fragment = ""

	result := parsed.String()

	// Handle edge case: url.Parse preserves trailing ? for empty queries
	// Example: "https://example.com/page?" becomes "https://example.com/page?" instead of "https://example.com/page"
	result = strings.TrimSuffix(result, "?")

	return result
}

// eventSubtypeDomains maps schema.org Event subtypes to SEL event_domain values.
// Only subtypes that map to recognized domains are included.
var eventSubtypeDomains = map[string]string{
	"MusicEvent":      "music",
	"DanceEvent":      "arts",
	"Festival":        "arts",
	"TheaterEvent":    "arts",
	"ScreeningEvent":  "arts",
	"LiteraryEvent":   "arts",
	"EducationEvent":  "education",
	"ExhibitionEvent": "arts",
	"FoodEvent":       "community",
	"SportsEvent":     "sports",
	"SaleEvent":       "community",
	"SocialEvent":     "community",
	"ComedyEvent":     "arts",
	"ChildrensEvent":  "community",
	"BusinessEvent":   "general",
	"VisualArtsEvent": "arts",
	// "Event" is generic — no domain mapping
}

// NormalizeEventInput trims and normalizes values for consistent storage and hashing.
// Also auto-corrects common data quality issues like timezone errors and malformed URLs.
func NormalizeEventInput(input EventInput) EventInput {
	input.Name = cleanText(input.Name)
	input.Description = cleanText(input.Description)
	input.StartDate = strings.TrimSpace(input.StartDate)
	input.EndDate = strings.TrimSpace(input.EndDate)
	input.DoorTime = strings.TrimSpace(input.DoorTime)
	input.Image = normalizeURL(input.Image)
	input.URL = normalizeURL(input.URL)
	input.License = strings.TrimSpace(input.License)

	// Map schema.org Event subtypes to event_domain if not already set
	if input.EventDomain == "" && input.Type != "" {
		if domain, ok := eventSubtypeDomains[input.Type]; ok {
			input.EventDomain = domain
		}
	}

	// Auto-correct timezone errors where endDate appears before startDate
	// This typically happens with midnight-spanning events that were incorrectly
	// converted from local time to UTC
	input = correctEndDateTimezoneError(input)

	input.Keywords = normalizeStringSlice(input.Keywords, true)
	input.InLanguage = normalizeStringSlice(input.InLanguage, true)
	input.SameAs = normalizeStringSlice(input.SameAs, false)

	if input.Location != nil {
		input.Location = normalizePlaceInput(*input.Location)
	}
	if input.VirtualLocation != nil {
		input.VirtualLocation = normalizeVirtualLocationInput(*input.VirtualLocation)
	}
	if input.Organizer != nil {
		input.Organizer = normalizeOrganizationInput(*input.Organizer)
	}
	if input.Offers != nil {
		input.Offers = normalizeOfferInput(*input.Offers)
	}
	if input.Source != nil {
		input.Source = normalizeSourceInput(*input.Source)
	}
	if len(input.Occurrences) > 0 {
		input.Occurrences = normalizeOccurrences(input.Occurrences)
	}

	return input
}

func normalizePlaceInput(place PlaceInput) *PlaceInput {
	place.ID = strings.TrimSpace(place.ID)
	place.Name = strings.TrimSpace(place.Name)
	place.StreetAddress = strings.TrimSpace(place.StreetAddress)
	place.AddressLocality = strings.TrimSpace(place.AddressLocality)
	place.AddressRegion = normalizeRegion(place.AddressRegion)
	place.PostalCode = strings.TrimSpace(place.PostalCode)
	place.AddressCountry = strings.TrimSpace(place.AddressCountry)
	return &place
}

func normalizeVirtualLocationInput(location VirtualLocationInput) *VirtualLocationInput {
	location.Type = strings.TrimSpace(location.Type)
	location.URL = normalizeURL(location.URL)
	location.Name = strings.TrimSpace(location.Name)
	return &location
}

func normalizeOrganizationInput(org OrganizationInput) *OrganizationInput {
	org.ID = strings.TrimSpace(org.ID)
	org.Name = strings.TrimSpace(org.Name)
	org.URL = normalizeURL(org.URL)
	org.Email = strings.TrimSpace(org.Email)
	org.Telephone = strings.TrimSpace(org.Telephone)
	return &org
}

func normalizeOfferInput(offer OfferInput) *OfferInput {
	offer.URL = normalizeURL(offer.URL)
	offer.Price = strings.TrimSpace(offer.Price)
	offer.PriceCurrency = strings.TrimSpace(offer.PriceCurrency)
	return &offer
}

func normalizeSourceInput(source SourceInput) *SourceInput {
	source.URL = normalizeURL(source.URL)
	source.EventID = strings.TrimSpace(source.EventID)
	return &source
}

func normalizeOccurrences(values []OccurrenceInput) []OccurrenceInput {
	result := make([]OccurrenceInput, 0, len(values))
	for _, occ := range values {
		occ.StartDate = strings.TrimSpace(occ.StartDate)
		occ.EndDate = strings.TrimSpace(occ.EndDate)
		occ.DoorTime = strings.TrimSpace(occ.DoorTime)
		occ.Timezone = strings.TrimSpace(occ.Timezone)
		occ.VenueID = strings.TrimSpace(occ.VenueID)
		occ.VirtualURL = normalizeURL(occ.VirtualURL)

		// Apply timezone error correction to occurrence dates (same as top-level)
		occ = correctOccurrenceEndDateTimezoneError(occ)

		result = append(result, occ)
	}
	return result
}

// correctOccurrenceEndDateTimezoneError applies the same timezone correction logic
// as correctEndDateTimezoneError but for individual occurrences.
// Only corrects if end time is in early morning (0-4 AM) and corrected duration < 7h.
func correctOccurrenceEndDateTimezoneError(occ OccurrenceInput) OccurrenceInput {
	if occ.EndDate == "" {
		return occ
	}

	startTime, err := time.Parse(time.RFC3339, occ.StartDate)
	if err != nil {
		return occ // Invalid startDate, let validation handle it
	}

	endTime, err := time.Parse(time.RFC3339, occ.EndDate)
	if err != nil {
		return occ // Invalid endDate, let validation handle it
	}

	// Check if endDate is before startDate (the error condition)
	if !endTime.Before(startTime) {
		return occ // No correction needed
	}

	endHour := endTime.Hour() // 0-23 in UTC

	// Only auto-correct if end time is in early morning (0-4 AM)
	if endHour <= 4 {
		correctedEnd := endTime.Add(24 * time.Hour)

		// Check if the corrected event duration is reasonable (< 7 hours)
		duration := correctedEnd.Sub(startTime)
		if duration > 0 && duration < 7*time.Hour {
			// Apply correction: add 24 hours to endDate
			occ.EndDate = correctedEnd.Format(time.RFC3339)
		}
	}

	return occ
}

func normalizeStringSlice(values []string, lower bool) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// correctEndDateTimezoneError detects and fixes common timezone conversion errors// where endDate appears chronologically before startDate.
//
// This typically occurs with midnight-spanning events that were incorrectly converted
// from local time to UTC. For example:
//   - Event: 11 PM - 2 AM local time (EST/EDT)
//   - Incorrect conversion: 2025-03-31T23:00Z to 2025-03-31T02:00Z
//   - Should be: 2025-03-31T23:00Z to 2025-04-01T02:00Z
//
// Auto-correction is conservative and only applies when:
//   - End time is in early morning (0-4 AM UTC), AND
//   - Corrected duration would be < 7 hours
//
// This filters out bad data while fixing typical overnight events.
// Validation adds warnings with different confidence levels for corrected events.
func correctEndDateTimezoneError(input EventInput) EventInput {
	if input.EndDate == "" {
		return input // No endDate to correct
	}

	startTime, err := time.Parse(time.RFC3339, input.StartDate)
	if err != nil {
		return input // Invalid startDate, let validation handle it
	}

	endTime, err := time.Parse(time.RFC3339, input.EndDate)
	if err != nil {
		return input // Invalid endDate, let validation handle it
	}

	// Check if endDate is before startDate (the error condition)
	if !endTime.Before(startTime) {
		return input // No correction needed, dates are in correct order
	}

	endHour := endTime.Hour() // 0-23 in UTC

	// Only auto-correct if end time is in early morning (0-4 AM)
	// This strongly suggests a legitimate overnight event
	if endHour <= 4 {
		correctedEnd := endTime.Add(24 * time.Hour)

		// Check if the corrected event duration is reasonable (< 7 hours)
		// This filters out bad data while allowing typical overnight events
		duration := correctedEnd.Sub(startTime)
		if duration > 0 && duration < 7*time.Hour {
			// Apply correction: add 24 hours to endDate
			input.EndDate = correctedEnd.Format(time.RFC3339)
		}
	}
	// If conditions aren't met, leave as-is and let validation handle it

	return input
}

// multiSessionThreshold is the default duration above which a single occurrence
// is considered suspicious as a multi-session event.
const multiSessionThreshold = 168 * time.Hour // 1 week

// IsMultiSessionEvent checks whether an event looks like a multi-session course
// or recurring series that was scraped as a single occurrence spanning a long period.
// Returns (true, reason) if the event should be flagged for review.
//
// Detection heuristics:
//  1. Duration > threshold (default 168h / 1 week, or custom per-source value from
//     EventInput.MultiSessionDurationThreshold): a single occurrence spanning > threshold is suspicious
//  2. Title patterns: "(N sessions)", "(N weeks)", "workshop series", etc.
func IsMultiSessionEvent(input EventInput) (bool, string) {
	threshold := multiSessionThreshold
	if input.MultiSessionDurationThreshold > 0 {
		threshold = input.MultiSessionDurationThreshold
	}

	// Duration check: only possible when both start and end dates are present.
	if input.EndDate != "" {
		startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartDate))
		endTime, err2 := time.Parse(time.RFC3339, strings.TrimSpace(input.EndDate))
		if err == nil && err2 == nil {
			duration := endTime.Sub(startTime)
			if duration > threshold {
				days := int(duration.Hours() / 24)
				return true, fmt.Sprintf("single occurrence spans %d days", days)
			}
		}
	}

	// Title pattern check (case-insensitive, compiled at package level).
	for _, re := range multiSessionPatterns {
		if re.MatchString(input.Name) {
			return true, fmt.Sprintf("title matches multi-session pattern: %s", re.String())
		}
	}

	return false, ""
}

// normalizeRegion normalises an addressRegion value to its ISO 3166-2 short
// subdivision code (upper-case). This matches the schema.org addressRegion
// convention (e.g. "WA" rather than "Washington").
// Unknown or already-abbreviated values are returned trimmed and upper-cased.
func normalizeRegion(region string) string {
	r := strings.ToUpper(strings.TrimSpace(region))
	if code, ok := regionNameToCode[r]; ok {
		return code
	}
	return r
}

// regionNameToCode maps full region names (upper-cased) to ISO 3166-2 short codes.
var regionNameToCode = map[string]string{
	// Canadian provinces & territories
	"ALBERTA":                   "AB",
	"BRITISH COLUMBIA":          "BC",
	"MANITOBA":                  "MB",
	"NEW BRUNSWICK":             "NB",
	"NEWFOUNDLAND AND LABRADOR": "NL",
	"NEWFOUNDLAND":              "NL",
	"NOVA SCOTIA":               "NS",
	"ONTARIO":                   "ON",
	"PRINCE EDWARD ISLAND":      "PE",
	"QUEBEC":                    "QC",
	"QUÉBEC":                    "QC",
	"SASKATCHEWAN":              "SK",
	"NORTHWEST TERRITORIES":     "NT",
	"NUNAVUT":                   "NU",
	"YUKON":                     "YT",
	// US states
	"ALABAMA":              "AL",
	"ALASKA":               "AK",
	"ARIZONA":              "AZ",
	"ARKANSAS":             "AR",
	"CALIFORNIA":           "CA",
	"COLORADO":             "CO",
	"CONNECTICUT":          "CT",
	"DELAWARE":             "DE",
	"FLORIDA":              "FL",
	"GEORGIA":              "GA",
	"HAWAII":               "HI",
	"IDAHO":                "ID",
	"ILLINOIS":             "IL",
	"INDIANA":              "IN",
	"IOWA":                 "IA",
	"KANSAS":               "KS",
	"KENTUCKY":             "KY",
	"LOUISIANA":            "LA",
	"MAINE":                "ME",
	"MARYLAND":             "MD",
	"MASSACHUSETTS":        "MA",
	"MICHIGAN":             "MI",
	"MINNESOTA":            "MN",
	"MISSISSIPPI":          "MS",
	"MISSOURI":             "MO",
	"MONTANA":              "MT",
	"NEBRASKA":             "NE",
	"NEVADA":               "NV",
	"NEW HAMPSHIRE":        "NH",
	"NEW JERSEY":           "NJ",
	"NEW MEXICO":           "NM",
	"NEW YORK":             "NY",
	"NORTH CAROLINA":       "NC",
	"NORTH DAKOTA":         "ND",
	"OHIO":                 "OH",
	"OKLAHOMA":             "OK",
	"OREGON":               "OR",
	"PENNSYLVANIA":         "PA",
	"RHODE ISLAND":         "RI",
	"SOUTH CAROLINA":       "SC",
	"SOUTH DAKOTA":         "SD",
	"TENNESSEE":            "TN",
	"TEXAS":                "TX",
	"UTAH":                 "UT",
	"VERMONT":              "VT",
	"VIRGINIA":             "VA",
	"WASHINGTON":           "WA",
	"WEST VIRGINIA":        "WV",
	"WISCONSIN":            "WI",
	"WYOMING":              "WY",
	"DISTRICT OF COLUMBIA": "DC",
}
