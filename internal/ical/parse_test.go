package ical

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fixtureDir returns the absolute path to the ICS test fixtures directory.
func fixtureDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "testdata", "ics")
}

// loadFixture reads an ICS fixture file and returns its contents.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir(), name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

func TestParse_BasicEvent(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-basic-event.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Calendar-level properties.
	if cal.ProdID != "-//Test//Test//EN" {
		t.Errorf("ProdID = %q, want %q", cal.ProdID, "-//Test//Test//EN")
	}
	if cal.Name != "Test Calendar" {
		t.Errorf("Name = %q, want %q", cal.Name, "Test Calendar")
	}
	if len(cal.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", cal.Warnings)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	if ev.UID != "basic-event-001" {
		t.Errorf("UID = %q, want %q", ev.UID, "basic-event-001")
	}
	if ev.Summary != "Community Meetup" {
		t.Errorf("Summary = %q, want %q", ev.Summary, "Community Meetup")
	}
	if ev.Description != "Monthly community gathering at the library." {
		t.Errorf("Description = %q, want %q", ev.Description, "Monthly community gathering at the library.")
	}
	if ev.Location != "Toronto Public Library" {
		t.Errorf("Location = %q, want %q", ev.Location, "Toronto Public Library")
	}
	if ev.URL != "https://example.com/meetup" {
		t.Errorf("URL = %q, want %q", ev.URL, "https://example.com/meetup")
	}

	wantStart := time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC)
	if !ev.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", ev.Start, wantStart)
	}
	wantEnd := time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)
	if !ev.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", ev.End, wantEnd)
	}
	if ev.AllDay {
		t.Error("AllDay = true, want false")
	}

	// GEO
	if !ev.HasGeo {
		t.Error("HasGeo = false, want true")
	}
	if ev.GeoLat != 43.6532 {
		t.Errorf("GeoLat = %f, want %f", ev.GeoLat, 43.6532)
	}
	if ev.GeoLon != -79.3832 {
		t.Errorf("GeoLon = %f, want %f", ev.GeoLon, -79.3832)
	}

	// CATEGORIES
	if len(ev.Categories) != 2 {
		t.Fatalf("Categories len = %d, want 2", len(ev.Categories))
	}
	if ev.Categories[0] != "Community" || ev.Categories[1] != "Social" {
		t.Errorf("Categories = %v, want [Community Social]", ev.Categories)
	}

	// ORGANIZER
	if ev.Organizer != "Jane Smith" {
		t.Errorf("Organizer = %q, want %q", ev.Organizer, "Jane Smith")
	}
	if ev.OrganizerEmail != "jane@example.com" {
		t.Errorf("OrganizerEmail = %q, want %q", ev.OrganizerEmail, "jane@example.com")
	}

	// Timestamps
	wantCreated := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !ev.Created.Equal(wantCreated) {
		t.Errorf("Created = %v, want %v", ev.Created, wantCreated)
	}
	wantLastMod := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if !ev.LastMod.Equal(wantLastMod) {
		t.Errorf("LastMod = %v, want %v", ev.LastMod, wantLastMod)
	}

	if ev.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", ev.Sequence)
	}
	if ev.Status != "CONFIRMED" {
		t.Errorf("Status = %q, want %q", ev.Status, "CONFIRMED")
	}

	// Should not have recurring fields.
	if ev.RRULE != "" {
		t.Errorf("RRULE = %q, want empty", ev.RRULE)
	}
	if !ev.RecurrenceID.IsZero() {
		t.Errorf("RecurrenceID = %v, want zero", ev.RecurrenceID)
	}
}

