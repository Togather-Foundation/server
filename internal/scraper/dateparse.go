package scraper

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// assembleDateTimeParts takes a list of text strings extracted from CSS
// selectors (via date_selectors) and attempts to assemble RFC 3339 start
// and end datetimes. Each string may contain a date, a time, both, or a
// date range. The function classifies each part and combines them.
//
// Supported input patterns:
//   - "Thu 5th March"           → date-only
//   - "9:30 PM"                 → time-only
//   - "Thu 5th March 9:30 PM"   → date+time
//   - "March 5, 2026"           → date-only with year
//   - "2026-03-05"              → ISO date
//   - "2026-03-05T21:30:00"     → ISO datetime
//   - "March 5-7"               → date range (future)
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

		// Try to extract time first (more specific pattern).
		if t, ok := parseTimeText(part); ok {
			// Check if there's also a date in this text.
			dateText := removeTimePattern(part)
			dateText = normalizeWhitespace(dateText)
			if dateText != "" {
				if d, ok := parseDateText(dateText); ok {
					dates = append(dates, d)
				}
			}
			times = append(times, t)
			continue
		}

		// Try date-only parse.
		if d, ok := parseDateText(part); ok {
			dates = append(dates, d)
			continue
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

// ── Time parsing ──────────────────────────────────────────────────────

// timePattern matches "9:30 PM", "21:30", "8:30PM", "12:00 am", "9:30 p.m.", etc.
var timePattern = regexp.MustCompile(
	`(?i)\b(\d{1,2}):(\d{2})\s*(AM|PM|am|pm|a\.m\.|p\.m\.)?(?:\b|$)`,
)

// parseTimeText extracts a time from a text string.
func parseTimeText(s string) (parsedTime, bool) {
	m := timePattern.FindStringSubmatch(s)
	if m == nil {
		return parsedTime{}, false
	}

	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	ampm := strings.ToUpper(strings.ReplaceAll(m[3], ".", ""))

	if ampm == "PM" && hour < 12 {
		hour += 12
	} else if ampm == "AM" && hour == 12 {
		hour = 0
	}

	if hour > 23 || minute > 59 {
		return parsedTime{}, false
	}

	return parsedTime{hour: hour, minute: minute}, true
}

// removeTimePattern strips the time portion from a string, leaving date text.
func removeTimePattern(s string) string {
	return timePattern.ReplaceAllString(s, "")
}

// ── Date parsing ──────────────────────────────────────────────────────

// monthNames maps lowercase month names and abbreviations to time.Month.
var monthNames = map[string]time.Month{
	"january": time.January, "jan": time.January,
	"february": time.February, "feb": time.February,
	"march": time.March, "mar": time.March,
	"april": time.April, "apr": time.April,
	"may":  time.May,
	"june": time.June, "jun": time.June,
	"july": time.July, "jul": time.July,
	"august": time.August, "aug": time.August,
	"september": time.September, "sep": time.September, "sept": time.September,
	"october": time.October, "oct": time.October,
	"november": time.November, "nov": time.November,
	"december": time.December, "dec": time.December,
}

// ordinalSuffix strips "st", "nd", "rd", "th" from a day number string.
var ordinalSuffix = regexp.MustCompile(`(\d+)(?:st|nd|rd|th)\b`)

// parseDateText attempts to parse a human-readable date string.
// Handles: "Thu 5th March", "March 5, 2026", "5 March 2026", "2026-03-05", etc.
func parseDateText(s string) (parsedDate, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return parsedDate{}, false
	}

	// Try ISO date first: 2026-03-05
	if d, ok := parseISODate(s); ok {
		return d, true
	}

	// Strip ordinal suffixes: "5th" → "5"
	cleaned := ordinalSuffix.ReplaceAllString(s, "$1")

	// Remove day-of-week prefixes: "Thu", "Friday", etc.
	cleaned = removeDayOfWeek(cleaned)
	cleaned = normalizeWhitespace(cleaned)

	// Remove stray punctuation (commas, periods) but keep hyphens and spaces.
	cleaned = removePunctuation(cleaned)

	// Tokenize.
	tokens := strings.Fields(cleaned)
	if len(tokens) == 0 {
		return parsedDate{}, false
	}

	var month time.Month
	var day, year int
	var foundMonth, foundDay, foundYear bool

	for _, tok := range tokens {
		tok = strings.ToLower(tok)

		// Check if token is a month name.
		if m, ok := monthNames[tok]; ok && !foundMonth {
			month = m
			foundMonth = true
			continue
		}

		// Check if token is a number.
		n, err := strconv.Atoi(tok)
		if err != nil {
			continue
		}

		// Heuristic: numbers > 31 are years, 1-31 are days.
		if n > 31 && !foundYear {
			year = n
			foundYear = true
		} else if n >= 1 && n <= 31 && !foundDay {
			day = n
			foundDay = true
		}
	}

	if !foundMonth || !foundDay {
		return parsedDate{}, false
	}

	result := parsedDate{month: month, day: day}
	if foundYear {
		result.year = year
	}
	return result, true
}

// parseISODate parses "2026-03-05" or "2026-03-05T..." format.
func parseISODate(s string) (parsedDate, bool) {
	// Match YYYY-MM-DD at the start.
	if len(s) < 10 {
		return parsedDate{}, false
	}
	if s[4] != '-' || s[7] != '-' {
		return parsedDate{}, false
	}

	year, err1 := strconv.Atoi(s[0:4])
	monthNum, err2 := strconv.Atoi(s[5:7])
	day, err3 := strconv.Atoi(s[8:10])
	if err1 != nil || err2 != nil || err3 != nil {
		return parsedDate{}, false
	}
	if monthNum < 1 || monthNum > 12 || day < 1 || day > 31 {
		return parsedDate{}, false
	}

	return parsedDate{year: year, month: time.Month(monthNum), day: day}, true
}

// dayOfWeekPattern matches day-of-week names at the start of strings.
var dayOfWeekPattern = regexp.MustCompile(
	`(?i)^(?:Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday|` +
		`Mon|Tue|Tues|Wed|Weds|Thu|Thur|Thurs|Fri|Sat|Sun)\s*`,
)

// removeDayOfWeek strips a leading day-of-week name from a string.
func removeDayOfWeek(s string) string {
	return dayOfWeekPattern.ReplaceAllString(s, "")
}

// removePunctuation removes commas and periods from a string.
func removePunctuation(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ',' || r == '.' {
			return -1
		}
		return r
	}, s)
}

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
