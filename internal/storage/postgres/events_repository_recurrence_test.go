package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetByULID_RecurrenceRule covers the three recurrence scenarios for T2 (srv-i1f0t):
//  1. Event with no series       → SeriesID=nil, Recurrence=nil
//  2. Event with series but no RRULE → SeriesID set, Recurrence=nil
//  3. Event with series + RRULE  → both SeriesID and Recurrence populated
func TestGetByULID_RecurrenceRule(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool}

	// Seed a place so occurrences satisfy the location constraint.
	place := insertPlace(t, ctx, pool, "Test Venue", "Toronto", "ON")

	// Helper: insert a minimal event with an occurrence and an optional series_id.
	insertEventWithSeries := func(t *testing.T, seriesID *string) string {
		t.Helper()
		eventULID := ulid.Make().String()
		var eventID string
		err := pool.QueryRow(ctx,
			`INSERT INTO events (ulid, name, event_domain, lifecycle_state, primary_venue_id, series_id)
			 VALUES ($1, $2, $3, $4, $5, $6::uuid)
			 RETURNING id`,
			eventULID, "Test Recurring Event", "arts", "published", place.ID, seriesID,
		).Scan(&eventID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx,
			`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
			 VALUES ($1, $2, $3, $4)`,
			eventID,
			time.Now().Add(24*time.Hour),
			time.Now().Add(26*time.Hour),
			place.ID,
		)
		require.NoError(t, err)
		return eventULID
	}

	// Helper: insert an event_series row with optional rrule.
	insertSeries := func(t *testing.T, rrule *string) string {
		t.Helper()
		var seriesID string
		err := pool.QueryRow(ctx,
			`INSERT INTO event_series (name, series_start_date, schedule_timezone, rrule)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			"Test Series",
			time.Now().Format("2006-01-02"),
			"America/Toronto",
			rrule,
		).Scan(&seriesID)
		require.NoError(t, err)
		return seriesID
	}

	t.Run("no series — SeriesID and Recurrence nil", func(t *testing.T) {
		eventULID := insertEventWithSeries(t, nil)

		evt, err := repo.GetByULID(ctx, eventULID)
		require.NoError(t, err)
		require.NotNil(t, evt)

		assert.Nil(t, evt.SeriesID, "SeriesID should be nil for event with no series")
		assert.Nil(t, evt.Recurrence, "Recurrence should be nil for event with no series")
	})

	t.Run("series without RRULE — SeriesID set, Recurrence nil", func(t *testing.T) {
		seriesID := insertSeries(t, nil) // rrule = NULL
		eventULID := insertEventWithSeries(t, &seriesID)

		evt, err := repo.GetByULID(ctx, eventULID)
		require.NoError(t, err)
		require.NotNil(t, evt)

		assert.NotNil(t, evt.SeriesID, "SeriesID should be set when event has a series")
		assert.Equal(t, seriesID, *evt.SeriesID)
		assert.Nil(t, evt.Recurrence, "Recurrence should be nil when series has no RRULE")
	})

	t.Run("series with RRULE — SeriesID and Recurrence populated", func(t *testing.T) {
		rrule := "FREQ=WEEKLY;BYDAY=MO,WE"
		seriesStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		seriesEnd := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		var seriesID string
		err := pool.QueryRow(ctx,
			`INSERT INTO event_series (name, series_start_date, series_end_date, schedule_timezone, rrule)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id`,
			"Test Series",
			seriesStart.Format("2006-01-02"),
			seriesEnd.Format("2006-01-02"),
			"America/Toronto",
			rrule,
		).Scan(&seriesID)
		require.NoError(t, err)

		// Also set exdates on the series row to verify array population.
		exDate := time.Date(2026, 5, 4, 18, 0, 0, 0, time.UTC)
		_, err = pool.Exec(ctx,
			`UPDATE event_series SET exdates = $1 WHERE id = $2::uuid`,
			[]time.Time{exDate},
			seriesID,
		)
		require.NoError(t, err)

		eventULID := insertEventWithSeries(t, &seriesID)

		evt, err := repo.GetByULID(ctx, eventULID)
		require.NoError(t, err)
		require.NotNil(t, evt)

		assert.NotNil(t, evt.SeriesID, "SeriesID should be set")
		assert.Equal(t, seriesID, *evt.SeriesID)

		require.NotNil(t, evt.Recurrence, "Recurrence should be populated for series with RRULE")
		assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO,WE", evt.Recurrence.RRule)
		assert.Equal(t, "America/Toronto", evt.Recurrence.TZID)
		require.Len(t, evt.Recurrence.ExDates, 1)
		assert.True(t, evt.Recurrence.ExDates[0].Equal(exDate),
			"ExDate should match stored value; got %v want %v", evt.Recurrence.ExDates[0], exDate)

		require.NotNil(t, evt.Recurrence.SeriesStart, "SeriesStart should be populated from event_series.series_start_date")
		assert.Equal(t, seriesStart.Year(), evt.Recurrence.SeriesStart.Year())
		assert.Equal(t, seriesStart.Month(), evt.Recurrence.SeriesStart.Month())
		assert.Equal(t, seriesStart.Day(), evt.Recurrence.SeriesStart.Day())

		require.NotNil(t, evt.Recurrence.SeriesEnd, "SeriesEnd should be populated from event_series.series_end_date")
		assert.Equal(t, seriesEnd.Year(), evt.Recurrence.SeriesEnd.Year())
		assert.Equal(t, seriesEnd.Month(), evt.Recurrence.SeriesEnd.Month())
		assert.Equal(t, seriesEnd.Day(), evt.Recurrence.SeriesEnd.Day())
	})
}

// TestList_RecurrenceRule verifies that List() populates Recurrence from the
// event_series LEFT JOIN, mirroring the same three scenarios as GetByULID.
func TestList_RecurrenceRule(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool}

	place := insertPlace(t, ctx, pool, "Test Venue", "Toronto", "ON")

	insertEventWithSeries := func(t *testing.T, name string, seriesID *string) string {
		t.Helper()
		eventULID := ulid.Make().String()
		var eventID string
		err := pool.QueryRow(ctx,
			`INSERT INTO events (ulid, name, event_domain, lifecycle_state, primary_venue_id, series_id)
			 VALUES ($1, $2, $3, $4, $5, $6::uuid)
			 RETURNING id`,
			eventULID, name, "arts", "published", place.ID, seriesID,
		).Scan(&eventID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx,
			`INSERT INTO event_occurrences (event_id, start_time, end_time, venue_id)
			 VALUES ($1, $2, $3, $4)`,
			eventID,
			time.Now().Add(24*time.Hour),
			time.Now().Add(26*time.Hour),
			place.ID,
		)
		require.NoError(t, err)
		return eventULID
	}

	t.Run("event without series — Recurrence nil", func(t *testing.T) {
		eventULID := insertEventWithSeries(t, "No Series List Event", nil)

		results, err := repo.List(ctx, events.Filters{}, events.Pagination{Limit: 100})
		require.NoError(t, err)

		var found *events.Event
		for i := range results.Events {
			if results.Events[i].ULID == eventULID {
				found = &results.Events[i]
				break
			}
		}
		require.NotNil(t, found, "event should be found in list results")
		assert.Nil(t, found.Recurrence, "Recurrence should be nil for event with no series")
	})

	t.Run("event with series + RRULE — Recurrence populated via List", func(t *testing.T) {
		rrule := "FREQ=WEEKLY;BYDAY=MO"
		seriesStart := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
		seriesEnd := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
		var seriesID string
		err := pool.QueryRow(ctx,
			`INSERT INTO event_series (name, series_start_date, series_end_date, schedule_timezone, rrule)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id`,
			"List Test Series",
			seriesStart.Format("2006-01-02"),
			seriesEnd.Format("2006-01-02"),
			"America/Toronto",
			rrule,
		).Scan(&seriesID)
		require.NoError(t, err)

		eventULID := insertEventWithSeries(t, "Series List Event", &seriesID)

		results, err := repo.List(ctx, events.Filters{}, events.Pagination{Limit: 100})
		require.NoError(t, err)

		var found *events.Event
		for i := range results.Events {
			if results.Events[i].ULID == eventULID {
				found = &results.Events[i]
				break
			}
		}
		require.NotNil(t, found, "event should be found in list results")

		require.NotNil(t, found.Recurrence, "Recurrence should be populated via List when event belongs to series with RRULE")
		assert.Equal(t, "FREQ=WEEKLY;BYDAY=MO", found.Recurrence.RRule)
		assert.Equal(t, "America/Toronto", found.Recurrence.TZID)

		require.NotNil(t, found.Recurrence.SeriesStart, "SeriesStart should be populated")
		assert.Equal(t, seriesStart.Year(), found.Recurrence.SeriesStart.Year())
		assert.Equal(t, seriesStart.Month(), found.Recurrence.SeriesStart.Month())
		assert.Equal(t, seriesStart.Day(), found.Recurrence.SeriesStart.Day())

		require.NotNil(t, found.Recurrence.SeriesEnd, "SeriesEnd should be populated")
		assert.Equal(t, seriesEnd.Year(), found.Recurrence.SeriesEnd.Year())
		assert.Equal(t, seriesEnd.Month(), found.Recurrence.SeriesEnd.Month())
		assert.Equal(t, seriesEnd.Day(), found.Recurrence.SeriesEnd.Day())
	})
}
