package scraper

import (
	"fmt"
	"strings"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

// Quality warning code prefixes, matching the pattern used by
// internal/domain/events/ingest.go appendQualityWarnings().
const (
	// WarnDateSelectorNeverMatched indicates a date_selector that matched
	// zero events in the entire batch.
	WarnDateSelectorNeverMatched = "date_selector_never_matched"

	// WarnDateSelectorPartialMatch indicates a date_selector that matched
	// some but not all events.
	WarnDateSelectorPartialMatch = "date_selector_partial_match"

	// WarnAllMidnight indicates that a suspiciously high fraction of events
	// have T00:00:00 start times, suggesting time extraction failure.
	WarnAllMidnight = "all_midnight"
)

// checkDateSelectorQuality analyses the raw events extracted via
// date_selectors and returns quality warnings when selectors fail to match.
//
// It compares each event's DateParts length against the number of configured
// DateSelectors. When a selector index fails to produce text for ANY event
// in the batch, a WarnDateSelectorNeverMatched warning is emitted. When a
// selector fails for some (but not all) events, a
// WarnDateSelectorPartialMatch warning is emitted.
//
// This function is a no-op (returns nil) when date_selectors is empty or
// when rawEvents is empty.
func checkDateSelectorQuality(rawEvents []RawEvent, config SourceConfig) []string {
	numSelectors := len(config.Selectors.DateSelectors)
	if numSelectors == 0 || len(rawEvents) == 0 {
		return nil
	}

	// matchCount[i] = number of events where selector i produced a non-empty
	// DateParts entry.
	matchCount := make([]int, numSelectors)

	for _, raw := range rawEvents {
		// DateParts are appended in order by extractEventsFromHTML: the Nth
		// entry corresponds to the Nth selector ONLY if all preceding selectors
		// also matched. When a selector fails (no element found or empty text),
		// no entry is appended, so len(DateParts) < numSelectors.
		//
		// To correctly attribute which selectors matched, we count entries up
		// to min(len(DateParts), numSelectors).
		for i := 0; i < len(raw.DateParts) && i < numSelectors; i++ {
			if raw.DateParts[i] != "" {
				matchCount[i]++
			}
		}
	}

	totalEvents := len(rawEvents)
	var warnings []string

	for i := 0; i < numSelectors; i++ {
		selector := config.Selectors.DateSelectors[i]
		switch {
		case matchCount[i] == 0:
			warnings = append(warnings, fmt.Sprintf(
				"%s: selector #%d (%q) matched 0/%d events",
				WarnDateSelectorNeverMatched, i+1, selector, totalEvents,
			))
		case matchCount[i] < totalEvents:
			warnings = append(warnings, fmt.Sprintf(
				"%s: selector #%d (%q) matched %d/%d events",
				WarnDateSelectorPartialMatch, i+1, selector, matchCount[i], totalEvents,
			))
		}
	}

	return warnings
}

// checkAllMidnight examines normalized events and returns a quality warning
// when a suspiciously high fraction have T00:00:00 start times. This is a
// safety net for date extraction failures where the date portion parsed
// correctly but the time portion was lost.
//
// The heuristic fires when:
//   - There are 3 or more events (below this threshold midnight times are
//     plausible for single-event pages)
//   - More than 80% of events have midnight (T00:00:00) start times
//
// Returns nil when no issue is detected.
func checkAllMidnight(validEvents []events.EventInput) []string {
	if len(validEvents) < 3 {
		return nil
	}

	midnightCount := 0
	for _, evt := range validEvents {
		if hasMidnightTime(evt.StartDate) {
			midnightCount++
		}
	}

	total := len(validEvents)
	// >=80% threshold: midnightCount * 5 >= total * 4
	if midnightCount*5 >= total*4 {
		return []string{fmt.Sprintf(
			"%s: %d/%d events have T00:00:00 start times — possible time extraction failure",
			WarnAllMidnight, midnightCount, total,
		)}
	}

	return nil
}

// hasMidnightTime checks whether an RFC 3339 (or partial ISO 8601) date
// string contains T00:00:00. Date-only strings (no "T") are NOT considered
// midnight — they represent a deliberate date-only value.
func hasMidnightTime(dateStr string) bool {
	return strings.Contains(dateStr, "T00:00:00")
}
