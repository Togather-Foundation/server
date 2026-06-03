package ical

import (
	"testing"
	"time"
)

// fixedNow is a reference time for tests. Tests that depend on "now" must
// use dtstart values relative to this. We set dtstart to a future date
// so occurrences fall within the horizon window.
func futureStart() time.Time {
	return time.Now().Add(24 * time.Hour) // tomorrow
}

func TestExpandRRule_DailyWithCount(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	occurrences, _, err := ExpandRRule("FREQ=DAILY;COUNT=5", dtstart, nil, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 5 {
		t.Fatalf("expected 5 occurrences, got %d", len(occurrences))
	}
	for i, occ := range occurrences {
		expected := dtstart.AddDate(0, 0, i)
		if !occ.Equal(expected) {
			t.Errorf("occurrence %d: expected %v, got %v", i, expected, occ)
		}
	}
}

func TestExpandRRule_WeeklyWithByday(t *testing.T) {
	t.Parallel()
	// Start on a Monday, recur on Wednesdays and Fridays.
	dtstart := futureStart().Truncate(time.Second)
	// Adjust to next Monday.
	for dtstart.Weekday() != time.Monday {
		dtstart = dtstart.AddDate(0, 0, 1)
	}

	occurrences, _, err := ExpandRRule("FREQ=WEEKLY;BYDAY=WE,FR;COUNT=6", dtstart, nil, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// COUNT=6 should give 6 occurrences: the first Wed, first Fri, second Wed, etc.
	// Note: DTSTART on Monday is NOT included because BYDAY=WE,FR doesn't match Monday.
	// teambition/rrule-go behavior: DTSTART is included only if it matches the rule.
	if len(occurrences) != 6 {
		t.Fatalf("expected 6 occurrences, got %d", len(occurrences))
	}
	for _, occ := range occurrences {
		wd := occ.Weekday()
		if wd != time.Wednesday && wd != time.Friday {
			t.Errorf("expected Wednesday or Friday, got %v on %v", wd, occ)
		}
	}
}

func TestExpandRRule_MonthlyWithBymonthday(t *testing.T) {
	t.Parallel()
	// Start on the 15th, recur monthly on the 15th.
	dtstart := time.Date(time.Now().Year(), time.Now().Month(), 15, 19, 0, 0, 0, time.UTC)
	if dtstart.Before(time.Now()) {
		dtstart = dtstart.AddDate(0, 1, 0)
	}

	// Use a 180-day horizon to ensure all 4 monthly occurrences fit.
	occurrences, _, err := ExpandRRule("FREQ=MONTHLY;BYMONTHDAY=15;COUNT=4", dtstart, nil, nil, RRuleOptions{
		HorizonDays: 180,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 4 {
		t.Fatalf("expected 4 occurrences, got %d", len(occurrences))
	}
	for _, occ := range occurrences {
		if occ.Day() != 15 {
			t.Errorf("expected day 15, got day %d on %v", occ.Day(), occ)
		}
	}
}

func TestExpandRRule_ExdateExclusion(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	// Daily for 5 days, exclude day 2 and day 4.
	exdates := []time.Time{
		dtstart.AddDate(0, 0, 1), // day 2
		dtstart.AddDate(0, 0, 3), // day 4
	}

	occurrences, _, err := ExpandRRule("FREQ=DAILY;COUNT=5", dtstart, exdates, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 3 {
		t.Fatalf("expected 3 occurrences (5 - 2 excluded), got %d", len(occurrences))
	}
	// Verify excluded dates are not present.
	for _, occ := range occurrences {
		for _, ex := range exdates {
			if occ.Equal(ex) {
				t.Errorf("excluded date %v should not appear", ex)
			}
		}
	}
}

func TestExpandRRule_RdateInclusion(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	// Daily for 3 days, plus 2 extra dates.
	rdates := []time.Time{
		dtstart.AddDate(0, 0, 10), // extra date 10 days out
		dtstart.AddDate(0, 0, 20), // extra date 20 days out
	}

	occurrences, _, err := ExpandRRule("FREQ=DAILY;COUNT=3", dtstart, nil, rdates, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 3 from RRULE + 2 from RDATE = 5
	if len(occurrences) != 5 {
		t.Fatalf("expected 5 occurrences, got %d", len(occurrences))
	}
	// Check that RDATE times are present.
	found := 0
	for _, occ := range occurrences {
		for _, rd := range rdates {
			if occ.Equal(rd) {
				found++
			}
		}
	}
	if found != 2 {
		t.Errorf("expected both RDATEs in results, found %d", found)
	}
}

func TestExpandRRule_MaxOccurrencesCap(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	// Daily with COUNT=200, but MaxOccurrences=10.
	occurrences, capped, err := ExpandRRule("FREQ=DAILY;COUNT=200", dtstart, nil, nil, RRuleOptions{
		MaxOccurrences: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 10 {
		t.Fatalf("expected 10 occurrences (capped), got %d", len(occurrences))
	}
	if !capped {
		t.Error("expected capped=true when MaxOccurrences was applied")
	}
}

func TestExpandRRule_HorizonDaysWindow(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	// Daily with COUNT=500 (would produce 500 days), HorizonDays=30.
	occurrences, _, err := ExpandRRule("FREQ=DAILY;COUNT=500", dtstart, nil, nil, RRuleOptions{
		HorizonDays:    30,
		MaxOccurrences: 500, // high cap, horizon should be the limit
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should get roughly 30 occurrences (within 30-day horizon from now).
	// The exact count depends on the relationship between dtstart and now.
	if len(occurrences) > 32 {
		t.Errorf("expected ~30 occurrences within horizon, got %d", len(occurrences))
	}
	if len(occurrences) == 0 {
		t.Error("expected at least some occurrences within 30-day horizon")
	}

	// Verify all occurrences are within the window.
	windowEnd := time.Now().AddDate(0, 0, 30)
	for _, occ := range occurrences {
		if occ.After(windowEnd.Add(time.Second)) {
			t.Errorf("occurrence %v is beyond horizon window %v", occ, windowEnd)
		}
	}
}

func TestExpandRRule_InvalidRRule(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	_, _, err := ExpandRRule("NOTARRULE", dtstart, nil, nil, RRuleOptions{})
	if err == nil {
		t.Fatal("expected error for invalid RRULE, got nil")
	}
}

func TestExpandRRule_NoCountNoUntil(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	// Infinite daily rule — should be bounded by HorizonDays.
	occurrences, _, err := ExpandRRule("FREQ=DAILY", dtstart, nil, nil, RRuleOptions{
		HorizonDays:    30,
		MaxOccurrences: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should get roughly 30 occurrences.
	if len(occurrences) == 0 {
		t.Error("expected occurrences for infinite rule within horizon")
	}
	if len(occurrences) > 32 {
		t.Errorf("expected ~30 occurrences, got %d (infinite rule should be horizon-bounded)", len(occurrences))
	}
}

func TestExpandRRule_AllOccurrencesInPast(t *testing.T) {
	t.Parallel()
	// DTSTART far in the past with COUNT=3 — all occurrences are before now.
	dtstart := time.Date(2020, 1, 1, 10, 0, 0, 0, time.UTC)

	occurrences, _, err := ExpandRRule("FREQ=DAILY;COUNT=3", dtstart, nil, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 0 {
		t.Errorf("expected 0 occurrences (all in past), got %d", len(occurrences))
	}
}

func TestExpandRRule_EmptyRRule(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	occurrences, _, err := ExpandRRule("", dtstart, nil, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 1 {
		t.Fatalf("expected 1 occurrence (just dtstart), got %d", len(occurrences))
	}
	if !occurrences[0].Equal(dtstart) {
		t.Errorf("expected dtstart %v, got %v", dtstart, occurrences[0])
	}
}

func TestExpandRRule_EmptyRRule_ExcludedByExdate(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	occurrences, _, err := ExpandRRule("", dtstart, []time.Time{dtstart}, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 0 {
		t.Errorf("expected 0 occurrences (dtstart excluded), got %d", len(occurrences))
	}
}

func TestExpandRRule_DtstartBeyondHorizon(t *testing.T) {
	t.Parallel()

	dtstart := time.Now().Add(120 * 24 * time.Hour).Truncate(time.Second)

	occurrences, _, err := ExpandRRule("FREQ=WEEKLY", dtstart, nil, nil, RRuleOptions{
		HorizonDays: 90,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 0 {
		t.Errorf("expected 0 occurrences (dtstart beyond horizon), got %d", len(occurrences))
	}
}

func TestExpandRRule_DefaultOptions(t *testing.T) {
	t.Parallel()
	dtstart := futureStart().Truncate(time.Second)

	// Zero-value options should use defaults (90 days, 100 max).
	occurrences, _, err := ExpandRRule("FREQ=DAILY;COUNT=5", dtstart, nil, nil, RRuleOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(occurrences) != 5 {
		t.Fatalf("expected 5 occurrences with default options, got %d", len(occurrences))
	}
}
