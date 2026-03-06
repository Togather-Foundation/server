package scraper

import (
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

func TestCheckDateSelectorQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		rawEvents      []RawEvent
		config         SourceConfig
		firstProbes    []DateSelectorProbe
		wantWarnings   int
		wantSubstrings []string // each warning must contain at least one of these
	}{
		{
			name:      "no date_selectors configured — no warnings",
			rawEvents: []RawEvent{{Name: "A", DateParts: []string{"Thu 5th March"}}},
			config:    SourceConfig{Selectors: SelectorConfig{}},
		},
		{
			name:      "empty rawEvents — no warnings",
			rawEvents: nil,
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
		},
		{
			name: "all selectors match all events — no warnings",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"Thu 5th March", "9:30 PM"}},
				{Name: "B", DateParts: []string{"Fri 6th March", "10:00 PM"}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
		},
		{
			name: "selector #2 never matches — one warning",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"Thu 5th March", ""}},
				{Name: "B", DateParts: []string{"Fri 6th March", ""}},
				{Name: "C", DateParts: []string{"Sat 7th March", ""}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			wantWarnings:   1,
			wantSubstrings: []string{"date_selector_never_matched", "selector #2", "0/3"},
		},
		{
			name: "selector #2 partially matches — one warning",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"Thu 5th March", "9:30 PM"}},
				{Name: "B", DateParts: []string{"Fri 6th March", ""}},
				{Name: "C", DateParts: []string{"Sat 7th March", "11:00 PM"}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			wantWarnings:   1,
			wantSubstrings: []string{"date_selector_partial_match", "selector #2", "2/3"},
		},
		{
			name: "both selectors never match — two warnings",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: nil},
				{Name: "B", DateParts: nil},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			wantWarnings:   2,
			wantSubstrings: []string{"date_selector_never_matched"},
		},
		{
			name: "single event single selector — no warning when matched",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"2026-03-05"}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date"},
			}},
		},
		{
			name: "single event single selector — warning when not matched",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: nil},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date"},
			}},
			wantWarnings:   1,
			wantSubstrings: []string{"date_selector_never_matched", "0/1"},
		},
		// --- probe-enhanced tests ---
		{
			name: "never matched with probe: no DOM match",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"", ""}},
				{Name: "B", DateParts: []string{"", ""}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			firstProbes: []DateSelectorProbe{
				{Selector: ".date", Matched: false, Text: ""},
				{Selector: ".time", Matched: false, Text: ""},
			},
			wantWarnings:   2,
			wantSubstrings: []string{"no element found"},
		},
		{
			name: "never matched with probe: element found but empty text",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"", ""}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			firstProbes: []DateSelectorProbe{
				{Selector: ".date", Matched: true, Text: ""},
				{Selector: ".time", Matched: true, Text: ""},
			},
			wantWarnings:   2,
			wantSubstrings: []string{"element found but text was empty"},
		},
		{
			name: "partial match with probe showing text",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"Thu 5th March", "9:30 PM"}},
				{Name: "B", DateParts: []string{"Fri 6th March", ""}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			firstProbes: []DateSelectorProbe{
				{Selector: ".date", Matched: true, Text: "Thu 5th March"},
				{Selector: ".time", Matched: true, Text: "9:30 PM"},
			},
			wantWarnings:   1,
			wantSubstrings: []string{"partial_match", "first container:", "9:30 PM"},
		},
		{
			// Tier 1 (Colly) path: Colly appends only non-empty results, so
			// DateParts may be shorter than DateSelectors. Here one event has a
			// single DatePart against two DateSelectors, and firstProbes is nil
			// because the Colly path does not populate probe data. This is
			// intentional Colly backward-compat behavior — not a regression in
			// the always-indexed path used by Tier 2 (Rod).
			name: "nil probes — Tier 1 Colly path: DateParts shorter than DateSelectors",
			rawEvents: []RawEvent{
				{Name: "A", DateParts: []string{"Thu 5th March"}},
			},
			config: SourceConfig{Selectors: SelectorConfig{
				DateSelectors: []string{".date", ".time"},
			}},
			firstProbes:    nil,
			wantWarnings:   1,
			wantSubstrings: []string{"never_matched", "0/1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			warnings := checkDateSelectorQuality(tc.rawEvents, tc.config, tc.firstProbes)
			if len(warnings) != tc.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tc.wantWarnings, warnings)
			}
			for _, substr := range tc.wantSubstrings {
				found := false
				for _, w := range warnings {
					if contains(w, substr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected a warning containing %q, got: %v", substr, warnings)
				}
			}
		})
	}
}

