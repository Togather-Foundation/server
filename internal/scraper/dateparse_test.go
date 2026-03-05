package scraper

import (
	"testing"
	"time"
)

// ── isPartialISO8601 ──────────────────────────────────────────────────

func TestIsPartialISO8601(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"date only", "2026-03-05", true},
		{"datetime no tz", "2026-03-05T19:00:00", true},
		{"datetime no seconds", "2026-03-05T19:00", true},
		{"full RFC3339 Z", "2026-03-05T19:00:00Z", false},
		{"full RFC3339 offset", "2026-03-05T19:00:00-05:00", false},
		{"too short", "2026-03", false},
		{"empty", "", false},
		{"human date", "Thu 5th March", false},
		{"time only", "9:30 PM", false},
		{"year only", "2026", false},
		{"month-day", "03-05", false},
		{"bad separator", "2026/03/05", false},
		{"date with trailing text", "2026-03-05 extra", false},
		{"datetime with ms", "2026-03-05T19:00:00.000", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isPartialISO8601(tc.s)
			if got != tc.want {
				t.Errorf("isPartialISO8601(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// ── parseTimeText ─────────────────────────────────────────────────────

func TestParseTimeText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		wantOK bool
		hour   int
		minute int
	}{
		{"12h PM", "9:30 PM", true, 21, 30},
		{"12h AM", "9:30 AM", true, 9, 30},
		{"12h noon", "12:00 PM", true, 12, 0},
		{"12h midnight", "12:00 AM", true, 0, 0},
		{"24h", "21:30", true, 21, 30},
		{"no space AM/PM", "8:30PM", true, 20, 30},
		{"with periods", "9:30 p.m.", true, 21, 30},
		{"embedded in text", "Doors open at 7:30 PM sharp", true, 19, 30},
		{"no match", "March 5th", false, 0, 0},
		{"empty", "", false, 0, 0},
		{"invalid hour", "25:00 PM", false, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseTimeText(tc.s)
			if ok != tc.wantOK {
				t.Fatalf("parseTimeText(%q) ok = %v, want %v", tc.s, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.hour != tc.hour || got.minute != tc.minute {
				t.Errorf("parseTimeText(%q) = %d:%02d, want %d:%02d", tc.s, got.hour, got.minute, tc.hour, tc.minute)
			}
		})
	}
}

// ── parseDateText ─────────────────────────────────────────────────────

func TestParseDateText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		s         string
		wantOK    bool
		wantMonth time.Month
		wantDay   int
		wantYear  int // 0 = no year specified
	}{
		{"day-of-week ordinal month", "Thu 5th March", true, time.March, 5, 0},
		{"month day year", "March 5, 2026", true, time.March, 5, 2026},
		{"day month year", "5 March 2026", true, time.March, 5, 2026},
		{"abbreviated month", "5 Mar 2026", true, time.March, 5, 2026},
		{"no year", "5 March", true, time.March, 5, 0},
		{"friday ordinal", "Friday 1st August", true, time.August, 1, 0},
		{"second", "Monday 2nd June", true, time.June, 2, 0},
		{"third", "Wed 3rd September", true, time.September, 3, 0},
		{"ISO date", "2026-03-05", true, time.March, 5, 2026},
		{"ISO datetime", "2026-12-25T19:00:00", true, time.December, 25, 2026},
		{"empty", "", false, 0, 0, 0},
		{"time only", "9:30 PM", false, 0, 0, 0},
		{"just day-of-week", "Thursday", false, 0, 0, 0},
		{"number only", "42", false, 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseDateText(tc.s)
			if ok != tc.wantOK {
				t.Fatalf("parseDateText(%q) ok = %v, want %v", tc.s, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.month != tc.wantMonth {
				t.Errorf("month = %v, want %v", got.month, tc.wantMonth)
			}
			if got.day != tc.wantDay {
				t.Errorf("day = %d, want %d", got.day, tc.wantDay)
			}
			if got.year != tc.wantYear {
				t.Errorf("year = %d, want %d", got.year, tc.wantYear)
			}
		})
	}
}

// ── assembleDateTimeParts ─────────────────────────────────────────────

func TestAssembleDateTimeParts(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name      string
		parts     []string
		wantStart string // prefix to check (RFC 3339 up to seconds)
		wantEnd   string // prefix to check, or empty
	}{
		{
			name:      "Ticket Spot: date + time separate",
			parts:     []string{"Thu 5th March", "9:30 PM"},
			wantStart: "2026-03-05T21:30:00-",
		},
		{
			name:      "combined date+time in one string",
			parts:     []string{"March 5, 2026 9:30 PM"},
			wantStart: "2026-03-05T21:30:00-",
		},
		{
			name:      "RFC 3339 passthrough",
			parts:     []string{"2026-03-05T21:30:00Z"},
			wantStart: "2026-03-05T21:30:00Z",
		},
		{
			name:      "date only no time",
			parts:     []string{"March 5, 2026"},
			wantStart: "2026-03-05T00:00:00-",
		},
		{
			name:      "two times → start and end",
			parts:     []string{"March 5, 2026", "7:00 PM", "11:00 PM"},
			wantStart: "2026-03-05T19:00:00-",
			wantEnd:   "2026-03-05T23:00:00-",
		},
		{
			name:  "empty parts",
			parts: []string{},
		},
		{
			name:  "whitespace only",
			parts: []string{"  ", "\t"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start, end := assembleDateTimeParts(tc.parts, tz)

			if tc.wantStart == "" {
				if start != "" {
					t.Errorf("start = %q, want empty", start)
				}
				return
			}

			if !hasPrefix(start, tc.wantStart) {
				t.Errorf("start = %q, want prefix %q", start, tc.wantStart)
			}
			if tc.wantEnd != "" && !hasPrefix(end, tc.wantEnd) {
				t.Errorf("end = %q, want prefix %q", end, tc.wantEnd)
			}
			if tc.wantEnd == "" && end != "" {
				t.Errorf("end = %q, want empty", end)
			}
		})
	}
}

