package scraper

import (
	"testing"
	"time"
)

// testNow returns a fixed reference time for deterministic date-parsing tests.
// Pinned to Jan 15 2026 so yearless dates like "Thu 5th March" resolve to 2026
// (March is still in the future relative to January).
func testNow() time.Time {
	return time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
}

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
			d, tm, ok := parseFuzzy(tc.s, loc, testNow())
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

func TestSplitDateRange_TimeRangeNotDateRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool // true = should be a range, false = should NOT split
	}{
		{
			name:  "time range with AM/PM is not a date range",
			input: "1 - 3:30 pm",
			want:  false,
		},
		{
			name:  "time range with no month names is not a date range",
			input: "7:00 - 9:00 PM",
			want:  false,
		},
		{
			name:  "date range with month names still works",
			input: "Jan 20 - Mar 1, 2026",
			want:  true,
		},
		{
			name:  "date range with one month name still works",
			input: "Feb 3 - Mar 8, 2026",
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := splitDateRange(tc.input)
			if tc.want && got == nil {
				t.Errorf("splitDateRange(%q) = nil, want non-nil (should be recognized as date range)", tc.input)
			}
			if !tc.want && got != nil {
				t.Errorf("splitDateRange(%q) = %v, want nil (time range should not be split as date range)", tc.input, got)
			}
		})
	}
}

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
			start, end, inferred := assembleDateTimeParts(tc.parts, tz, testNow())

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
			_ = inferred // captured for completeness; explicit test coverage in TestAssembleDateTimeParts_YearInferred
		})
	}
}

// ── assembleDateTimeParts pipe-separated ──────────────────────────────

