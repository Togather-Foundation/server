package ical

import (
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
)

var uidLineRe = regexp.MustCompile(`(?m)^UID:(.+)$`)

func extractUIDs(data []byte) []string {
	matches := uidLineRe.FindAllSubmatch(data, -1)
	uids := make([]string, len(matches))
	for i, m := range matches {
		uids[i] = string(m[1])
	}
	sort.Strings(uids)
	return uids
}

func TestUIDStability_FlattenedMode_SameInputProducesSameUIDs(t *testing.T) {
	evts := []events.Event{
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
		{
			ULID: "01HZZZZZZZZZZZZZZZZZZZZZZZ",
			Name: "One-off Event",
			Occurrences: []events.Occurrence{
				{
					ID:        "01HZZZZZZZZZZZZZZZZZZZZZZZ1",
					StartTime: time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	result1, err := SerializeEvents(evts, SerializeOptions{})
	if err != nil {
		t.Fatalf("first serialization error = %v", err)
	}
	result2, err := SerializeEvents(evts, SerializeOptions{})
	if err != nil {
		t.Fatalf("second serialization error = %v", err)
	}

	uids1 := extractUIDs(result1.Data)
	uids2 := extractUIDs(result2.Data)

	if len(uids1) == 0 {
		t.Fatal("expected at least one UID in first result")
	}
	if len(uids2) == 0 {
		t.Fatal("expected at least one UID in second result")
	}

	if len(uids1) != len(uids2) {
		t.Fatalf("UID count mismatch: first=%d, second=%d", len(uids1), len(uids2))
	}

	for i := range uids1 {
		if uids1[i] != uids2[i] {
			t.Errorf("UID mismatch at index %d: first=%q, second=%q", i, uids1[i], uids2[i])
		}
	}
}

func TestUIDStability_RRULEMode_SameInputProducesSameUIDs(t *testing.T) {
	evts := []events.Event{
		{
			ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
			Name: "RRULE Meetup",
			Recurrence: &events.RecurrenceRule{
				RRule: "FREQ=WEEKLY;BYDAY=MO,WE",
				TZID:  "America/Toronto",
			},
			Occurrences: []events.Occurrence{
				{
					ID:        "occ1",
					StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	result1, err := SerializeEvents(evts, SerializeOptions{IncludeRRule: true})
	if err != nil {
		t.Fatalf("first serialization error = %v", err)
	}
	result2, err := SerializeEvents(evts, SerializeOptions{IncludeRRule: true})
	if err != nil {
		t.Fatalf("second serialization error = %v", err)
	}

	uids1 := extractUIDs(result1.Data)
	uids2 := extractUIDs(result2.Data)

	if len(uids1) != 1 {
		t.Fatalf("expected 1 UID in RRULE mode, got %d", len(uids1))
	}
	if len(uids2) != 1 {
		t.Fatalf("expected 1 UID in RRULE mode, got %d", len(uids2))
	}

	if uids1[0] != uids2[0] {
		t.Errorf("UID mismatch: first=%q, second=%q", uids1[0], uids2[0])
	}
}

func TestUIDStability_FlattenedMode_UIDFormat(t *testing.T) {
	t.Run("with occurrence ID", func(t *testing.T) {
		evts := []events.Event{
			{
				ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
				Name: "Format Test",
				Occurrences: []events.Occurrence{
					{
						ID:        "occ1",
						StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
					},
				},
			},
		}

		result, err := SerializeEvents(evts, SerializeOptions{})
		if err != nil {
			t.Fatalf("SerializeEvents() error = %v", err)
		}

		uids := extractUIDs(result.Data)
		if len(uids) != 1 {
			t.Fatalf("expected 1 UID, got %d", len(uids))
		}

		expected := "01HYX7QVATM8PZK6J7X7PZK6J7-occ1@togather.foundation"
		if uids[0] != expected {
			t.Errorf("expected UID %q, got %q", expected, uids[0])
		}
	})

	t.Run("without occurrence ID", func(t *testing.T) {
		evts := []events.Event{
			{
				ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
				Name: "Format Test",
				Occurrences: []events.Occurrence{
					{
						ID:        "",
						StartTime: time.Date(2026, 4, 15, 19, 0, 0, 0, time.UTC),
					},
				},
			},
		}

		result, err := SerializeEvents(evts, SerializeOptions{})
		if err != nil {
			t.Fatalf("SerializeEvents() error = %v", err)
		}

		uids := extractUIDs(result.Data)
		if len(uids) != 1 {
			t.Fatalf("expected 1 UID, got %d", len(uids))
		}

		expected := "01HYX7QVATM8PZK6J7X7PZK6J7@togather.foundation"
		if uids[0] != expected {
			t.Errorf("expected UID %q, got %q", expected, uids[0])
		}
	})
}

func TestUIDStability_RRULEMode_UIDFormat(t *testing.T) {
	evts := []events.Event{
		{
			ULID: "01HYX7QVATM8PZK6J7X7PZK6J7",
			Name: "RRULE Format Test",
			Recurrence: &events.RecurrenceRule{
				RRule: "FREQ=WEEKLY;BYDAY=TU",
				TZID:  "America/Toronto",
			},
			Occurrences: []events.Occurrence{
				{
					ID:        "occ1",
					StartTime: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	result, err := SerializeEvents(evts, SerializeOptions{IncludeRRule: true})
	if err != nil {
		t.Fatalf("SerializeEvents() error = %v", err)
	}

	uids := extractUIDs(result.Data)
	if len(uids) != 1 {
		t.Fatalf("expected 1 UID in RRULE mode, got %d", len(uids))
	}

	expected := "01HYX7QVATM8PZK6J7X7PZK6J7@togather.foundation"
	if uids[0] != expected {
		t.Errorf("expected UID %q (no occurrence suffix in RRULE mode), got %q", expected, uids[0])
	}
}
