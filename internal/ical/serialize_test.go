package ical

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

var update = flag.Bool("update", false, "update golden fixtures")

func TestSerializeEvents(t *testing.T) {
	venueName := "Test Venue"
	virtualURL := "https://example.com/stream"

	tests := []struct {
		name           string
		events         []events.Event
		opts           SerializeOptions
		wantEventCount int
		wantWarnings   int
		checkFunc      func(*testing.T, []byte, []string)
	}{
		{
			name:           "empty events slice",
			events:         []events.Event{},
			opts:           SerializeOptions{CalendarName: "Empty Calendar"},
			wantEventCount: 0,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if !bytes.Contains(data, []byte("BEGIN:VCALENDAR")) {
					t.Error("expected VCALENDAR wrapper")
				}
				if !bytes.Contains(data, []byte("VERSION:2.0")) {
					t.Error("expected VERSION:2.0")
				}
			},
		},
		{
			name: "single event single occurrence",
			events: []events.Event{
				{
					ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7X7PZK6J7",
					Name:        "Test Event",
					Description: "A test event description",
					Occurrences: []events.Occurrence{
						{
							ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
							Timezone:  "America/Toronto",
							VenueULID: ptr("01HYX7QVATM8PZK6J7X7PZK6J7"),
						},
					},
					PrimaryVenueName: &venueName,
				},
			},
			opts:           SerializeOptions{},
			wantEventCount: 1,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if !bytes.Contains(data, []byte("01HYX7QVATM8PZK6J7X7PZK6J7@")) {
					t.Error("expected event ULID in UID")
				}
				if !bytes.Contains(data, []byte("DTSTART:")) {
					t.Error("expected DTSTART")
				}
				if !bytes.Contains(data, []byte("SUMMARY:Test Event")) {
					t.Error("expected SUMMARY")
				}
			},
		},
		{
			name: "single event multi occurrence",
			events: []events.Event{
				{
					ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name: "Weekly Meetup",
					Occurrences: []events.Occurrence{
						{
							ID:        "occ1",
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
						},
						{
							ID:        "occ2",
							StartTime: time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)),
						},
					},
				},
			},
			opts:           SerializeOptions{},
			wantEventCount: 2,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if !bytes.Contains(data, []byte("01HYX7QVATM8PZK6J7X7PZK6J7-occ1@")) {
					t.Error("expected occurrence 1 UID")
				}
				if !bytes.Contains(data, []byte("01HYX7QVATM8PZK6J7X7PZK6J7-occ2@")) {
					t.Error("expected occurrence 2 UID")
				}
			},
		},
		{
			name: "nil end time",
			events: []events.Event{
				{
					ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name: "Open House",
					Occurrences: []events.Occurrence{
						{
							ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
							StartTime: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
						},
					},
				},
			},
			opts:           SerializeOptions{},
			wantEventCount: 1,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if bytes.Contains(data, []byte("DTEND:")) {
					t.Error("expected no DTEND when EndTime is nil")
				}
			},
		},
		{
			name: "location from venue",
			events: []events.Event{
				{
					ULID:             "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name:             "Venue Event",
					PrimaryVenueName: &venueName,
					Occurrences: []events.Occurrence{
						{
							ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							VenueULID: ptr("01HYX7QVATM8PZK6J7X7PZK6J7"),
						},
					},
				},
			},
			opts:           SerializeOptions{},
			wantEventCount: 1,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if !bytes.Contains(data, []byte("LOCATION:Test Venue")) {
					t.Error("expected LOCATION from venue name")
				}
			},
		},
		{
			name: "location from virtual URL",
			events: []events.Event{
				{
					ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name: "Online Event",
					Occurrences: []events.Occurrence{
						{
							ID:         "01HYX7QVATM8PZK6J7X7PZK6J7",
							StartTime:  time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							VirtualURL: &virtualURL,
						},
					},
				},
			},
			opts:           SerializeOptions{},
			wantEventCount: 1,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if !bytes.Contains(data, []byte("LOCATION:https://example.com/stream")) {
					t.Error("expected LOCATION from virtual URL")
				}
			},
		},
		{
			name: "ticket URL as URL",
			events: []events.Event{
				{
					ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name: "Ticketed Event",
					Occurrences: []events.Occurrence{
						{
							ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							TicketURL: "https://tickets.example.com/event123",
						},
					},
				},
			},
			opts:           SerializeOptions{},
			wantEventCount: 1,
			wantWarnings:   0,
			checkFunc: func(t *testing.T, data []byte, warnings []string) {
				if !bytes.Contains(data, []byte("URL:https://tickets.example.com/event123")) {
					t.Error("expected URL from ticket URL")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SerializeEvents(tt.events, tt.opts)
			if err != nil {
				t.Fatalf("SerializeEvents() error = %v", err)
			}

			if tt.wantEventCount > 0 && len(result.Data) == 0 {
				t.Error("expected non-empty data")
			}

			eventCount := bytes.Count(result.Data, []byte("BEGIN:VEVENT"))
			if eventCount != tt.wantEventCount {
				t.Errorf("got %d VEVENTs, want %d", eventCount, tt.wantEventCount)
			}

			if len(result.Warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(result.Warnings), tt.wantWarnings, result.Warnings)
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, result.Data, result.Warnings)
			}
		})
	}
}

