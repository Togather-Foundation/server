package ical

import (
	"fmt"
	"time"

	"github.com/teambition/rrule-go"
)

// DefaultHorizonDays is the default time window for RRULE expansion.
const DefaultHorizonDays = 90

// DefaultMaxOccurrences is the default safety cap on expanded occurrences.
const DefaultMaxOccurrences = 100

// RRuleOptions controls RRULE expansion behavior.
type RRuleOptions struct {
	HorizonDays    int // How far forward to expand (default: 90)
	MaxOccurrences int // Safety cap (default: 100)
}

// ExpandRRule expands an RRULE string into concrete occurrence times.
//
// Parameters:
//   - rruleStr: raw RRULE value (e.g. "FREQ=WEEKLY;BYDAY=TU;COUNT=10")
//   - dtstart: the DTSTART of the master event (anchor for the rule)
//   - exdates: EXDATE times to exclude from expansion
//   - rdates: RDATE times to add as extra occurrences
//   - opts: expansion options (horizon, max cap)
//
// Returns occurrence start times in chronological order and a boolean
// indicating whether the MaxOccurrences cap was applied (true = truncated).
// The first occurrence is dtstart itself (unless excluded by EXDATE).
//
// If rruleStr is empty, returns just dtstart (unless excluded).
//
// Uses teambition/rrule-go for RFC 5545 RRULE evaluation.
func ExpandRRule(rruleStr string, dtstart time.Time, exdates, rdates []time.Time, opts RRuleOptions) ([]time.Time, bool, error) {
	horizonDays := opts.HorizonDays
	if horizonDays <= 0 {
		horizonDays = DefaultHorizonDays
	}
	maxOcc := opts.MaxOccurrences
	if maxOcc <= 0 {
		maxOcc = DefaultMaxOccurrences
	}

	// Empty RRULE: return just dtstart (unless excluded by EXDATE).
	if rruleStr == "" {
		for _, ex := range exdates {
			if ex.Equal(dtstart) {
				return nil, false, nil
			}
		}
		return []time.Time{dtstart}, false, nil
	}

	// Parse the RRULE string into options.
	rOpt, err := rrule.StrToROptionInLocation(rruleStr, dtstart.Location())
	if err != nil {
		return nil, false, fmt.Errorf("rrule parse %q: %w", rruleStr, err)
	}

	// Set DTSTART as the anchor.
	rOpt.Dtstart = dtstart

	r, err := rrule.NewRRule(*rOpt)
	if err != nil {
		return nil, false, fmt.Errorf("rrule create: %w", err)
	}

	// Build an RRuleSet with EXDATE and RDATE support.
	set := rrule.Set{}
	set.RRule(r)
	for _, ex := range exdates {
		set.ExDate(ex)
	}
	for _, rd := range rdates {
		set.RDate(rd)
	}

	// Use Between() to avoid materializing unbounded series.
	// Window: from dtstart to now + HorizonDays.
	// Past occurrences (before now) are excluded — a series that has ended
	// returns an empty slice.
	now := time.Now()
	windowStart := now
	if dtstart.After(now) {
		windowStart = dtstart
	}
	windowEnd := now.AddDate(0, 0, horizonDays)

	occurrences := set.Between(windowStart, windowEnd, true)

	// Apply MaxOccurrences cap.
	capped := false
	if len(occurrences) > maxOcc {
		occurrences = occurrences[:maxOcc]
		capped = true
	}

	return occurrences, capped, nil
}