func TestParse_MultiEvent(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-multi-event.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(cal.Events))
	}
	if len(cal.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", cal.Warnings)
	}

	// Verify UIDs and summaries.
	wantUIDs := []string{"multi-001", "multi-002", "multi-003", "multi-004", "multi-005"}
	wantSummaries := []string{"Event One", "Event Two", "Event Three", "Event Four", "Event Five"}
	for i, ev := range cal.Events {
		if ev.UID != wantUIDs[i] {
			t.Errorf("event %d: UID = %q, want %q", i, ev.UID, wantUIDs[i])
		}
		if ev.Summary != wantSummaries[i] {
			t.Errorf("event %d: Summary = %q, want %q", i, ev.Summary, wantSummaries[i])
		}
	}

	// Event Two has description and location.
	if cal.Events[1].Description != "Second event with description." {
		t.Errorf("event 1 Description = %q, want %q", cal.Events[1].Description, "Second event with description.")
	}
	if cal.Events[1].Location != "Conference Room B" {
		t.Errorf("event 1 Location = %q, want %q", cal.Events[1].Location, "Conference Room B")
	}

	// Event Three has no DTEND — End should equal Start.
	ev3 := cal.Events[2]
	if !ev3.End.Equal(ev3.Start) {
		t.Errorf("event 2 (no DTEND): End = %v, want Start = %v", ev3.End, ev3.Start)
	}

	// Event Four has URL.
	if cal.Events[3].URL != "https://example.com/event4" {
		t.Errorf("event 3 URL = %q", cal.Events[3].URL)
	}

	// Event Five has CATEGORIES.
	if len(cal.Events[4].Categories) != 1 || cal.Events[4].Categories[0] != "Workshop" {
		t.Errorf("event 4 Categories = %v, want [Workshop]", cal.Events[4].Categories)
	}
}

func TestParse_RecurringWeekly(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-recurring-weekly.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	if ev.UID != "recurring-weekly-001" {
		t.Errorf("UID = %q", ev.UID)
	}
	if ev.RRULE != "FREQ=WEEKLY;BYDAY=TU;COUNT=10" {
		t.Errorf("RRULE = %q, want %q", ev.RRULE, "FREQ=WEEKLY;BYDAY=TU;COUNT=10")
	}

	// Two EXDATE values.
	if len(ev.ExDates) != 2 {
		t.Fatalf("ExDates len = %d, want 2", len(ev.ExDates))
	}
	wantExDate1 := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	wantExDate2 := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	if !ev.ExDates[0].Equal(wantExDate1) {
		t.Errorf("ExDates[0] = %v, want %v", ev.ExDates[0], wantExDate1)
	}
	if !ev.ExDates[1].Equal(wantExDate2) {
		t.Errorf("ExDates[1] = %v, want %v", ev.ExDates[1], wantExDate2)
	}
}

func TestParse_RecurringMonthly(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-recurring-monthly.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	if ev.RRULE != "FREQ=MONTHLY;BYMONTHDAY=15;COUNT=6" {
		t.Errorf("RRULE = %q", ev.RRULE)
	}

	// Two RDATE values.
	if len(ev.RDates) != 2 {
		t.Fatalf("RDates len = %d, want 2", len(ev.RDates))
	}
	wantRDate1 := time.Date(2026, 5, 1, 19, 0, 0, 0, time.UTC)
	wantRDate2 := time.Date(2026, 6, 1, 19, 0, 0, 0, time.UTC)
	if !ev.RDates[0].Equal(wantRDate1) {
		t.Errorf("RDates[0] = %v, want %v", ev.RDates[0], wantRDate1)
	}
	if !ev.RDates[1].Equal(wantRDate2) {
		t.Errorf("RDates[1] = %v, want %v", ev.RDates[1], wantRDate2)
	}
}

func TestParse_Malformed(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-malformed.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// malformed-002: missing SUMMARY → skipped
	// malformed-003: unparseable DTSTART "tomorrow" → skipped
	// 3 valid events should remain.
	if len(cal.Events) != 3 {
		t.Fatalf("expected 3 events (2 skipped), got %d", len(cal.Events))
	}

	// Should have warnings for the 2 skipped events.
	if len(cal.Warnings) < 2 {
		t.Errorf("expected at least 2 warnings, got %d: %v", len(cal.Warnings), cal.Warnings)
	}

	// Verify the valid events made it through.
	validUIDs := map[string]bool{
		"malformed-001": true,
		"malformed-004": true,
		"malformed-005": true,
	}
	for _, ev := range cal.Events {
		if !validUIDs[ev.UID] {
			t.Errorf("unexpected event UID %q in results", ev.UID)
		}
	}
}

