package events

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/config"
	"github.com/rs/zerolog"
)

// The all-day invariant: is_all_day = true if and only if the row's start_time
// is local midnight in the row's own zone. These tests pin the single
// normalisation helper (`NormalizeOccurrenceAllDay`) and the two admin writers
// that used to set the marker and the instant independently.

func toronto(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load America/Toronto: %v", err)
	}
	return loc
}

func TestNormalizeOccurrenceAllDay(t *testing.T) {
	loc := toronto(t)
	end := time.Date(2026, 11, 5, 19, 0, 0, 0, loc)

	t.Run("timed instant is returned verbatim with the marker clear", func(t *testing.T) {
		start := time.Date(2026, 11, 5, 19, 0, 0, 0, loc)
		gotStart, gotEnd, err := NormalizeOccurrenceAllDay(start, &end, false, loc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !gotStart.Equal(start) {
			t.Errorf("start changed: got %s want %s", gotStart, start)
		}
		if gotEnd == nil || !gotEnd.Equal(end) {
			t.Errorf("end changed: got %v want %s", gotEnd, end)
		}
	})

	t.Run("all-day anchors a timed instant to local midnight in the row zone", func(t *testing.T) {
		start := time.Date(2026, 11, 5, 19, 0, 0, 0, loc)
		gotStart, gotEnd, err := NormalizeOccurrenceAllDay(start, &end, true, loc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantStart := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		if !gotStart.Equal(wantStart) {
			t.Errorf("start = %s, want %s", gotStart.Format(time.RFC3339), wantStart.Format(time.RFC3339))
		}
		wantEnd := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		if gotEnd == nil || !gotEnd.Equal(wantEnd) {
			t.Errorf("end = %v, want %s", gotEnd, wantEnd.Format(time.RFC3339))
		}
		if !isLocalMidnight(gotStart, loc) {
			t.Errorf("anchored start %s is not local midnight in %s", gotStart, loc)
		}
	})

	t.Run("the civil date is the date in the occurrence's own zone", func(t *testing.T) {
		// 2026-11-06T02:00:00Z is 2026-11-05 22:00 in Toronto. The civil date is
		// the one in the occurrence's own (target) zone, so the 5th — not the
		// 6th the value's own UTC offset would suggest — is the date anchored.
		start := time.Date(2026, 11, 6, 2, 0, 0, 0, time.UTC)
		gotStart, _, err := NormalizeOccurrenceAllDay(start, nil, true, loc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		if !gotStart.Equal(want) {
			t.Errorf("start = %s, want %s", gotStart, want)
		}
	})

	t.Run("bare date anchoring matches ingest for an ambiguous zone", func(t *testing.T) {
		// Same rule as parseAllDayDate with a YYYY-MM-DD input: midnight in the
		// target zone.
		start := time.Date(2026, 3, 8, 12, 30, 0, 0, loc) // DST spring-forward day
		gotStart, _, err := NormalizeOccurrenceAllDay(start, nil, true, loc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotStart.Hour() != 0 || gotStart.Minute() != 0 || gotStart.Day() != 8 {
			t.Errorf("start = %s, want local midnight on the 8th", gotStart)
		}
	})
}

// TestAnchorAllDayInstantPositiveOffset pins the F1 fix for a positive-offset
// zone: a correctly-anchored Paris all-day row read back from the DB carries
// the server's local zone (pgx ScanLocation nil → time.Unix → time.Local), not
// Paris. The civil date must come from the occurrence's own zone (Paris), so
// the 6th survives a DB round-trip — not the 5th the value's own UTC offset
// would produce.
func TestAnchorAllDayInstantPositiveOffset(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("load Europe/Paris: %v", err)
	}

	midnightParis := time.Date(2026, 11, 6, 0, 0, 0, 0, paris)
	dbValue := midnightParis.UTC() // what a pgx read yields (location UTC)

	got, err := anchorAllDayInstant(dbValue, paris)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(midnightParis) {
		t.Errorf("anchor(%s, Europe/Paris) = %s, want %s (the civil date must survive a DB round-trip)",
			dbValue.Format(time.RFC3339), got.Format(time.RFC3339), midnightParis.Format(time.RFC3339))
	}

	// nil loc is treated as UTC.
	gotUTC, err := anchorAllDayInstant(dbValue, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if y, m, d := gotUTC.Date(); y != 2026 || m != time.November || d != 5 {
		t.Errorf("anchor(nil loc → UTC) date = %04d-%02d-%02d, want 2026-11-05", y, m, d)
	}
}

// TestUpdateOccurrenceOnEvent_TzChangePositiveOffsetPreservesCivilDate pins the
// F1 fix end to end for a timezone change: a Paris all-day row read back with
// the server's local (UTC) location must keep its civil date when the zone
// changes, re-anchored to local midnight in the new zone.
func TestUpdateOccurrenceOnEvent_TzChangePositiveOffsetPreservesCivilDate(t *testing.T) {
	ctx := context.Background()
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("load Europe/Paris: %v", err)
	}
	vancouver, err := time.LoadLocation("America/Vancouver")
	if err != nil {
		t.Fatalf("load America/Vancouver: %v", err)
	}

	// midnight Paris Nov 6, as the DB would read it back (location UTC).
	dbStart := time.Date(2026, 11, 6, 0, 0, 0, 0, paris).UTC()

	var captured OccurrenceUpdateParams
	venueID := "11111111-1111-1111-1111-111111111111"
	repo := &mockTransactionalRepo{}
	repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
		return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
	}
	repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
		return &Occurrence{ID: "occ-uuid", StartTime: dbStart, Timezone: "Europe/Paris", IsAllDay: true}, nil
	}
	repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
		captured = params
		return &Occurrence{ID: "occ-uuid"}, nil
	}

	zone := "America/Vancouver"
	service := newAdminServiceForOccurrenceTest(repo)
	if _, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{Timezone: &zone}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.StartTime == nil {
		t.Fatal("the re-anchored instant must be written on a zone change")
	}
	if !isLocalMidnight(*captured.StartTime, vancouver) {
		t.Errorf("stored start %s must be local midnight in the new zone", captured.StartTime.Format(time.RFC3339))
	}
	if y, m, d := captured.StartTime.In(vancouver).Date(); y != 2026 || m != time.November || d != 6 {
		t.Errorf("civil date = %04d-%02d-%02d, want 2026-11-06", y, m, d)
	}
}

