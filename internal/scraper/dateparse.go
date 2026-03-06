package scraper

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	dps "github.com/markusmobius/go-dateparser"
)

// assembleDateTimeParts takes a list of text strings extracted from CSS
// selectors (via date_selectors) and attempts to assemble RFC 3339 start
// and end datetimes. Each string may contain a date, a time, both, or a
// date range. The function classifies each part and combines them.
//
// Supported input patterns (non-exhaustive — go-dateparser handles 200+ locales):
//   - "Thu 5th March"           → date-only
//   - "9:30 PM"                 → time-only
//   - "Thu 5th March 9:30 PM"   → date+time
//   - "March 5, 2026"           → date-only with year
//   - "2026-03-05"              → ISO date
//   - "2026-03-05T21:30:00"     → ISO datetime
//   - "Feb 3 - Mar 8, 2026"    → date range (split on dash/en-dash)
//   - "March 5-7"              → short date range
//
// When year is missing, the current year is assumed. If the resulting date
// is more than 30 days in the past, the next year is used instead.
//
// The timezone parameter provides the IANA timezone for the source venue
// (e.g. "America/Toronto"). If empty, UTC is used.
//
// Returns startDate, endDate as RFC 3339 strings. endDate may be empty if
// no end information was found.
func assembleDateTimeParts(parts []string, timezone string) (startDate, endDate string) {
	if len(parts) == 0 {
		return "", ""
	}

	loc := loadTimezone(timezone)

	var dates []parsedDate
	var times []parsedTime

	for _, part := range parts {
		part = normalizeWhitespace(part)
		if part == "" {
			continue
		}

		// RFC 3339 passthrough — check first to avoid false matches
		// from the time/date extractors on ISO strings.
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(part)); err == nil {
			return strings.TrimSpace(part), ""
		}

		// Partial ISO 8601 passthrough (e.g. "2026-03-05T19:00:00", "2026-03-05").
		if isPartialISO8601(strings.TrimSpace(part)) {
			return strings.TrimSpace(part), ""
		}

		// Try splitting date ranges ("Feb 3 - Mar 8, 2026", "March 5-7").
		if rangeParts := splitDateRange(part); len(rangeParts) == 2 {
			for _, rp := range rangeParts {
				d, t, ok := parseFuzzy(rp, loc)
				if ok {
					if d != nil {
						dates = append(dates, *d)
					}
					if t != nil {
						times = append(times, *t)
					}
				}
			}
			continue
		}

		// Single value — parse via go-dateparser.
		d, t, ok := parseFuzzy(part, loc)
		if ok {
			if d != nil {
				dates = append(dates, *d)
			}
			if t != nil {
				times = append(times, *t)
			}
		}
	}

	// Propagate explicit years across date ranges: if one date in a pair
	// has an explicit year and the other doesn't, copy the year forward.
	// This handles "Feb 3 - Mar 8, 2026" where "Feb 3" has no year.
	if len(dates) == 2 {
		if dates[0].year == 0 && dates[1].year != 0 {
			dates[0].year = dates[1].year
		} else if dates[1].year == 0 && dates[0].year != 0 {
			dates[1].year = dates[0].year
		}
	}

	// Assemble: first date + first time → startDate.
	// If a second time exists, use first date + second time → endDate.
	now := time.Now().In(loc)

	if len(dates) == 0 && len(times) == 0 {
		return "", ""
	}

	// Build start datetime.
	startDT := buildDateTime(dates, times, 0, now, loc)
	if startDT.IsZero() {
		return "", ""
	}
	startDate = startDT.Format(time.RFC3339)

	// Build end datetime if there's a second time or second date.
	if len(times) > 1 || len(dates) > 1 {
		endDT := buildDateTime(dates, times, 1, now, loc)
		if !endDT.IsZero() {
			// End should be after start — if same day and end time < start time,
			// assume end is next day.
			if endDT.Before(startDT) {
				endDT = endDT.Add(24 * time.Hour)
			}
			endDate = endDT.Format(time.RFC3339)
		}
	}

	return startDate, endDate
}

// parsedDate holds the components of a parsed human-readable date.
type parsedDate struct {
	year  int // 0 means "not specified"
	month time.Month
	day   int
}

// parsedTime holds the components of a parsed human-readable time.
type parsedTime struct {
	hour   int
	minute int
}