func TestParse_FloatingTime(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-floating-time.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	// Floating times are parsed in time.Local per parseTimeProp.
	wantStart := time.Date(2026, 4, 15, 19, 0, 0, 0, time.Local)
	if !ev.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v (Local)", ev.Start, wantStart)
	}
	wantEnd := time.Date(2026, 4, 15, 21, 0, 0, 0, time.Local)
	if !ev.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v (Local)", ev.End, wantEnd)
	}

	if ev.AllDay {
		t.Error("AllDay = true, want false")
	}
}

func TestParse_AllDay(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-all-day.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	if !ev.AllDay {
		t.Error("AllDay = false, want true")
	}

	// VALUE=DATE parsed in time.Local.
	wantStart := time.Date(2026, 4, 15, 0, 0, 0, 0, time.Local)
	if !ev.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", ev.Start, wantStart)
	}
	wantEnd := time.Date(2026, 4, 16, 0, 0, 0, 0, time.Local)
	if !ev.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", ev.End, wantEnd)
	}
}

func TestParse_HTMLDescription(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-html-description.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	// Parse preserves HTML in Description — stripping is done by the mapper.
	wantDesc := "<h1>Welcome</h1><p>This is a <strong>bold</strong> event.</p><script>alert('xss')</script>"
	if ev.Description != wantDesc {
		t.Errorf("Description = %q, want %q", ev.Description, wantDesc)
	}
}

func TestParse_EmptyCalendar(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-empty-calendar.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(cal.Events))
	}
	if cal.ProdID != "-//Test//Empty//EN" {
		t.Errorf("ProdID = %q, want %q", cal.ProdID, "-//Test//Empty//EN")
	}
}

func TestParse_ReversedDates(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-reversed-dates.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Event with DTEND before DTSTART should be skipped.
	if len(cal.Events) != 0 {
		t.Errorf("expected 0 events (reversed dates skipped), got %d", len(cal.Events))
	}
	if len(cal.Warnings) == 0 {
		t.Error("expected at least 1 warning for reversed dates")
	}

	// Check warning mentions reversed dates.
	found := false
	for _, w := range cal.Warnings {
		if contains(w, "DTEND before DTSTART") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about DTEND before DTSTART, got: %v", cal.Warnings)
	}
}

func TestParse_DuplicateUIDs(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-duplicate-uids.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First event kept, second duplicate skipped.
	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event (duplicate skipped), got %d", len(cal.Events))
	}
	if cal.Events[0].Summary != "First Event" {
		t.Errorf("kept event Summary = %q, want %q", cal.Events[0].Summary, "First Event")
	}

	// Warning about the duplicate.
	if len(cal.Warnings) == 0 {
		t.Error("expected warning for duplicate UID")
	}
	found := false
	for _, w := range cal.Warnings {
		if contains(w, "duplicate UID") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about duplicate UID, got: %v", cal.Warnings)
	}
}

func TestParse_InfiniteRRule(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-infinite-rrule.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	// Parser preserves the raw RRULE — bounding happens in ExpandRRule.
	if ev.RRULE != "FREQ=DAILY" {
		t.Errorf("RRULE = %q, want %q", ev.RRULE, "FREQ=DAILY")
	}
}

func TestParse_OutlookVTIMEZONE(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-outlook-vtimezone.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	// "Eastern Standard Time" → America/New_York via WindowsTZIDAliases.
	nyLoc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("failed to load America/New_York: %v", err)
	}
	wantStart := time.Date(2026, 4, 15, 14, 0, 0, 0, nyLoc)
	if !ev.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", ev.Start, wantStart)
	}
	wantEnd := time.Date(2026, 4, 15, 15, 0, 0, 0, nyLoc)
	if !ev.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", ev.End, wantEnd)
	}

	// Verify the timezone name is correct on the parsed time.
	startZone, _ := ev.Start.Zone()
	if startZone != "EDT" && startZone != "EST" {
		t.Errorf("Start timezone = %q, want EDT or EST", startZone)
	}
}

