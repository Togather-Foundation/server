package ical

// This test file covers the ICS compatibility matrix defined in
// docs/integration/ics-compatibility-matrix.md
//
// Matrix targets:
// - Strict ICS parser (arran4/golang-ical) - validates structural correctness
// - Apple Calendar - validates DTEND absence, proper escaping
// - Google Calendar - validates UTC timestamps, UID format

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

func TestCompatMatrix_RequiredProperties(t *testing.T) {
	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Required Props Test",
		Description: "Test description",
		Occurrences: []events.Occurrence{
			{
				ID:        "",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	data := string(result.Data)

	check := func(substr, name string) {
		if !strings.Contains(data, substr) {
			t.Errorf("expected %s in output", name)
		}
	}

	check("PRODID:-//Togather//Togather Events//EN", "PRODID")
	check("VERSION:2.0", "VERSION")
	check("CALSCALE:GREGORIAN", "CALSCALE")
	check("METHOD:PUBLISH", "METHOD")

	check("UID:01HYX7QVATM8PZK6J7X7PZK6J7@togather.foundation", "UID")
	check("DTSTART:", "DTSTART")
	check("SUMMARY:", "SUMMARY")
}

func TestCompatMatrix_UTCTimestamps(t *testing.T) {
	evt := events.Event{
		ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name: "UTC Timestamp Test",
		Occurrences: []events.Occurrence{
			{
				ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
				StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	data := string(result.Data)

	if !strings.HasSuffix(strings.Split(data, "DTSTART:")[1][:16], "Z") {
		t.Error("DTSTART must end with Z (UTC)")
	}

	if !strings.HasSuffix(strings.Split(data, "DTEND:")[1][:16], "Z") {
		t.Error("DTEND must end with Z (UTC)")
	}

	if strings.Contains(data, "TZID") {
		t.Error("UTC timestamps should not use TZID")
	}
}

func TestCompatMatrix_UIDFormat(t *testing.T) {
	tests := []struct {
		name         string
		eventULID    string
		occID        string
		wantUIDBase  string
		wantMultiOcc bool
	}{
		{
			name:         "single occurrence",
			eventULID:    "01HYX7QVATM8PZK6J7X7PZK6J7",
			occID:        "",
			wantUIDBase:  "01HYX7QVATM8PZK6J7X7PZK6J7@togather.foundation",
			wantMultiOcc: false,
		},
		{
			name:         "multi occurrence",
			eventULID:    "01HYX7QVATM8PZK6J7X7PZK6J7",
			occID:        "occ1",
			wantUIDBase:  "01HYX7QVATM8PZK6J7X7PZK6J7-occ1@togather.foundation",
			wantMultiOcc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evts := []events.Event{
				{
					ULID: tt.eventULID,
					Name: "UID Format Test",
					Occurrences: []events.Occurrence{
						{
							ID:        tt.occID,
							StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
						},
					},
				},
			}

			if tt.wantMultiOcc {
				evts[0].Occurrences = append(evts[0].Occurrences, events.Occurrence{
					ID:        "occ2",
					StartTime: time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
				})
			}

			result, err := SerializeEvents(evts, SerializeOptions{})
			if err != nil {
				t.Fatalf("SerializeEvents() error = %v", err)
			}

			data := string(result.Data)
			if !strings.Contains(data, tt.wantUIDBase) {
				t.Errorf("expected UID %q in output", tt.wantUIDBase)
			}
		})
	}
}

func TestCompatMatrix_ICSEscaping(t *testing.T) {
	t.Run("roundtrip with escaping", func(t *testing.T) {
		desc := "Test, with; special \\ chars\nand newlines"
		evt := events.Event{
			ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
			Name:        "Roundtrip Test",
			Description: desc,
			Occurrences: []events.Occurrence{
				{
					ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
					StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				},
			},
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
			t.Fatalf("expected 1 event, got %d", len(parsed.Events))
		}

		if parsed.Events[0].Summary != "Roundtrip Test" {
			t.Errorf("expected summary 'Roundtrip Test', got %q", parsed.Events[0].Summary)
		}
	})
}

func TestCompatMatrix_OptionalDTEND(t *testing.T) {
	evt := events.Event{
		ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name: "No DTEND Test",
		Occurrences: []events.Occurrence{
			{
				ID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
				StartTime: time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	data := string(result.Data)

	if strings.Contains(data, "DTEND:") {
		t.Error("DTEND must be absent when EndTime is nil (Apple Calendar compatibility)")
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(parsed.Events))
	}
}

func TestCompatMatrix_EmptyCalendar(t *testing.T) {
	result, err := SerializeEvents([]events.Event{}, SerializeOptions{
		CalendarName: "Empty Test",
	})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	data := string(result.Data)

	if !strings.Contains(data, "BEGIN:VCALENDAR") {
		t.Error("expected VCALENDAR wrapper")
	}
	if !strings.Contains(data, "END:VCALENDAR") {
		t.Error("expected VCALENDAR end")
	}
	if !strings.Contains(data, "VERSION:2.0") {
		t.Error("expected VERSION")
	}
	if !strings.Contains(data, "PRODID:") {
		t.Error("expected PRODID")
	}

	eventCount := strings.Count(data, "BEGIN:VEVENT")
	if eventCount != 0 {
		t.Errorf("expected 0 VEVENTs, got %d", eventCount)
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Events) != 0 {
		t.Errorf("expected 0 parsed events, got %d", len(parsed.Events))
	}
}

func TestCompatMatrix_MultipleVEVENTs(t *testing.T) {
	evt := events.Event{
		ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name: "Three Occurrences",
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
			{
				ID:        "occ3",
				StartTime: time.Date(2026, 4, 29, 19, 0, 0, 0, time.UTC),
				EndTime:   ptr(time.Date(2026, 4, 29, 21, 0, 0, 0, time.UTC)),
			},
		},
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	data := string(result.Data)

	eventCount := strings.Count(data, "BEGIN:VEVENT")
	if eventCount != 3 {
		t.Errorf("expected 3 VEVENTs, got %d", eventCount)
	}

	if !strings.Contains(data, "01HYX7QVATM8PZK6J7X7PZK6J7-occ1@togather.foundation") {
		t.Error("expected occurrence 1 UID")
	}
	if !strings.Contains(data, "01HYX7QVATM8PZK6J7X7PZK6J7-occ2@togather.foundation") {
		t.Error("expected occurrence 2 UID")
	}
	if !strings.Contains(data, "01HYX7QVATM8PZK6J7X7PZK6J7-occ3@togather.foundation") {
		t.Error("expected occurrence 3 UID")
	}

	parsed, err := Parse(result.Data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Events) != 3 {
		t.Errorf("expected 3 parsed events, got %d", len(parsed.Events))
	}
}

func TestCompatMatrix_StrictParserRoundtrip(t *testing.T) {
	venueName := "Test Venue"
	evt := events.Event{
		ULID:        "01HYX7QVATM8PZK6J7X7PZK6J7",
		Name:        "Roundtrip Test Event",
		Description: "Test description; with special: chars\nand newlines",
		Occurrences: []events.Occurrence{
			{
				ID:         "01HYX7QVATM8PZK6J7X7PZK6J7",
				StartTime:  time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				EndTime:    ptr(time.Date(2026, 4, 15, 21, 0, 0, 0, time.UTC)),
				Timezone:   "America/New_York",
				VenueULID:  ptr("01HYX7QVATM8PZK6J7X7PZK6J7"),
				TicketURL:  "https://tickets.example.com/event",
				VirtualURL: ptr("https://stream.example.com"),
			},
		},
		PrimaryVenueName: &venueName,
	}

	result, err := SerializeEvents([]events.Event{evt}, SerializeOptions{
		CalendarName: "Test Calendar",
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

	ev := parsed.Events[0]

	if ev.Summary != "Roundtrip Test Event" {
		t.Errorf("expected summary, got %q", ev.Summary)
	}

	if !bytes.Contains(result.Data, []byte("BEGIN:VCALENDAR")) {
		t.Error("expected VCALENDAR wrapper")
	}
	if !bytes.Contains(result.Data, []byte("VERSION:2.0")) {
		t.Error("expected VERSION")
	}
	if !bytes.Contains(result.Data, []byte("PRODID:")) {
		t.Error("expected PRODID")
	}
	if !bytes.Contains(result.Data, []byte("CALSCALE:GREGORIAN")) {
		t.Error("expected CALSCALE")
	}
	if !bytes.Contains(result.Data, []byte("METHOD:PUBLISH")) {
		t.Error("expected METHOD")
	}
}