// ── go-dateparser integration ─────────────────────────────────────────

// yearPattern detects whether the input string contains an explicit 4-digit
// year. When absent, parseFuzzy sets parsedDate.year = 0 so the caller's
// inferYear logic can apply its own heuristic (current year, bump if >30
// days in the past).
var yearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

// parseFuzzy uses go-dateparser to extract a date and/or time from a
// human-readable string. Returns pointers to parsedDate and parsedTime
// (nil if not found). The ok return is true if anything was extracted.
//
// Only calendar components (year, month, day, hour, minute) are extracted
// from go-dateparser's result — timezone handling is left to the caller's
// buildDateTime / loadTimezone logic.
func parseFuzzy(s string, loc *time.Location) (d *parsedDate, t *parsedTime, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil, false
	}

	cfg := &dps.Configuration{
		// Use the venue timezone so go-dateparser resolves "today/tomorrow"
		// relative to the venue, but we only extract the calendar fields.
		DefaultTimezone: loc,
		CurrentTime:     time.Now().In(loc),
	}

	dt, err := dps.Parse(cfg, s)
	if err != nil {
		return nil, nil, false
	}

	// Extract calendar components in the venue timezone so DST boundaries
	// don't shift the date/time values we pull out.
	local := dt.Time.In(loc)

	// Detect if the input is a time-only string (e.g. "9:30 PM", "21:30").
	// In this case go-dateparser fills in today's date, but we should only
	// return the time component so the assembly logic doesn't get a spurious
	// date.
	timeOnly := isTimeOnly(s)

	hasTime := hasTimePattern(s) ||
		(local.Hour() != 0 || local.Minute() != 0)

	if timeOnly {
		if !hasTime {
			return nil, nil, false
		}
		pt := parsedTime{
			hour:   local.Hour(),
			minute: local.Minute(),
		}
		return nil, &pt, true
	}

	pd := parsedDate{
		month: local.Month(),
		day:   local.Day(),
	}

	// If the input contained an explicit year, use it. Otherwise leave
	// year = 0 so inferYear applies the "current year, bump if stale" rule.
	if yearPattern.MatchString(s) {
		pd.year = local.Year()
	}

	if hasTime {
		pt := parsedTime{
			hour:   local.Hour(),
			minute: local.Minute(),
		}
		return &pd, &pt, true
	}

	return &pd, nil, true
}

// timeIndicator detects explicit time patterns in text. Used to distinguish
// "date with midnight time" from "date without time" since go-dateparser
// returns 00:00 for both.
var timeIndicator = regexp.MustCompile(
	`(?i)(\d{1,2}:\d{2}|\d{1,2}\s*(?:AM|PM|a\.m\.|p\.m\.))`,
)

// hasTimePattern reports whether s contains an explicit time reference.
func hasTimePattern(s string) bool {
	return timeIndicator.MatchString(s)
}

// isTimeOnly reports whether s contains only a time (no date content).
// Examples: "9:30 PM", "21:30", "7:00 PM" → true.
// Examples: "March 5 9:30 PM", "Thu 5th March" → false.
func isTimeOnly(s string) bool {
	if !hasTimePattern(s) {
		return false
	}
	// Strip the time pattern and see if anything date-like remains.
	stripped := timeIndicator.ReplaceAllString(s, "")
	stripped = strings.TrimSpace(stripped)
	// If nothing substantial remains, or only non-date words remain
	// (e.g. "Doors open at ... sharp"), treat as time-only.
	// Check for month names or digit sequences that look like dates.
	if stripped == "" {
		return true
	}
	// If what remains contains a month name or date-like number, it's not time-only.
	return !monthNamePattern.MatchString(stripped) && !yearPattern.MatchString(stripped) &&
		!dateDigitPattern.MatchString(stripped)
}

// dateDigitPattern matches standalone day-of-month numbers (1-31) that
// aren't part of a time pattern.
var dateDigitPattern = regexp.MustCompile(`\b([1-9]|[12]\d|3[01])\b`)

// ── Date range splitting ──────────────────────────────────────────────

// rangeSeparator matches " - ", " – ", " to ", or a bare dash between
// date-like fragments. The negative lookbehind/lookahead prevents splitting
// on dashes inside ISO dates (e.g. "2026-03-05").
var rangeSeparator = regexp.MustCompile(
	`\s+[-–]\s+|\s+to\s+`,
)