func TestSerializeSingleEvent(t *testing.T) {
	venueName := "Test Venue"
	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Single Event Test",
		Description: "Test description",
		Occurrences: []events.Occurrence{
			{
				ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
			},
		},
		PrimaryVenueName: &venueName,
	}

	result, err := SerializeSingleEvent(evt, SerializeOptions{
		CalendarName: "Test Calendar",
	})
	if err != nil {
		t.Fatalf("SerializeSingleEvent() error = %v", err)
	}

	eventCount := bytes.Count(result.Data, []byte("BEGIN:VEVENT"))
	if eventCount != 1 {
		t.Errorf("got %d VEVENTs, want 1", eventCount)
	}

	if !bytes.Contains(result.Data, []byte("X-WR-CALNAME:Test Calendar")) {
		t.Error("expected X-WR-CALNAME property")
	}
}

func TestSerializeEventsParsesBack(t *testing.T) {
	venueName := "Parsing Test Venue"
	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Roundtrip Test Event",
		Description: "Test description for roundtrip",
		Occurrences: []events.Occurrence{
			{
				ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
				Timezone:  "America/New_York",
			},
		},
		PrimaryVenueName: &venueName,
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 parsed event, got %d", len(parsed.Events))
	}

	if parsed.Events[0].Summary != "Roundtrip Test Event" {
		t.Errorf("expected summary 'Roundtrip Test Event', got %q", parsed.Events[0].Summary)
	}

	if !parsed.Events[0].Start.IsZero() {
		t.Logf("Start time parsed: %v", parsed.Events[0].Start)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestExportFixturesParse(t *testing.T) {
	files := []struct {
		path   string
		events []events.Event
		opts   SerializeOptions
	}{
		{
			path: "../../tests/testdata/ics/export-single-event.ics",
			events: []events.Event{
				{
					ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name:        "Community Meetup",
					Description: "Join us for a monthly community gathering.",
					Occurrences: []events.Occurrence{
						{
							ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
							VenueULID: ptr("01HYX7QVATM8PZK6J7X7PZK6J7"),
							TicketURL: "https://example.com/events/meetup",
						},
					},
					PrimaryVenueName: ptr("Community Center"),
				},
			},
			opts: SerializeOptions{},
		},
		{
			path: "../../tests/testdata/ics/export-multi-occurrence.ics",
			events: []events.Event{
				{
					ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
					Name:        "Weekly Workshop",
					Description: "Learn new skills in this weekly workshop series.",
					Occurrences: []events.Occurrence{
						{
							ID:        "occevent1",
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
						},
						{
							ID:        "occevent2",
							StartTime: time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)),
						},
						{
							ID:        "occevent3",
							StartTime: time.Date(2026, 4, 29, 19, 0, 0, 0, time.UTC),
							EndTime:   ptr(time.Date(2026, 4, 29, 21, 0, 0, 0, time.UTC)),
						},
					},
					PrimaryVenueName: ptr("Art Studio"),
				},
			},
			opts: SerializeOptions{},
		},
	}

	for _, f := range files {
		t.Run(f.path, func(t *testing.T) {
			result, err := SerializeEvents(f.events, f.opts)
			if err != nil {
				t.Fatalf("SerializeEvents() error = %v", err)
			}

			if *update {
				err := os.WriteFile(f.path, result.Data, 0644)
				if err != nil {
					t.Fatalf("failed to update fixture: %v", err)
				}
				t.Logf("updated fixture: %s", f.path)
				return
			}

			expected, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("failed to read fixture: %v", err)
			}

			resultData := stripDTSTAMP(result.Data)
			expectedData := stripDTSTAMP(expected)

			if !bytes.Equal(resultData, expectedData) {
				t.Errorf("fixture mismatch\ngot:\n%s\nexpected:\n%s", resultData, expectedData)
			}

			parsed, err := Parse(result.Data)
			if err != nil {
				t.Fatalf("failed to parse fixture: %v", err)
			}
			t.Logf("parsed %d events from %s", len(parsed.Events), f.path)
		})
	}
}

