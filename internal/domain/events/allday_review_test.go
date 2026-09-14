package events

import (
	"context"
	"testing"
	"time"
)

// TestFixAndApproveEventWithReview_DoesNotRewriteSiblingOccurrences pins the
// review fix/approve path against the collapse that motivated this work:
// approving used to rewrite EVERY occurrence row to occurrences[0]'s
// start/end plus the event-level all-day flag, turning a multi-date series into
// N identical rows and re-stamping genuinely timed rows as date-only.
func TestFixAndApproveEventWithReview_DoesNotRewriteSiblingOccurrences(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load America/Toronto: %v", err)
	}
	const eventULID = "01HEVENT000000000000000001"

	// A three-date series whose rows are all genuinely timed. The review entry
	// carries EventAllDay = true, which is what the event-level flag does for a
	// listing whose own startDate is a bare date (occurrences[0]) — exactly the
	// value that used to be smeared across all three rows.
	seriesEvent := func() *Event {
		return &Event{
			ID:   "event-uuid",
			ULID: eventULID,
			Name: "Three Night Run",
			Occurrences: []Occurrence{
				{ID: "occ-0", StartTime: time.Date(2026, 11, 1, 19, 0, 0, 0, loc), Timezone: "America/Toronto"},
				{ID: "occ-1", StartTime: time.Date(2026, 11, 2, 19, 0, 0, 0, loc), Timezone: "America/Toronto"},
				{ID: "occ-2", StartTime: time.Date(2026, 11, 3, 19, 0, 0, 0, loc), Timezone: "America/Toronto"},
			},
			LifecycleState: "draft",
		}
	}

	newRepo := func(event *Event) (*mockTransactionalRepo, *[]string, *[]string) {
		var wholeEventCalls []string
		var perRowCalls []string
		repo := &mockTransactionalRepo{}
		repo.lockReviewQueueEntryForUpdateFunc = func(_ context.Context, id int) (*ReviewQueueEntry, error) {
			return &ReviewQueueEntry{ID: id, EventULID: eventULID, Status: "pending", EventAllDay: true}, nil
		}
		repo.getByULIDFunc = func(_ context.Context, _ string) (*Event, error) { return event, nil }
		repo.updateOccurrenceDatesFunc = func(_ context.Context, _ string, _ time.Time, _ *time.Time, _ bool) error {
			wholeEventCalls = append(wholeEventCalls, "whole-event")
			return nil
		}
		repo.updateOccurrenceDatesByOccurrenceIDFunc = func(_ context.Context, _ string, occurrenceID string, _ time.Time, _ *time.Time, _ bool) error {
			perRowCalls = append(perRowCalls, occurrenceID)
			return nil
		}
		return repo, &wholeEventCalls, &perRowCalls
	}

	t.Run("approve with no corrections leaves every row alone", func(t *testing.T) {
		repo, wholeEventCalls, perRowCalls := newRepo(seriesEvent())
		service := newAdminServiceForOccurrenceTest(repo)

		if _, err := service.FixAndApproveEventWithReview(ctx, eventULID, 1, "admin", nil, nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(*wholeEventCalls) != 0 {
			t.Errorf("whole-event rewrite must not run: %v", *wholeEventCalls)
		}
		if len(*perRowCalls) != 0 {
			t.Errorf("no occurrence row may be written when no correction was asked for: %v", *perRowCalls)
		}
		if !repo.commitCalled {
			t.Error("approve must still commit")
		}
	})

	t.Run("a correction touches only the row the event dates mirror", func(t *testing.T) {
		repo, wholeEventCalls, perRowCalls := newRepo(seriesEvent())

		var capturedOccurrenceID string
		var capturedStart time.Time
		var capturedEnd *time.Time
		var capturedIsAllDay bool
		repo.updateOccurrenceDatesByOccurrenceIDFunc = func(_ context.Context, _ string, occurrenceID string, startTime time.Time, endTime *time.Time, isAllDay bool) error {
			capturedOccurrenceID = occurrenceID
			capturedStart = startTime
			capturedEnd = endTime
			capturedIsAllDay = isAllDay
			*perRowCalls = append(*perRowCalls, occurrenceID)
			return nil
		}

		// The admin corrects the event's own dates (what the review fix form
		// sends): a slightly later start on the first date.
		correctedStart := time.Date(2026, 11, 1, 20, 0, 0, 0, loc)
		correctedEnd := time.Date(2026, 11, 1, 22, 0, 0, 0, loc)

		service := newAdminServiceForOccurrenceTest(repo)
		if _, err := service.FixAndApproveEventWithReview(ctx, eventULID, 1, "admin", nil, &correctedStart, &correctedEnd); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(*wholeEventCalls) != 0 {
			t.Errorf("multi-occurrence corrections must not use the whole-event query: %v", *wholeEventCalls)
		}
		if len(*perRowCalls) != 1 || (*perRowCalls)[0] != "occ-0" {
			t.Fatalf("exactly occurrences[0] must be corrected, got %v", *perRowCalls)
		}
		if capturedOccurrenceID != "occ-0" {
			t.Errorf("occurrence id = %q, want occ-0", capturedOccurrenceID)
		}
		if !capturedStart.Equal(correctedStart) {
			t.Errorf("start = %s, want %s (a timed row keeps its instant)", capturedStart.Format(time.RFC3339), correctedStart.Format(time.RFC3339))
		}
		if capturedEnd == nil || !capturedEnd.Equal(correctedEnd) {
			t.Errorf("end = %v, want %s", capturedEnd, correctedEnd.Format(time.RFC3339))
		}
		if capturedIsAllDay {
			t.Error("the event-level all-day flag must not be stamped onto a timed row")
		}
	})

	t.Run("all-day series row keeps its marker and stays anchored", func(t *testing.T) {
		event := seriesEvent()
		event.Occurrences[0] = Occurrence{
			ID:        "occ-0",
			StartTime: time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			Timezone:  "America/Toronto",
			IsAllDay:  true,
		}
		repo, _, perRowCalls := newRepo(event)

		var capturedStart time.Time
		var capturedIsAllDay bool
		repo.updateOccurrenceDatesByOccurrenceIDFunc = func(_ context.Context, _ string, occurrenceID string, startTime time.Time, endTime *time.Time, isAllDay bool) error {
			capturedStart = startTime
			capturedIsAllDay = isAllDay
			*perRowCalls = append(*perRowCalls, occurrenceID)
			return nil
		}

		// Corrected to a timed value; because the row is all-day, it must be
		// re-anchored to local midnight rather than stored as a timed instant
		// carrying the marker.
		correctedStart := time.Date(2026, 11, 1, 15, 30, 0, 0, loc)

		service := newAdminServiceForOccurrenceTest(repo)
		if _, err := service.FixAndApproveEventWithReview(ctx, eventULID, 1, "admin", nil, &correctedStart, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !capturedIsAllDay {
			t.Error("an all-day row must keep its marker through the review path")
		}
		if !isLocalMidnight(capturedStart, loc) {
			t.Errorf("stored start %s must be local midnight for an all-day row", capturedStart.Format(time.RFC3339))
		}
	})

	t.Run("single-occurrence correction still carries the review marker", func(t *testing.T) {
		event := seriesEvent()
		event.Occurrences = event.Occurrences[:1]
		// The row itself is date-only, so the marker survives the review path on
		// its own merit (from the row's own is_all_day), not via the event-level
		// flag.
		event.Occurrences[0] = Occurrence{
			ID:        "occ-0",
			StartTime: time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
			Timezone:  "America/Toronto",
			IsAllDay:  true,
		}
		repo, wholeEventCalls, perRowCalls := newRepo(event)

		var capturedStart time.Time
		var capturedIsAllDay bool
		repo.updateOccurrenceDatesFunc = func(_ context.Context, _ string, startTime time.Time, _ *time.Time, isAllDay bool) error {
			capturedStart = startTime
			capturedIsAllDay = isAllDay
			*wholeEventCalls = append(*wholeEventCalls, "whole-event")
			return nil
		}

		// Corrected to a timed value; because the row is all-day, it must be
		// re-anchored to local midnight rather than stored as a timed instant
		// carrying the marker.
		correctedStart := time.Date(2026, 11, 1, 19, 0, 0, 0, loc)
		service := newAdminServiceForOccurrenceTest(repo)
		if _, err := service.FixAndApproveEventWithReview(ctx, eventULID, 1, "admin", nil, &correctedStart, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(*perRowCalls) != 0 {
			t.Errorf("single-occurrence events use the whole-event update, got per-row calls %v", *perRowCalls)
		}
		if len(*wholeEventCalls) != 1 {
			t.Fatalf("whole-event update must run once, got %v", *wholeEventCalls)
		}
		if !capturedIsAllDay {
			t.Error("marker must survive the review path for a date-only single-occurrence event")
		}
		if !isLocalMidnight(capturedStart, loc) {
			t.Errorf("marker and instant must agree: stored start %s is not local midnight", capturedStart.Format(time.RFC3339))
		}
	})
}