// shortRangePattern matches compact ranges like "March 5-7" or "March 5–7"
// where a month name is followed by day-dash-day.
var shortRangePattern = regexp.MustCompile(
	`(?i)((?:January|February|March|April|May|June|July|August|September|October|November|December|` +
		`Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)\s+\d{1,2})(?:st|nd|rd|th)?\s*[-–]\s*(\d{1,2})(?:st|nd|rd|th)?(?:\s*,?\s*(\d{4}))?`,
)

// splitDateRange splits a text containing a date range into two parts.
// Returns nil if the text is not a date range.
//
// Handles:
//   - "Feb 3, 2026 - Mar 8, 2026"  → ["Feb 3, 2026", "Mar 8, 2026"]
//   - "Feb 3 – Mar 8, 2026"        → ["Feb 3", "Mar 8, 2026"]
//   - "March 5-7"                   → ["March 5", "March 7"]
//   - "March 5-7, 2026"            → ["March 5, 2026", "March 7, 2026"]
func splitDateRange(s string) []string {
	// Try short range first: "March 5-7" or "March 5-7, 2026"
	if m := shortRangePattern.FindStringSubmatch(s); m != nil {
		startPart := m[1] // e.g. "March 5"
		endDay := m[2]    // e.g. "7"
		year := m[3]      // e.g. "2026" or ""

		// Extract month name from the start for the end date.
		monthName := extractMonthName(startPart)
		endPart := monthName + " " + endDay

		if year != "" {
			startPart += ", " + year
			endPart += ", " + year
		}
		return []string{startPart, endPart}
	}

	// Try long-form range: "Feb 3 - Mar 8, 2026"
	parts := rangeSeparator.Split(s, 2)
	if len(parts) == 2 {
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if left != "" && right != "" {
			return []string{left, right}
		}
	}

	return nil
}

// monthNamePattern extracts the first month name from a string.
var monthNamePattern = regexp.MustCompile(
	`(?i)(January|February|March|April|May|June|July|August|September|October|November|December|` +
		`Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Sept|Oct|Nov|Dec)`,
)

// extractMonthName returns the first month name found in s, or "".
func extractMonthName(s string) string {
	m := monthNamePattern.FindString(s)
	return m
}

// ── Assembly helpers ──────────────────────────────────────────────────

// buildDateTime constructs a time.Time from the idx-th date/time pair.
// For idx=0, uses first date + first time. For idx=1, uses second date (or
// first date if only one) + second time (or no time).
func buildDateTime(dates []parsedDate, times []parsedTime, idx int, now time.Time, loc *time.Location) time.Time {
	var d parsedDate
	var t parsedTime
	hasTime := false

	// Pick date: use idx-th if available, else first.
	if idx < len(dates) {
		d = dates[idx]
	} else if len(dates) > 0 {
		d = dates[0]
	} else {
		return time.Time{} // No date at all.
	}

	// Pick time: use idx-th if available.
	if idx < len(times) {
		t = times[idx]
		hasTime = true
	} else if idx == 0 && len(times) > 0 {
		t = times[0]
		hasTime = true
	}

	year := d.year
	if year == 0 {
		year = inferYear(d.month, d.day, now)
	}

	if hasTime {
		return time.Date(year, d.month, d.day, t.hour, t.minute, 0, 0, loc)
	}
	return time.Date(year, d.month, d.day, 0, 0, 0, 0, loc)
}

// inferYear guesses the year for a date with no year specified. Uses the
// current year unless the resulting date would be more than 30 days in the
// past, in which case next year is used.
func inferYear(month time.Month, day int, now time.Time) int {
	candidate := time.Date(now.Year(), month, day, 0, 0, 0, 0, now.Location())
	if candidate.Before(now.AddDate(0, 0, -30)) {
		return now.Year() + 1
	}
	return now.Year()
}

