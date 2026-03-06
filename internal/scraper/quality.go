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
// With always-indexed DateParts (one entry per selector, empty string for misses),
// matchCount[i] correctly reflects how many events produced non-empty text for
// selector i. When a selector index is empty for ALL events, a
// WarnDateSelectorNeverMatched warning is emitted. When empty for some events,
// a WarnDateSelectorPartialMatch warning is emitted.
//
// firstProbes, when non-nil, provides per-selector DOM diagnostics from the
// first event container; these are appended to warning messages for enhanced
// visibility. Pass nil from Colly (Tier 1) call sites for backward compatibility.
//
// This function is a no-op (returns nil) when date_selectors is empty or
// when rawEvents is empty.
func checkDateSelectorQuality(rawEvents []RawEvent, config SourceConfig, firstProbes []DateSelectorProbe) []string {
	numSelectors := len(config.Selectors.DateSelectors)
	if numSelectors == 0 || len(rawEvents) == 0 {
		return nil
	}

	// matchCount[i] = number of events where selector i produced a non-empty
	// DateParts entry. With always-indexed DateParts, entry i reliably maps
	// to DateSelectors[i].
	matchCount := make([]int, numSelectors)

	for _, raw := range rawEvents {
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
		var probe *DateSelectorProbe
		if i < len(firstProbes) {
			p := firstProbes[i]
			probe = &p
		}

		switch {
		case matchCount[i] == 0:
			msg := fmt.Sprintf(
				"%s: selector #%d (%q) matched 0/%d events",
				WarnDateSelectorNeverMatched, i+1, selector, totalEvents,
			)
			if probe != nil {
				switch {
				case !probe.Matched:
					msg += " — first container: no element found for this selector"
				case probe.Text == "":
					msg += " — first container: element found but text was empty"
				default:
					msg += fmt.Sprintf(" — first container: %q", probe.Text)
				}
			}
			warnings = append(warnings, msg)
		case matchCount[i] < totalEvents:
			msg := fmt.Sprintf(
				"%s: selector #%d (%q) matched %d/%d events",
				WarnDateSelectorPartialMatch, i+1, selector, matchCount[i], totalEvents,
			)
			if probe != nil {
				switch {
				case probe.Matched && probe.Text != "":
					msg += fmt.Sprintf(" — first container: %q", probe.Text)
				case !probe.Matched:
					msg += " — first container: no element found for this selector"
				}
			}
			warnings = append(warnings, msg)
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