func TestCheckAllMidnight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		events       []events.EventInput
		wantWarnings int
	}{
		{
			name:   "fewer than 3 events — no warning even if all midnight",
			events: makeEventsWithDates("2026-03-05T00:00:00", "2026-03-06T00:00:00"),
		},
		{
			name: "3 events all midnight — warning",
			events: makeEventsWithDates(
				"2026-03-05T00:00:00",
				"2026-03-06T00:00:00",
				"2026-03-07T00:00:00",
			),
			wantWarnings: 1,
		},
		{
			name: "3 events none midnight — no warning",
			events: makeEventsWithDates(
				"2026-03-05T19:00:00",
				"2026-03-06T20:00:00",
				"2026-03-07T21:30:00",
			),
		},
		{
			name: "5 events 4 midnight (80%) — warning",
			events: makeEventsWithDates(
				"2026-03-05T00:00:00",
				"2026-03-06T00:00:00",
				"2026-03-07T00:00:00",
				"2026-03-08T00:00:00",
				"2026-03-09T19:00:00",
			),
			wantWarnings: 1,
		},
		{
			name: "5 events 3 midnight (60%) — no warning (below 80%)",
			events: makeEventsWithDates(
				"2026-03-05T00:00:00",
				"2026-03-06T00:00:00",
				"2026-03-07T00:00:00",
				"2026-03-08T19:00:00",
				"2026-03-09T20:00:00",
			),
		},
		{
			name: "date-only strings without T — not counted as midnight",
			events: makeEventsWithDates(
				"2026-03-05",
				"2026-03-06",
				"2026-03-07",
			),
		},
		{
			name: "10 events all midnight — warning with count",
			events: makeEventsWithDates(
				"2026-03-01T00:00:00",
				"2026-03-02T00:00:00",
				"2026-03-03T00:00:00",
				"2026-03-04T00:00:00",
				"2026-03-05T00:00:00",
				"2026-03-06T00:00:00",
				"2026-03-07T00:00:00",
				"2026-03-08T00:00:00",
				"2026-03-09T00:00:00",
				"2026-03-10T00:00:00",
			),
			wantWarnings: 1,
		},
		{
			name: "midnight with timezone offset — still detected",
			events: makeEventsWithDates(
				"2026-03-05T00:00:00-05:00",
				"2026-03-06T00:00:00-05:00",
				"2026-03-07T00:00:00-05:00",
			),
			wantWarnings: 1,
		},
		{
			name:   "empty events — no warning",
			events: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			warnings := checkAllMidnight(tc.events)
			if len(warnings) != tc.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tc.wantWarnings, warnings)
			}
			if tc.wantWarnings > 0 {
				for _, w := range warnings {
					if !contains(w, "all_midnight") {
						t.Errorf("expected warning to contain 'all_midnight', got: %s", w)
					}
				}
			}
		})
	}
}

func TestHasMidnightTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"2026-03-05T00:00:00", true},
		{"2026-03-05T00:00:00-05:00", true},
		{"2026-03-05T00:00:00Z", true},
		{"2026-03-05T19:00:00", false},
		{"2026-03-05T12:00:00", false},
		{"2026-03-05", false}, // date-only, no T
		{"", false},           // empty
		{"T00:00:00", true},   // degenerate but contains the substring
		{"not a date", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := hasMidnightTime(tc.input)
			if got != tc.want {
				t.Errorf("hasMidnightTime(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// --- helpers ---

func makeEventsWithDates(dates ...string) []events.EventInput {
	evts := make([]events.EventInput, len(dates))
	for i, d := range dates {
		evts[i] = events.EventInput{
			Name:      "Event " + d,
			StartDate: d,
		}
	}
	return evts
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