// loadTimezone returns the *time.Location for a timezone name, falling back to UTC.
func loadTimezone(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ── Utility functions ─────────────────────────────────────────────────

// normalizeWhitespace collapses multiple whitespace chars to a single space
// and trims leading/trailing whitespace.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// ── Fuzzy RFC 3339 normalization ──────────────────────────────────────

// normalizeDateToRFC3339 attempts to convert a human-readable date/time
// string into RFC 3339 format. If the input is already valid RFC 3339, it
// is returned as-is. Also handles common ISO 8601 variants that lack a
// timezone suffix by appending the source timezone.
//
// Examples:
//
//	"Thu 5th March 9:30 PM"      → "2026-03-05T21:30:00-05:00"
//	"March 5, 2026"              → "2026-03-05T00:00:00-05:00"
//	"2026-03-05T21:30:00Z"       → "2026-03-05T21:30:00Z" (passthrough)
//	"2026-03-05T21:30:00"        → "2026-03-05T21:30:00" (passthrough, no tz appended)
//	"2026-03-05"                 → "2026-03-05" (passthrough)
func normalizeDateToRFC3339(s string, timezone string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Already RFC 3339? Return as-is.
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}

	// ISO 8601 without timezone (e.g. "2026-03-05T19:00:00" or "2026-03-05")?
	// Pass through as-is — the downstream pipeline already handles these.
	if isPartialISO8601(s) {
		return s
	}

	// Try to parse as combined date+time text.
	result, _ := assembleDateTimeParts([]string{s}, timezone)
	return result
}

// combineAndNormalizeDateParts is called from NormalizeRawEvent when
// RawEvent has DateParts populated (from date_selectors config). It
// assembles the parts into RFC 3339 start and end dates.
func combineAndNormalizeDateParts(dateParts []string, timezone string) (startDate, endDate string) {
	return assembleDateTimeParts(dateParts, timezone)
}

// normalizeStartDate is a convenience wrapper used by NormalizeRawEvent.
// It handles three cases in priority order:
//  1. DateParts populated → assemble from parts
//  2. StartDate is already RFC 3339 → passthrough
//  3. StartDate is human-readable → fuzzy parse
//
// Returns the RFC 3339 startDate and endDate (endDate may be empty).
func normalizeStartDate(raw RawEvent, timezone string) (startDate, endDate string) {
	// Case 1: date_selectors produced DateParts.
	if len(raw.DateParts) > 0 {
		return combineAndNormalizeDateParts(raw.DateParts, timezone)
	}

	// Case 2+3: traditional start_date selector.
	startDate = normalizeDateToRFC3339(raw.StartDate, timezone)
	endDate = normalizeDateToRFC3339(raw.EndDate, timezone)
	return startDate, endDate
}

// formatTimezone returns a timezone-appropriate suffix for constructing
// RFC 3339 strings manually. Exported only for testing.
func formatTimezone(loc *time.Location, refTime time.Time) string {
	_, offset := refTime.In(loc).Zone()
	if offset == 0 {
		return "Z"
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

// isPartialISO8601 reports whether s looks like a partial ISO 8601 date or
// datetime that lacks a timezone suffix. These are already understood by the
// downstream pipeline (time.Parse with layout "2006-01-02T15:04:05" or
// "2006-01-02") and must pass through unchanged — no fuzzy parsing.
//
// Recognized patterns:
//
//	"2026-03-05"               → date-only
//	"2026-03-05T19:00:00"      → datetime without tz
//	"2026-03-05T19:00"         → datetime without seconds
//
// Full RFC 3339 strings (with "Z" or "+05:00") are NOT matched here — those
// are caught earlier by the time.Parse(time.RFC3339, ...) check.
func isPartialISO8601(s string) bool {
	// Must start with YYYY-MM-DD (10 chars minimum).
	if len(s) < 10 {
		return false
	}
	// Verify digit pattern for YYYY-MM-DD.
	for i, c := range s[:10] {
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	// Date-only: exactly 10 chars.
	if len(s) == 10 {
		return true
	}
	// Must have 'T' separator for datetime portion.
	if s[10] != 'T' {
		return false
	}
	rest := s[11:]
	// HH:MM or HH:MM:SS (no timezone suffix).
	switch len(rest) {
	case 5: // HH:MM
		return rest[2] == ':' && isDigits(rest[:2]) && isDigits(rest[3:5])
	case 8: // HH:MM:SS
		return rest[2] == ':' && rest[5] == ':' &&
			isDigits(rest[:2]) && isDigits(rest[3:5]) && isDigits(rest[6:8])
	default:
		return false
	}
}

// isDigits reports whether every byte in s is an ASCII digit.
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}