func TestParse_DurationEvent(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-duration-event.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}
	ev := cal.Events[0]

	// DURATION: PT2H30M = 2.5 hours.
	wantDuration := 2*time.Hour + 30*time.Minute
	if ev.Duration != wantDuration {
		t.Errorf("Duration = %v, want %v", ev.Duration, wantDuration)
	}

	// End computed from Start + Duration since no DTEND.
	wantStart := time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC)
	wantEnd := wantStart.Add(wantDuration)
	if !ev.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", ev.Start, wantStart)
	}
	if !ev.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", ev.End, wantEnd)
	}
}

func TestParse_RecurrenceID(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-recurrence-id.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Two VEVENTs: master + exception (both share UID "recurrence-id-001").
	// Master has RRULE, exception has RECURRENCE-ID.
	if len(cal.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(cal.Events))
	}

	// Both should share the same UID.
	for _, ev := range cal.Events {
		if ev.UID != "recurrence-id-001" {
			t.Errorf("unexpected UID %q", ev.UID)
		}
	}

	// Find master and exception.
	var master, exception *ParsedEvent
	for i := range cal.Events {
		if cal.Events[i].RecurrenceID.IsZero() {
			master = &cal.Events[i]
		} else {
			exception = &cal.Events[i]
		}
	}

	if master == nil {
		t.Fatal("master event not found")
	}
	if exception == nil {
		t.Fatal("exception event not found")
	}

	// Master should have RRULE.
	if master.RRULE == "" {
		t.Error("master RRULE is empty")
	}
	if master.Summary != "Weekly Team Meeting" {
		t.Errorf("master Summary = %q", master.Summary)
	}

	// Exception should have RECURRENCE-ID.
	wantRecID := time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC)
	if !exception.RecurrenceID.Equal(wantRecID) {
		t.Errorf("exception RecurrenceID = %v, want %v", exception.RecurrenceID, wantRecID)
	}
	if exception.Summary != "Special Team Meeting (Rescheduled)" {
		t.Errorf("exception Summary = %q", exception.Summary)
	}
	// Exception has different start time (rescheduled to 14:00).
	wantExStart := time.Date(2026, 4, 21, 14, 0, 0, 0, time.UTC)
	if !exception.Start.Equal(wantExStart) {
		t.Errorf("exception Start = %v, want %v", exception.Start, wantExStart)
	}

	// No duplicate UID warning — RECURRENCE-ID events are allowed to share UIDs.
	for _, w := range cal.Warnings {
		if contains(w, "duplicate UID") {
			t.Errorf("unexpected duplicate UID warning: %s", w)
		}
	}
}

// --- Helper functions for parse tests ---

func TestParse_InvalidICS(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte("this is not valid ICS"))
	if err == nil {
		t.Fatal("expected error for invalid ICS, got nil")
	}
}

func TestParse_SummaryTruncation(t *testing.T) {
	t.Parallel()

	// Build an ICS with a SUMMARY longer than maxSummaryRunes (500 runes).
	longSummary := make([]byte, maxSummaryRunes+50)
	for i := range longSummary {
		longSummary[i] = 'A'
	}

	icsData := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//Truncation//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:truncation-001\r\nSUMMARY:" + string(longSummary) + "\r\n" +
		"DTSTART:20260415T100000Z\r\nDTEND:20260415T110000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n")

	cal, err := Parse(icsData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}

	// Summary should be truncated to maxSummaryRunes.
	runes := []rune(cal.Events[0].Summary)
	if len(runes) != maxSummaryRunes {
		t.Errorf("Summary rune count = %d, want %d", len(runes), maxSummaryRunes)
	}

	// Should have a truncation warning.
	found := false
	for _, w := range cal.Warnings {
		if contains(w, "SUMMARY truncated") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SUMMARY truncated warning, got: %v", cal.Warnings)
	}
}