// ── normalizeDateToRFC3339 ────────────────────────────────────────────

func TestNormalizeDateToRFC3339(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name string
		s    string
		want string // exact match for passthrough, prefix for fuzzy
	}{
		{"RFC 3339 passthrough", "2026-03-05T21:30:00Z", "2026-03-05T21:30:00Z"},
		{"RFC 3339 with offset", "2026-03-05T21:30:00-05:00", "2026-03-05T21:30:00-05:00"},
		{"partial ISO date passthrough", "2026-03-05", "2026-03-05"},
		{"partial ISO datetime passthrough", "2026-03-05T19:00:00", "2026-03-05T19:00:00"},
		{"human date", "Thu 5th March 9:30 PM", "2026-03-05T21:30:00-"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeDateToRFC3339(tc.s, tz)
			// For passthrough cases, exact match. For fuzzy, prefix match.
			if isPartialISO8601(tc.s) || tc.s == "" {
				if got != tc.want {
					t.Errorf("normalizeDateToRFC3339(%q) = %q, want %q", tc.s, got, tc.want)
				}
			} else if _, err := time.Parse(time.RFC3339, tc.s); err == nil {
				if got != tc.want {
					t.Errorf("normalizeDateToRFC3339(%q) = %q, want %q", tc.s, got, tc.want)
				}
			} else {
				if !hasPrefix(got, tc.want) {
					t.Errorf("normalizeDateToRFC3339(%q) = %q, want prefix %q", tc.s, got, tc.want)
				}
			}
		})
	}
}

// ── normalizeStartDate ────────────────────────────────────────────────