func TestAssembleDateTimeParts_PipeSeparatedDate(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name      string
		parts     []string
		wantStart string
		wantEnd   string
	}{
		{
			name:      "simple pipe: date | time",
			parts:     []string{"March 15, 2026 | 2:00 PM"},
			wantStart: "2026-03-15T14:00:00-",
		},
		{
			name:      "koffler-arts style: pipe between date and time range",
			parts:     []string{"Sunday March 29 | 1 - 3:30 pm"},
			wantStart: "2026-03-29T13:00:00-", // March 29 at 1pm (time range start)
			wantEnd:   "2026-03-29T15:30:00-", // March 29 at 3:30pm (time range end)
		},
		{
			name:      "date range still works (no pipe)",
			parts:     []string{"Jan 20 - Mar 1, 2026"},
			wantStart: "2026-01-20T00:00:00-",
			wantEnd:   "2026-03-01T00:00:00-",
		},
		{
			name:      "pipe at start (empty left side)",
			parts:     []string{"| March 15, 2026 2:00 PM"},
			wantStart: "2026-03-15T14:00:00-",
		},
		{
			name:      "pipe at end (empty right side)",
			parts:     []string{"March 15, 2026 2:00 PM |"},
			wantStart: "2026-03-15T14:00:00-",
		},
		{
			name:      "no pipe: normal date+time passes through",
			parts:     []string{"March 5, 2026 9:30 PM"},
			wantStart: "2026-03-05T21:30:00-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start, end, _ := assembleDateTimeParts(tc.parts, tz, testNow())

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
			got, _ := normalizeDateToRFC3339(tc.s, tz, testNow())
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
			start, end, _ := normalizeStartDate(tc.raw, tz, testNow())

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

// ── splitTimeRange ────────────────────────────────────────────────────

func TestSplitTimeRange(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantTimes []parsedTime
	}{
		{
			name:    "AM/PM propagation from end to start",
			input:   "1 - 3:30 pm",
			wantNil: false,
			wantTimes: []parsedTime{
				{hour: 13, minute: 0},
				{hour: 15, minute: 30},
			},
		},
		{
			name:    "AM/PM propagation from start to end",
			input:   "4 PM - 5",
			wantNil: false,
			wantTimes: []parsedTime{
				{hour: 16, minute: 0},
				{hour: 17, minute: 0},
			},
		},
		{
			name:    "both sides have AM/PM",
			input:   "10:00 am - 2:00 pm",
			wantNil: false,
			wantTimes: []parsedTime{
				{hour: 10, minute: 0},
				{hour: 14, minute: 0},
			},
		},
		{
			name:    "month name rejection (both sides)",
			input:   "Jan - Mar",
			wantNil: true,
		},
		{
			name:    "month name in first part",
			input:   "January 15 - 5:00 pm",
			wantNil: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantNil: true,
		},
		{
			name:    "single time (non-range)",
			input:   "3:30 pm",
			wantNil: true,
		},
		{
			name:    "malformed with extra text",
			input:   "foo bar baz",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := splitTimeRange(tc.input, loc, testNow())

			if tc.wantNil {
				if got != nil {
					t.Errorf("splitTimeRange(%q) = %+v, want nil", tc.input, got)
				}
				return
			}

			if len(got) != len(tc.wantTimes) {
				t.Fatalf("splitTimeRange(%q) returned %d times, want %d: %+v",
					tc.input, len(got), len(tc.wantTimes), got)
			}

			for i := range got {
				if got[i].hour != tc.wantTimes[i].hour || got[i].minute != tc.wantTimes[i].minute {
					t.Errorf("splitTimeRange(%q)[%d] = %d:%02d, want %d:%02d",
						tc.input, i, got[i].hour, got[i].minute,
						tc.wantTimes[i].hour, tc.wantTimes[i].minute)
				}
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

// ── year-inferred detection ────────────────────────────────────────────

func TestAssembleDateTimeParts_YearInferred(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name         string
		parts        []string
		wantStart    string // prefix to check
		wantInferred bool
	}{
		{
			name:         "date with explicit year - not inferred",
			parts:        []string{"March 5, 2026"},
			wantStart:    "2026-03-05T00:00:00-",
			wantInferred: false,
		},
		{
			name:         "date without explicit year - inferred",
			parts:        []string{"March 5"},
			wantStart:    "2026-03-05T00:00:00-",
			wantInferred: true,
		},
		{
			name:         "RFC 3339 passthrough - not inferred",
			parts:        []string{"2026-03-05T21:30:00Z"},
			wantStart:    "2026-03-05T21:30:00Z",
			wantInferred: false,
		},
		{
			name:         "date+time combined with year - not inferred",
			parts:        []string{"March 5, 2026 9:30 PM"},
			wantStart:    "2026-03-05T21:30:00-",
			wantInferred: false,
		},
		{
			name:         "date+time combined without year - inferred",
			parts:        []string{"March 5 9:30 PM"},
			wantStart:    "2026-03-05T21:30:00-",
			wantInferred: true,
		},
		{
			name:         "date range with explicit years - not inferred",
			parts:        []string{"Feb 3, 2026 - Mar 8, 2026"},
			wantStart:    "2026-02-03T00:00:00-",
			wantInferred: false,
		},
		{
			name:         "short date range without year - inferred",
			parts:        []string{"March 5-7"},
			wantStart:    "2026-03-05T00:00:00-",
			wantInferred: true,
		},
		{
			name:         "empty parts - not inferred",
			parts:        []string{},
			wantStart:    "",
			wantInferred: false,
		},
		{
			name:         "partial ISO date - not inferred",
			parts:        []string{"2026-03-05"},
			wantStart:    "2026-03-05T00:00:00-",
			wantInferred: false,
		},
		{
			name:         "day-of-week ordinal month without year - inferred",
			parts:        []string{"Thu 5th March"},
			wantStart:    "2026-03-05T00:00:00-",
			wantInferred: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start, _, inferred := assembleDateTimeParts(tc.parts, tz, testNow())

			if tc.wantStart == "" {
				if start != "" {
					t.Errorf("start = %q, want empty", start)
				}
			} else if !hasPrefix(start, tc.wantStart) {
				t.Errorf("start = %q, want prefix %q", start, tc.wantStart)
			}

			if inferred != tc.wantInferred {
				t.Errorf("yearWasInferred = %v, want %v", inferred, tc.wantInferred)
			}
		})
	}
}

func TestNormalizeDateToRFC3339_YearInferred(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name         string
		s            string
		wantInferred bool
	}{
		{"RFC 3339 explicit", "2026-03-05T21:30:00Z", false},
		{"partial ISO explicit", "2026-03-05", false},
		{"human date with year", "March 5, 2026", false},
		{"human date without year", "Thu 5th March", true},
		{"empty", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, inferred := normalizeDateToRFC3339(tc.s, tz, testNow())
			if inferred != tc.wantInferred {
				t.Errorf("yearWasInferred = %v, want %v", inferred, tc.wantInferred)
			}
		})
	}
}

func TestNormalizeStartDate_YearInferred(t *testing.T) {
	t.Parallel()

	tz := "America/Toronto"

	tests := []struct {
		name         string
		raw          RawEvent
		wantInferred bool
	}{
		{
			name: "DateParts with explicit year",
			raw: RawEvent{
				Name:      "Test",
				DateParts: []string{"March 5, 2026", "9:30 PM"},
			},
			wantInferred: false,
		},
		{
			name: "DateParts without year",
			raw: RawEvent{
				Name:      "Test",
				DateParts: []string{"Thu 5th March"},
			},
			wantInferred: true,
		},
		{
			name: "RFC 3339 StartDate - not inferred",
			raw: RawEvent{
				Name:      "Test",
				StartDate: "2026-03-05T21:30:00Z",
			},
			wantInferred: false,
		},
		{
			name: "human-readable StartDate without year",
			raw: RawEvent{
				Name:      "Show",
				StartDate: "March 15 8:00 PM",
			},
			wantInferred: true,
		},
		{
			name: "ISO StartDate has year - not inferred",
			raw: RawEvent{
				Name:      "Concert",
				StartDate: "2026-06-01",
			},
			wantInferred: false,
		},
		{
			name: "no date at all",
			raw: RawEvent{
				Name: "No Date",
			},
			wantInferred: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, inferred := normalizeStartDate(tc.raw, tz, testNow())
			if inferred != tc.wantInferred {
				t.Errorf("yearWasInferred = %v, want %v", inferred, tc.wantInferred)
			}
		})
	}
}