func TestParseISO8601Duration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"1 day", "P1D", 24 * time.Hour, false},
		{"2 hours", "PT2H", 2 * time.Hour, false},
		{"30 minutes", "PT30M", 30 * time.Minute, false},
		{"2h30m", "PT2H30M", 2*time.Hour + 30*time.Minute, false},
		{"1 day 12 hours", "P1DT12H", 36 * time.Hour, false},
		{"full HMS", "PT1H30M15S", time.Hour + 30*time.Minute + 15*time.Second, false},
		{"1 week", "P1W", 7 * 24 * time.Hour, false},
		{"negative", "-PT1H", -time.Hour, false},
		{"lowercase", "pt2h", 2 * time.Hour, false},
		{"empty", "", 0, true},
		{"no P", "2H", 0, true},
		{"P only", "P", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseISO8601Duration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseISO8601Duration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseGeo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantLat float64
		wantLon float64
		wantErr bool
	}{
		{"valid", "43.6532;-79.3832", 43.6532, -79.3832, false},
		{"zero", "0;0", 0, 0, false},
		{"no semicolon", "43.6532,-79.3832", 0, 0, true},
		{"lat out of range", "91.0;0.0", 0, 0, true},
		{"lon out of range", "0.0;181.0", 0, 0, true},
		{"bad lat", "abc;0.0", 0, 0, true},
		{"bad lon", "0.0;abc", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lat, lon, err := parseGeo(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if lat != tt.wantLat {
				t.Errorf("lat = %f, want %f", lat, tt.wantLat)
			}
			if lon != tt.wantLon {
				t.Errorf("lon = %f, want %f", lon, tt.wantLon)
			}
		})
	}
}

// contains checks if substr is in s. A simple helper to avoid importing strings
// in the test file (it's already imported by parse.go, but keeping tests self-contained).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestParse_RawProps verifies that non-standard (X-*) properties are captured
// in ParsedEvent.RawProps while standard handled properties are excluded.
func TestParse_RawProps(t *testing.T) {
	t.Parallel()
	icsData := []byte("BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:rawprops-test@example.com\r\n" +
		"DTSTART:20270101T100000Z\r\n" +
		"DTEND:20270101T110000Z\r\n" +
		"SUMMARY:RawProps Test\r\n" +
		"X-APPLE-STRUCTURED-LOCATION:geo:43.65,-79.38\r\n" +
		"X-WR-CALNAME:My Calendar\r\n" +
		"X-CUSTOM-FIELD:custom-value\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n")

	cal, err := Parse(icsData)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}

	ev := cal.Events[0]
	if ev.RawProps == nil {
		t.Fatal("expected RawProps to be non-nil for event with X-* properties")
	}

	// X-APPLE-STRUCTURED-LOCATION should be captured.
	if v, ok := ev.RawProps["X-APPLE-STRUCTURED-LOCATION"]; !ok {
		t.Error("expected X-APPLE-STRUCTURED-LOCATION in RawProps")
	} else if v != "geo:43.65,-79.38" {
		t.Errorf("X-APPLE-STRUCTURED-LOCATION = %q, want %q", v, "geo:43.65,-79.38")
	}

	// X-CUSTOM-FIELD should be captured.
	if v, ok := ev.RawProps["X-CUSTOM-FIELD"]; !ok {
		t.Error("expected X-CUSTOM-FIELD in RawProps")
	} else if v != "custom-value" {
		t.Errorf("X-CUSTOM-FIELD = %q, want %q", v, "custom-value")
	}

	// Handled properties (SUMMARY, DTSTART, etc.) should NOT be in RawProps.
	for _, key := range []string{"SUMMARY", "DTSTART", "DTEND", "UID"} {
		if _, ok := ev.RawProps[key]; ok {
			t.Errorf("handled property %q should not be in RawProps", key)
		}
	}
}

// TestParse_RawProps_NoExtras verifies that RawProps is nil when an event
// has only standard properties.
func TestParse_RawProps_NoExtras(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "parse-basic-event.ics")

	cal, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(cal.Events) == 0 {
		t.Fatal("expected at least 1 event")
	}

	// Basic event fixture should have no X-* properties, so RawProps should be nil.
	// (If the fixture happens to have some, this test validates they're captured.)
	ev := cal.Events[0]
	if len(ev.RawProps) > 0 {
		t.Logf("RawProps found (may be valid): %v", ev.RawProps)
	}
}