func TestIsLocalMidnight(t *testing.T) {
	loc := toronto(t)
	if !isLocalMidnight(time.Date(2026, 11, 5, 0, 0, 0, 0, loc), loc) {
		t.Error("local midnight must be recognised")
	}
	if isLocalMidnight(time.Date(2026, 11, 5, 19, 0, 0, 0, loc), loc) {
		t.Error("19:00 must not be recognised as midnight")
	}
	if isLocalMidnight(time.Date(2026, 11, 5, 0, 0, 1, 0, loc), loc) {
		t.Error("one second past midnight must not be recognised")
	}
	// Midnight UTC on a Toronto day is 19:00/20:00 local — not local midnight.
	if isLocalMidnight(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC), loc) {
		t.Error("midnight UTC is not midnight in the occurrence zone")
	}
}

// newAdminServiceForOccurrenceTest wires an AdminService against the
// transactional mock used across this package.
func newAdminServiceForOccurrenceTest(repo Repository) *AdminService {
	return NewAdminService(repo, false, "America/Toronto",
		config.ValidationConfig{MaxEventNameLength: 500},
		"https://toronto.togather.foundation", zerolog.Nop())
}

// TestUpdateOccurrenceOnEvent_AllDayInvariant pins the admin occurrence PUT:
// all_day:true with a timed start_time must store local midnight in the
// occurrence's own zone (never a timed instant carrying the marker), so the
// date-only rendering agrees with the stored instant.
func TestUpdateOccurrenceOnEvent_AllDayInvariant(t *testing.T) {
	ctx := context.Background()
	loc := toronto(t)
	timedStart := time.Date(2026, 11, 5, 19, 0, 0, 0, loc)
	timedEnd := time.Date(2026, 11, 5, 21, 0, 0, 0, loc)
	// The occurrence must resolve a location (venue or virtual URL); the event's
	// primary venue is inherited when the occurrence has none.
	venueID := "11111111-1111-1111-1111-111111111111"

	t.Run("all_day true anchors a timed start to local midnight", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: timedStart, EndTime: &timedEnd,
				Timezone: "America/Toronto", IsAllDay: false,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid", StartTime: time.Time{}}, nil
		}

		allDay := true
		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{
			StartTime: &timedStart,
			IsAllDay:  &allDay,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if captured.IsAllDay == nil || !*captured.IsAllDay {
			t.Fatalf("marker: got %v, want true", captured.IsAllDay)
		}
		if captured.StartTime == nil || !isLocalMidnight(*captured.StartTime, loc) {
			t.Fatalf("stored start %v is not local midnight in the occurrence zone", captured.StartTime)
		}
		if want := time.Date(2026, 11, 5, 0, 0, 0, 0, loc); !captured.StartTime.Equal(want) {
			t.Errorf("stored start = %s, want %s", captured.StartTime, want)
		}
		// The end must be re-anchored too, otherwise the row's window still
		// disagrees with its date-only rendering.
		if captured.EndTime == nil || !isLocalMidnight(*captured.EndTime, loc) {
			t.Errorf("stored end %v is not local midnight in the occurrence zone", captured.EndTime)
		}
	})

	t.Run("all_day true with no start_time anchors the existing instant", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{ID: "occ-uuid", StartTime: timedStart, Timezone: "America/Toronto"}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		allDay := true
		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{IsAllDay: &allDay})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.StartTime == nil || !isLocalMidnight(*captured.StartTime, loc) {
			t.Fatalf("stored start %v must be local midnight once the marker is set", captured.StartTime)
		}
	})

	t.Run("retiming off midnight clears a stale marker", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: time.Date(2026, 11, 5, 0, 0, 0, 0, loc),
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{StartTime: &timedStart})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.IsAllDay == nil || *captured.IsAllDay {
			t.Fatalf("marker must be cleared when the row is retimed off local midnight, got %v", captured.IsAllDay)
		}
	})

	t.Run("timed row keeps its instant and marker when untouched", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{ID: "occ-uuid", StartTime: timedStart, Timezone: "America/Toronto"}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{StartTime: &timedStart})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.IsAllDay != nil {
			t.Errorf("a plain retime must not touch the marker, got %v", *captured.IsAllDay)
		}
		if captured.StartTime == nil || !captured.StartTime.Equal(timedStart) {
			t.Errorf("timed instant must be preserved verbatim, got %v", captured.StartTime)
		}
	})

	t.Run("changing the timezone of a date-only row keeps its civil date", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		vancouver, err := time.LoadLocation("America/Vancouver")
		if err != nil {
			t.Fatalf("load America/Vancouver: %v", err)
		}
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: time.Date(2026, 11, 5, 0, 0, 0, 0, loc),
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		zone := "America/Vancouver"
		service := newAdminServiceForOccurrenceTest(repo)
		_, err = service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{Timezone: &zone})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.StartTime == nil {
			t.Fatal("the anchored instant must be rewritten when the zone changes")
		}
		if !isLocalMidnight(*captured.StartTime, vancouver) {
			t.Errorf("stored start %s must be local midnight in the new zone", captured.StartTime.Format(time.RFC3339))
		}
		if y, m, d := captured.StartTime.In(vancouver).Date(); y != 2026 || m != time.November || d != 5 {
			t.Errorf("the civil date must survive the zone change, got %04d-%02d-%02d", y, m, d)
		}
		if captured.IsAllDay != nil && !*captured.IsAllDay {
			t.Error("a date-only row must not be demoted to timed by a zone change")
		}
	})

	t.Run("an unrelated patch writes neither the marker nor the instants", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		ticket := "https://example.com/tickets"
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: time.Date(2026, 11, 5, 0, 0, 0, 0, loc),
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		ticketSet := true
		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{
			TicketURL:    &ticket,
			TicketURLSet: ticketSet,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.StartTime != nil || captured.EndTimeSet {
			t.Errorf("an unrelated patch must not rewrite the occurrence window (%v / %v)", captured.StartTime, captured.EndTimeSet)
		}
		if captured.IsAllDay != nil {
			t.Errorf("an unrelated patch must not rewrite the marker, got %v", *captured.IsAllDay)
		}
	})
}
