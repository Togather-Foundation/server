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

// ── parseFuzzy ────────────────────────────────────────────────────────

func TestParseFuzzy(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		s         string
		wantOK    bool
		wantDate  bool // expect non-nil date
		wantMonth time.Month
		wantDay   int
		wantYear  int  // 0 = don't check
		wantTime  bool // expect non-nil time
		wantHour  int
		wantMin   int
	}{
		{"day-of-week ordinal month", "Thu 5th March", true, true, time.March, 5, 0, false, 0, 0},
		{"month day year", "March 5, 2026", true, true, time.March, 5, 2026, false, 0, 0},
		{"day month year", "5 March 2026", true, true, time.March, 5, 2026, false, 0, 0},
		{"abbreviated month", "5 Mar 2026", true, true, time.March, 5, 2026, false, 0, 0},
		{"no year", "5 March", true, true, time.March, 5, 0, false, 0, 0},
		{"friday ordinal", "Friday 1st August", true, true, time.August, 1, 0, false, 0, 0},
		{"second", "Monday 2nd June", true, true, time.June, 2, 0, false, 0, 0},
		{"third", "Wed 3rd September", true, true, time.September, 3, 0, false, 0, 0},
		{"12h PM time-only", "9:30 PM", true, false, 0, 0, 0, true, 21, 30},
		{"12h AM time-only", "9:30 AM", true, false, 0, 0, 0, true, 9, 30},
		{"24h time-only", "21:30", true, false, 0, 0, 0, true, 21, 30},
		{"date+time combined", "March 5, 2026 9:30 PM", true, true, time.March, 5, 2026, true, 21, 30},
		{"empty", "", false, false, 0, 0, 0, false, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, tm, ok := parseFuzzy(tc.s, loc)
			if ok != tc.wantOK {
				t.Fatalf("parseFuzzy(%q) ok = %v, want %v", tc.s, ok, tc.wantOK)
			}
			if !ok {
				return
			}

			if tc.wantDate {
				if d == nil {
					t.Fatal("expected date component, got nil")
				}
				if d.month != tc.wantMonth {
					t.Errorf("month = %v, want %v", d.month, tc.wantMonth)
				}
				if d.day != tc.wantDay {
					t.Errorf("day = %d, want %d", d.day, tc.wantDay)
				}
				if tc.wantYear != 0 && d.year != tc.wantYear {
					t.Errorf("year = %d, want %d", d.year, tc.wantYear)
				}
			} else if d != nil {
				t.Errorf("expected no date component, got %+v", *d)
			}

			if tc.wantTime {
				if tm == nil {
					t.Fatal("expected time component, got nil")
				}
				if tm.hour != tc.wantHour || tm.minute != tc.wantMin {
					t.Errorf("time = %d:%02d, want %d:%02d", tm.hour, tm.minute, tc.wantHour, tc.wantMin)
				}
			} else if tm != nil {
				t.Errorf("expected no time component, got %+v", *tm)
			}
		})
	}
}

// ── splitDateRange ────────────────────────────────────────────────────

func TestSplitDateRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string // nil = not a range
	}{
		{
			name:  "long dash range",
			input: "Feb 3, 2026 - Mar 8, 2026",
			want:  []string{"Feb 3, 2026", "Mar 8, 2026"},
		},
		{
			name:  "en-dash range",
			input: "Feb 3 – Mar 8, 2026",
			want:  []string{"Feb 3", "Mar 8, 2026"},
		},
		{
			name:  "short range same month",
			input: "March 5-7",
			want:  []string{"March 5", "March 7"},
		},
		{
			name:  "short range with year",
			input: "March 5-7, 2026",
			want:  []string{"March 5, 2026", "March 7, 2026"},
		},
		{
			name:  "short range abbreviated month",
			input: "Mar 5-7",
			want:  []string{"Mar 5", "Mar 7"},
		},
		{
			name:  "to separator",
			input: "Feb 3 to Mar 8",
			want:  []string{"Feb 3", "Mar 8"},
		},
		{
			name:  "not a range - single date",
			input: "March 5, 2026",
			want:  nil,
		},
		{
			name:  "not a range - time only",
			input: "9:30 PM",
			want:  nil,
		},
		{
			name:  "short range with ordinals",
			input: "March 5th-7th",
			want:  []string{"March 5", "March 7"},
		},
		{
			// Institutional listings (AGA Khan Museum, Art Museum U of T) use
			// en-dash WITHOUT surrounding spaces: "May 6, 2025–May 25, 2026".
			// The regular hyphen with no spaces is NOT matched (it appears in ISO
			// date strings like 2026-03-05), but en-dash is safe to match bare.
			name:  "en-dash range without spaces",
			input: "May 6, 2025–May 25, 2026",
			want:  []string{"May 6, 2025", "May 25, 2026"},
		},
		{
			name:  "en-dash range without spaces, abbreviated months",
			input: "Nov. 13, 2025–Mar. 17, 2026",
			want:  []string{"Nov. 13, 2025", "Mar. 17, 2026"},
		},
		{
			// ISO dates must NOT be split on the hyphen.
			name:  "ISO date is not a range",
			input: "2026-03-05",
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := splitDateRange(tc.input)
			if tc.want == nil {
				if got != nil {
					t.Errorf("splitDateRange(%q) = %v, want nil", tc.input, got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("splitDateRange(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitDateRange(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
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
		{
			name:      "date range in single string",
			parts:     []string{"Feb 3, 2026 - Mar 8, 2026"},
			wantStart: "2026-02-03T00:00:00-",
			wantEnd:   "2026-03-08T00:00:00-",
		},
		{
			name:      "short date range",
			parts:     []string{"March 5-7, 2026"},
			wantStart: "2026-03-05T00:00:00-",
			wantEnd:   "2026-03-07T00:00:00-",
		},
		{
			name:      "crows-theatre style date range",
			parts:     []string{"Feb 3 - Mar 8, 2026"},
			wantStart: "2026-02-03T00:00:00-",
			wantEnd:   "2026-03-08T00:00:00-",
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

// ── applyTimezoneToPartialISO8601 ─────────────────────────────────────

func TestApplyTimezoneToPartialISO8601(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name       string
		s          string
		wantPrefix string // prefix to match (timezone suffix varies with DST)
	}{
		{
			name:       "date-only",
			s:          "2026-03-05",
			wantPrefix: "2026-03-05T00:00:00-",
		},
		{
			name:       "datetime no seconds",
			s:          "2026-03-05T19:00",
			wantPrefix: "2026-03-05T19:00:00-",
		},
		{
			name:       "datetime with seconds",
			s:          "2026-03-05T19:00:00",
			wantPrefix: "2026-03-05T19:00:00-",
		},
		{
			name:       "summer datetime (EDT -04:00)",
			s:          "2026-07-15T20:00:00",
			wantPrefix: "2026-07-15T20:00:00-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := applyTimezoneToPartialISO8601(tc.s, tz)
			// Must be valid RFC 3339.
			if _, err := time.Parse(time.RFC3339, got); err != nil {
				t.Errorf("applyTimezoneToPartialISO8601(%q) = %q, not valid RFC 3339: %v", tc.s, got, err)
			}
			if !hasPrefix(got, tc.wantPrefix) {
				t.Errorf("applyTimezoneToPartialISO8601(%q) = %q, want prefix %q", tc.s, got, tc.wantPrefix)
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
		// Partial ISO 8601 inputs now get timezone applied to produce full RFC 3339.
		{"partial ISO date", "2026-03-05", "2026-03-05T00:00:00-"},
		{"partial ISO datetime", "2026-03-05T19:00:00", "2026-03-05T19:00:00-"},
		{"human date", "Thu 5th March 9:30 PM", "2026-03-05T21:30:00-"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeDateToRFC3339(tc.s, tz)
			// Exact match for RFC 3339 passthroughs; prefix match for all others.
			if _, err := time.Parse(time.RFC3339, tc.s); err == nil {
				if got != tc.want {
					t.Errorf("normalizeDateToRFC3339(%q) = %q, want %q", tc.s, got, tc.want)
				}
			} else if tc.s == "" {
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
			name: "partial ISO StartDate gets timezone applied",
			raw: RawEvent{
				Name:      "Test Event",
				StartDate: "2026-05-10T19:00:00",
				EndDate:   "2026-05-10T21:00:00",
			},
			// Now produces full RFC 3339 with timezone offset.
			wantStart: "2026-05-10T19:00:00-",
			wantEnd:   "2026-05-10T21:00:00-",
		},
		{
			name: "date-only gets timezone applied",
			raw: RawEvent{
				Name:      "Concert",
				StartDate: "2026-06-01",
			},
			wantStart: "2026-06-01T00:00:00-",
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
		{
			name: "DateParts with date range",
			raw: RawEvent{
				Name:      "Multi-day Show",
				DateParts: []string{"Feb 3 - Mar 8, 2026"},
			},
			wantStart: "2026-02-03T00:00:00-",
			wantEnd:   "2026-03-08T00:00:00-",
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

			if !hasPrefix(start, tc.wantStart) {
				t.Errorf("start = %q, want prefix %q", start, tc.wantStart)
			}

			if tc.wantEnd != "" {
				if !hasPrefix(end, tc.wantEnd) {
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

func TestHasTimePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"12h PM", "9:30 PM", true},
		{"12h AM", "9:30 AM", true},
		{"24h", "21:30", true},
		{"no space PM", "8:30PM", true},
		{"with periods", "9:30 p.m.", true},
		{"date only", "March 5, 2026", false},
		{"empty", "", false},
		{"no time", "Thursday", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasTimePattern(tc.s)
			if got != tc.want {
				t.Errorf("hasTimePattern(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// hasPrefix reports whether s starts with prefix. Simple helper since
// RFC 3339 timezone suffixes vary.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
