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
		gotStart, gotEnd, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: loc}, start, &end, false, loc.String())
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
		gotStart, gotEnd, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: loc}, start, &end, true, loc.String())
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

	t.Run("the civil date is the date the value carries", func(t *testing.T) {
		// 2026-11-06T02:00:00Z carries the 6th. That is the date ingestion takes
		// for the same startDate, so the admin writers must take it too, anchored
		// to midnight in the occurrence's zone.
		start := time.Date(2026, 11, 6, 2, 0, 0, 0, time.UTC)
		gotStart, _, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: loc}, start, nil, true, loc.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
		if !gotStart.Equal(want) {
			t.Errorf("start = %s, want %s", gotStart, want)
		}
	})

	t.Run("bare date anchoring matches ingest for an ambiguous zone", func(t *testing.T) {
		// Same rule as parseAllDayCivilDate with a YYYY-MM-DD input: the carried date
		// target zone.
		start := time.Date(2026, 3, 8, 12, 30, 0, 0, loc) // DST spring-forward day
		gotStart, _, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: loc}, start, nil, true, loc.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotStart.Hour() != 0 || gotStart.Minute() != 0 || gotStart.Day() != 8 {
			t.Errorf("start = %s, want local midnight on the 8th", gotStart)
		}
	})
}

// TestAllDayDateRuleBothEntryPoints pins that ingestion (parseAllDayCivilDate) and the
// admin writers (NormalizeOccurrenceAllDay) anchor the same wire value to the
// same instant, including values whose own offset differs from the
// occurrence's zone.
func TestAllDayDateRuleBothEntryPoints(t *testing.T) {
	loc := toronto(t)
	cases := []struct {
		name string
		raw  string
		want string // civil date
	}{
		{"UTC value that is the previous evening in the zone", "2026-11-06T02:00:00Z", "2026-11-06"},
		{"already midnight in the zone", "2026-11-06T00:00:00-05:00", "2026-11-06"},
		{"midnight east of the zone", "2026-03-05T00:00:00+01:00", "2026-03-05"},
		{"bare date", "2026-11-06", "2026-11-06"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ingestDate, err := parseAllDayCivilDate(tc.raw)
			if err != nil {
				t.Fatalf("parseAllDayCivilDate: %v", err)
			}
			ingest, _, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: loc}, ingestDate, nil, true, loc.String())
			if err != nil {
				t.Fatalf("NormalizeOccurrenceAllDay: %v", err)
			}
			parsed := ingestDate
			if len(tc.raw) > len("2006-01-02") {
				if parsed, err = time.Parse(time.RFC3339, tc.raw); err != nil {
					t.Fatalf("parse %q: %v", tc.raw, err)
				}
			}
			admin, _, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: loc}, parsed, nil, true, loc.String())
			if err != nil {
				t.Fatalf("NormalizeOccurrenceAllDay: %v", err)
			}
			if !ingest.Equal(admin) {
				t.Errorf("entry points disagree on %q: ingest %s, admin %s", tc.raw, ingest.Format(time.RFC3339), admin.Format(time.RFC3339))
			}
			if got := admin.In(loc).Format(time.DateOnly); got != tc.want {
				t.Errorf("civil date = %s, want %s", got, tc.want)
			}
			if !isLocalMidnight(admin, loc) {
				t.Errorf("anchored %s is not local midnight in %s", admin, loc)
			}
		})
	}

	t.Run("row read keeps the row's civil date", func(t *testing.T) {
		paris, err := time.LoadLocation("Europe/Paris")
		if err != nil {
			t.Fatalf("load Europe/Paris: %v", err)
		}
		// The database says the row's civil date is the 5th. The API must use
		// that date, not one re-derived from the instant in the host's tzdata.
		dbDate := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
		got, _, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: paris}, dbDate, nil, true, paris.String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := time.Date(2026, 3, 5, 0, 0, 0, 0, paris); !got.Equal(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	})
}