func stripDTSTAMP(data []byte) []byte {
	s := string(data)
	lines := strings.Split(s, "\r\n")
	if len(lines) == 1 {
		lines = strings.Split(s, "\n")
	}
	var filtered []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "DTSTAMP:") {
			filtered = append(filtered, line)
		}
	}
	return []byte(strings.Join(filtered, "\r\n"))
}

func TestSerializeRRuleMode_RecurringEvent(t *testing.T) {
	recurrence := &events.RecurrenceRule{
		RRule:   "FREQ=WEEKLY;BYDAY=MO,WE",
		ExDates: []time.Time{time.Date(2026, 5, 4, 18, 0, 0, 0, time.UTC)},
		RDates:  []time.Time{time.Date(2026, 6, 1, 19, 0, 0, 0, time.UTC)},
		TZID:    "America/Toronto",
	}

	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Weekly Meetup",
		Description: "Recurring weekly meetup.",
		Recurrence:  recurrence,
		Occurrences: []events.Occurrence{
			{
				ID:        "occ1",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
			},
			{
				ID:        "occ2",
				StartTime: time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{
		IncludeRRule: true,
	})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	data := result.Data

	eventCount := bytes.Count(data, []byte("BEGIN:VEVENT"))
	if eventCount != 1 {
		t.Errorf("expected 1 VEVENT in RRULE mode, got %d", eventCount)
	}

	if !bytes.Contains(data, []byte("RRULE:FREQ=WEEKLY;BYDAY=MO,WE")) {
		t.Errorf("expected RRULE property in wire bytes, got:\n%s", data)
	}

	if !bytes.Contains(data, []byte("EXDATE;")) {
		t.Errorf("expected EXDATE property in wire bytes, got:\n%s", data)
	}

	if !bytes.Contains(data, []byte("RDATE;")) {
		t.Errorf("expected RDATE property in wire bytes, got:\n%s", data)
	}

	uidExpected := "01HYX7QVATM8PZK6J7X7PZK6J7@togather.foundation"
	if !bytes.Contains(data, []byte("UID:"+uidExpected)) {
		t.Errorf("expected UID %q in RRULE mode (no occurrence ID), got:\n%s", uidExpected, data)
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 parsed event, got %d", len(parsed.Events))
	}
	if parsed.Events[0].RRULE == "" {
		t.Error("expected RRULE in parsed event")
	}
}

func TestSerializeRRuleMode_DefaultFalse_IsWireIdenticalToPhase2(t *testing.T) {
	occurrences := []events.Occurrence{
		{
			ID:        "occ1",
			StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
			EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
		},
		{
			ID:        "occ2",
			StartTime: time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
			EndTime:   ptr(time.Date(2026, 4, 22, 21, 0, 0, 0, time.UTC)),
		},
	}

	evtWithRecurrence := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Weekly Meetup",
		Description: "Recurring weekly meetup.",
		Recurrence: &events.RecurrenceRule{
			RRule: "FREQ=WEEKLY;BYDAY=MO,WE",
		},
		Occurrences: occurrences,
	}

	evtWithoutRecurrence := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Weekly Meetup",
		Description: "Recurring weekly meetup.",
		Occurrences: occurrences,
	}

	resultWith, err := SerializeEvents([]events.Event{evtWithRecurrence}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents(with Recurrence, default opts) error = %v", err)
	}
	resultWithout, err := SerializeEvents([]events.Event{evtWithoutRecurrence}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents(without Recurrence, default opts) error = %v", err)
	}

	strippedWith := stripDTSTAMP(resultWith.Data)
	strippedWithout := stripDTSTAMP(resultWithout.Data)

	if !bytes.Equal(strippedWith, strippedWithout) {
		t.Errorf("default IncludeRRule=false should produce wire-identical output regardless of Recurrence field\ngot (with Recurrence):\n%s\nexpected (without Recurrence):\n%s", strippedWith, strippedWithout)
	}

	if bytes.Contains(resultWith.Data, []byte("RRULE:")) {
		t.Error("default IncludeRRule=false should not emit RRULE")
	}

	eventCount := bytes.Count(resultWith.Data, []byte("BEGIN:VEVENT"))
	if eventCount != 2 {
		t.Errorf("expected 2 VEVENTs in flattened mode, got %d", eventCount)
	}
}