func TestNormalizeStartDate(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name      string
		raw       RawEvent
		wantStart string
		wantEnd   string
	}{
		{
			name: "DateParts from date_selectors",
			raw: RawEvent{
				Name:      "Test Event",
				DateParts: []string{"Thu 5th March", "9:30 PM"},
			},
			// Fuzzy-parsed: includes timezone offset.
			wantStart: "2026-03-05T21:30:00-",
		},
		{
			name: "partial ISO StartDate passthrough",
			raw: RawEvent{
				Name:      "Test Event",
				StartDate: "2026-05-10T19:00:00",
				EndDate:   "2026-05-10T21:00:00",
			},
			wantStart: "2026-05-10T19:00:00",
			wantEnd:   "2026-05-10T21:00:00",
		},
		{
			name: "date-only passthrough",
			raw: RawEvent{
				Name:      "Concert",
				StartDate: "2026-06-01",
			},
			wantStart: "2026-06-01",
		},
		{
			name: "human-readable StartDate",
			raw: RawEvent{
				Name:      "Show",
				StartDate: "March 15, 2026 8:00 PM",
			},
			// Fuzzy-parsed: includes timezone offset.
			wantStart: "2026-03-15T20:00:00-",
		},
		{
			name: "no date at all",
			raw: RawEvent{
				Name: "No Date Event",
			},
			wantStart: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start, end := normalizeStartDate(tc.raw, tz)

			if tc.wantStart == "" {
				if start != "" {
					t.Errorf("start = %q, want empty", start)
				}
				return
			}

			if isPartialISO8601(tc.wantStart) {
				// Exact match for passthrough.
				if start != tc.wantStart {
					t.Errorf("start = %q, want %q", start, tc.wantStart)
				}
			} else if !hasPrefix(start, tc.wantStart) {
				t.Errorf("start = %q, want prefix %q", start, tc.wantStart)
			}

			if tc.wantEnd != "" {
				if isPartialISO8601(tc.wantEnd) {
					if end != tc.wantEnd {
						t.Errorf("end = %q, want %q", end, tc.wantEnd)
					}
				} else if !hasPrefix(end, tc.wantEnd) {
					t.Errorf("end = %q, want prefix %q", end, tc.wantEnd)
				}
			}
		})
	}
}

// ── inferYear ─────────────────────────────────────────────────────────

func TestInferYear(t *testing.T) {
	t.Parallel()

	// Use a fixed "now" so tests are deterministic.
	now := time.Date(2026, time.March, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		month time.Month
		day   int
		want  int
	}{
		{"future month same year", time.June, 15, 2026},
		{"same month later day", time.March, 20, 2026},
		{"recent past within 30 days", time.February, 10, 2026},
		{"far past rolls to next year", time.January, 1, 2027},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := inferYear(tc.month, tc.day, now)
			if got != tc.want {
				t.Errorf("inferYear(%v, %d) = %d, want %d", tc.month, tc.day, got, tc.want)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────

func TestRemoveDayOfWeek(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want string
	}{
		{"Thursday prefix", "Thursday 5th March", "5th March"},
		{"Thu prefix", "Thu 5th March", "5th March"},
		{"no prefix", "5th March", "5th March"},
		{"Friday", "Fri 1st August", "1st August"},
		{"Monday full", "Monday 2nd June", "2nd June"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := removeDayOfWeek(tc.s)
			if got != tc.want {
				t.Errorf("removeDayOfWeek(%q) = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want string
	}{
		{"multiple spaces", "hello   world", "hello world"},
		{"tabs and newlines", "hello\t\n world", "hello world"},
		{"leading trailing", "  hello world  ", "hello world"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeWhitespace(tc.s)
			if got != tc.want {
				t.Errorf("normalizeWhitespace(%q) = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

func TestFormatTimezone(t *testing.T) {
	t.Parallel()

	ref := time.Date(2026, time.March, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		tz   string
		want string
	}{
		{"UTC", "UTC", "Z"},
		{"Toronto EST", "America/Toronto", "-05:00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loc, err := time.LoadLocation(tc.tz)
			if err != nil {
				t.Fatalf("LoadLocation(%q): %v", tc.tz, err)
			}
			got := formatTimezone(loc, ref)
			if got != tc.want {
				t.Errorf("formatTimezone(%q) = %q, want %q", tc.tz, got, tc.want)
			}
		})
	}
}

// hasPrefix reports whether s starts with prefix. Simple helper since
// RFC 3339 timezone suffixes vary.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