// TestNormalizeOccurrenceAllDayUsesAnchorerAuthority pins the single-authority
// rule: the instant comes from the anchorer (Postgres in production), not from
// converting the civil date with the Go host's tzdata. The fake anchorer claims
// a -08:00 offset for America/Vancouver while a host on the newer tzdata says
// -07:00; the anchored instant must follow the authority.
func TestNormalizeOccurrenceAllDayUsesAnchorerAuthority(t *testing.T) {
	carried := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)

	// Host tzdata shape (BC permanent time, -07:00) vs database shape (-08:00).
	hostShape := time.FixedZone("America/Vancouver", -7*3600)
	dbShape := time.FixedZone("America/Vancouver", -8*3600)

	got, _, err := NormalizeOccurrenceAllDay(context.Background(), fakeAllDayAnchorer{loc: dbShape}, carried, nil, true, "America/Vancouver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2027, 1, 15, 0, 0, 0, 0, dbShape)
	if !got.Equal(want) {
		t.Errorf("anchor = %s, want the authority's %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if hostAnchor := time.Date(2027, 1, 15, 0, 0, 0, 0, hostShape); got.Equal(hostAnchor) {
		t.Errorf("anchor = %s followed the host tzdata; it must follow the anchoring authority", got.Format(time.RFC3339))
	}
	if !isLocalMidnight(got, dbShape) {
		t.Errorf("anchored %s is not local midnight in the authority's zone", got.Format(time.RFC3339))
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
		return &Occurrence{
			ID: "occ-uuid", StartTime: dbStart, Timezone: "Europe/Paris", IsAllDay: true,
			LocalDate: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
		}, nil
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

	t.Run("retiming only the end off midnight clears the stale marker", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		midnight := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		timedEnd := time.Date(2026, 11, 5, 21, 0, 0, 0, loc)
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: midnight, EndTime: &midnight,
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{
			EndTime: &timedEnd, EndTimeSet: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.IsAllDay == nil || *captured.IsAllDay {
			t.Fatalf("retiming only the end off midnight must clear the marker, got %v", captured.IsAllDay)
		}
		if !captured.EndTimeSet || captured.EndTime == nil || !captured.EndTime.Equal(timedEnd) {
			t.Fatalf("the requested end must be preserved verbatim, got set=%v end=%v", captured.EndTimeSet, captured.EndTime)
		}
	})

	// The two subtests below are the guards for the end-only condition itself.
	// They pass on the pre-widening revision too (the end was ignored entirely
	// there): they exist to fail if the condition is widened past a retime —
	// dropping the local-midnight test, or the nil test, over-clears the marker.
	t.Run("an end-only retime onto another civil midnight keeps the marker", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		midnight := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		nextMidnight := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: midnight, EndTime: &midnight,
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{
			EndTime: &nextMidnight, EndTimeSet: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The requested end is still local midnight (of another civil date), so
		// the row is still date-only. No marker write is the correct outcome:
		// the row's own is_all_day = true stands.
		if captured.IsAllDay != nil {
			t.Fatalf("an end that stays on local midnight must not rewrite the marker, got %v", *captured.IsAllDay)
		}
		if !captured.EndTimeSet || captured.EndTime == nil || !captured.EndTime.Equal(nextMidnight) {
			t.Errorf("the end must stay local midnight of its civil date, got set=%v end=%v", captured.EndTimeSet, captured.EndTime)
		}
		if captured.StartTime != nil {
			t.Errorf("an end-only edit must not rewrite the start, got %v", captured.StartTime)
		}
	})

	t.Run("clearing the end of a date-only row keeps the marker", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		midnight := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: midnight, EndTime: &midnight,
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		// EndTimeSet with a nil instant is what the handler produces for a
		// literal end_time: null. Clearing the end is not a retime: a date-only
		// occurrence may simply have no end.
		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{
			EndTimeSet: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.IsAllDay != nil {
			t.Fatalf("clearing the end must not rewrite the marker, got %v", *captured.IsAllDay)
		}
		if !captured.EndTimeSet || captured.EndTime != nil {
			t.Errorf("end_time must be cleared, got set=%v end=%v", captured.EndTimeSet, captured.EndTime)
		}
	})

	t.Run("all_day true with only a timed end anchors it to midnight", func(t *testing.T) {
		var captured OccurrenceUpdateParams
		midnight := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
		timedEnd := time.Date(2026, 11, 5, 21, 0, 0, 0, loc)
		repo := &mockTransactionalRepo{}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) {
			return &Event{ID: "event-uuid", ULID: "01HEVENT000000000000000001", LifecycleState: "draft", PrimaryVenueID: &venueID}, nil
		}
		repo.getOccurrenceByIDFunc = func(_ context.Context, _ string, _ string) (*Occurrence, error) {
			return &Occurrence{
				ID: "occ-uuid", StartTime: midnight, EndTime: &midnight,
				Timezone: "America/Toronto", IsAllDay: true,
			}, nil
		}
		repo.updateOccurrenceFunc = func(_ context.Context, _, _ string, params OccurrenceUpdateParams) (*Occurrence, error) {
			captured = params
			return &Occurrence{ID: "occ-uuid"}, nil
		}

		allDay := true
		service := newAdminServiceForOccurrenceTest(repo)
		_, err := service.UpdateOccurrenceOnEvent(ctx, "01HEVENT000000000000000001", "occ-uuid", OccurrenceUpdateParams{
			IsAllDay: &allDay, EndTime: &timedEnd, EndTimeSet: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.IsAllDay == nil || !*captured.IsAllDay {
			t.Fatalf("an explicit all_day:true must keep the marker, got %v", captured.IsAllDay)
		}
		if captured.EndTime == nil || !isLocalMidnight(*captured.EndTime, loc) {
			t.Fatalf("all_day:true must re-anchor the timed end to local midnight, got %v", captured.EndTime)
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