func TestSerializeRRuleMode_NonRecurringEvent(t *testing.T) {
	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "One-off Event",
		Description: "A non-recurring event.",
		Occurrences: []events.Occurrence{
			{
				ID:        "occ1",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{
		IncludeRRule: true,
	})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	if bytes.Contains(result.Data, []byte("RRULE:")) {
		t.Error("non-recurring event in RRULE mode should not emit RRULE")
	}

	eventCount := bytes.Count(result.Data, []byte("BEGIN:VEVENT"))
	if eventCount != 1 {
		t.Errorf("expected 1 VEVENT, got %d", eventCount)
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 parsed event, got %d", len(parsed.Events))
	}
}

func TestSerializeRRuleMode_RoundTripValidation(t *testing.T) {
	recurrence := &events.RecurrenceRule{
		RRule:   "FREQ=WEEKLY;BYDAY=TU",
		ExDates: nil,
		RDates:  nil,
		TZID:    "",
	}

	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Roundtrip RRULE Event",
		Description: "Validates RRULE round-trip through parse.",
		Recurrence:  recurrence,
		Occurrences: []events.Occurrence{
			{
				ID:        "occ1",
				StartTime: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{
		IncludeRRule: true,
	})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 parsed event, got %d", len(parsed.Events))
	}

	if parsed.Events[0].RRULE != "FREQ=WEEKLY;BYDAY=TU" {
		t.Errorf("expected RRULE 'FREQ=WEEKLY;BYDAY=TU', got %q", parsed.Events[0].RRULE)
	}

	if parsed.Events[0].Summary != "Roundtrip RRULE Event" {
		t.Errorf("expected summary 'Roundtrip RRULE Event', got %q", parsed.Events[0].Summary)
	}
}

func TestExportFixturesRRule(t *testing.T) {
	recurrence := &events.RecurrenceRule{
		RRule: "FREQ=WEEKLY;BYDAY=MO,WE",
		ExDates: []time.Time{
			time.Date(2026, 5, 4, 18, 0, 0, 0, time.UTC),
		},
		RDates: []time.Time{
			time.Date(2026, 6, 1, 19, 0, 0, 0, time.UTC),
		},
		TZID: "America/Toronto",
	}

	venueName := "Community Center"
	evt := events.Event{
		ULID:             "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:             "Weekly Meetup",
		Description:      "A recurring community meetup with exceptions.",
		Recurrence:       recurrence,
		PrimaryVenueName: &venueName,
		Occurrences: []events.Occurrence{
			{
				ID:        "occ1",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
				VenueULID: ptr("01HYX7QVATM8PZK6J7X7PZK6J7"),
			},
		},
	}

	fixturePath := "../../tests/testdata/ics/export-recurring-rrule.ics"

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{
		CalendarName: "Test Calendar",
		IncludeRRule: true,
	})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 parsed event from RRULE fixture, got %d", len(parsed.Events))
	}

	if *update {
		if err := os.WriteFile(fixturePath, result.Data, 0644); err != nil {
			t.Fatalf("failed to update fixture: %v", err)
		}
		t.Logf("updated fixture: %s", fixturePath)
		return
	}

	expected, err := os.ReadFile(fixturePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(fixturePath, result.Data, 0644); err != nil {
				t.Fatalf("failed to create fixture: %v", err)
			}
			t.Logf("created fixture: %s", fixturePath)
			return
		}
		t.Fatalf("failed to read fixture: %v", err)
	}

	resultData := stripDTSTAMP(result.Data)
	expectedData := stripDTSTAMP(expected)

	if !bytes.Equal(resultData, expectedData) {
		t.Errorf("fixture mismatch\ngot:\n%s\nexpected:\n%s", resultData, expectedData)
	}
}
