package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/stretchr/testify/require"
	"github.com/rs/zerolog"
)

func TestUpsertEventSeries(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	seriesStart := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	seriesEnd := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	exdate := time.Date(2026, 7, 6, 19, 0, 0, 0, time.UTC)

	t.Run("create new series", func(t *testing.T) {
		result, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-001",
			Name:        "Weekly Workshop",
			SeriesStart: seriesStart,
			SeriesEnd:   &seriesEnd,
			RRule:       "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20260831T235959Z",
			ExDates:     []time.Time{exdate},
			RDates:      nil,
			TZID:        "America/Toronto",
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.SeriesID, "SeriesID should not be empty")

		// Verify row was inserted correctly
		var name, rrule, tzid string
		var extKey *string
		err = pool.QueryRow(ctx,
			`SELECT name, rrule, schedule_timezone, external_key FROM event_series WHERE id = $1`,
			result.SeriesID,
		).Scan(&name, &rrule, &tzid, &extKey)
		require.NoError(t, err)
		require.Equal(t, "Weekly Workshop", name)
		require.Equal(t, "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20260831T235959Z", rrule)
		require.Equal(t, "America/Toronto", tzid)
		require.NotNil(t, extKey)
		require.Equal(t, "myorg-ical:EVT-001", *extKey)
	})

	t.Run("upsert updates existing series by external_key", func(t *testing.T) {
		// First insert
		result1, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-002",
			Name:        "Original Name",
			SeriesStart: seriesStart,
			SeriesEnd:   nil,
			RRule:       "FREQ=DAILY;COUNT=10",
			ExDates:     nil,
			RDates:      nil,
			TZID:        "UTC",
		})
		require.NoError(t, err)
		require.NotEmpty(t, result1.SeriesID)

		// Upsert with same external_key — should update, not create new
		newSeriesEnd := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		result2, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-002",
			Name:        "Updated Name",
			SeriesStart: seriesStart,
			SeriesEnd:   &newSeriesEnd,
			RRule:       "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20261231T235959Z",
			ExDates:     []time.Time{exdate},
			RDates:      nil,
			TZID:        "America/Toronto",
		})
		require.NoError(t, err)
		require.Equal(t, result1.SeriesID, result2.SeriesID, "Upsert should return same SeriesID")

		// Verify updated values
		var name, rrule, tzid string
		err = pool.QueryRow(ctx,
			`SELECT name, rrule, schedule_timezone FROM event_series WHERE id = $1`,
			result2.SeriesID,
		).Scan(&name, &rrule, &tzid)
		require.NoError(t, err)
		require.Equal(t, "Updated Name", name)
		require.Equal(t, "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20261231T235959Z", rrule)
		require.Equal(t, "America/Toronto", tzid)
	})

	t.Run("nil SeriesEnd", func(t *testing.T) {
		result, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-003",
			Name:        "Infinite Series",
			SeriesStart: seriesStart,
			SeriesEnd:   nil,
			RRule:       "FREQ=DAILY",
			ExDates:     nil,
			RDates:      nil,
			TZID:        "UTC",
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.SeriesID)

		var seriesEndDate *time.Time
		err = pool.QueryRow(ctx,
			`SELECT series_end_date FROM event_series WHERE id = $1`,
			result.SeriesID,
		).Scan(&seriesEndDate)
		require.NoError(t, err)
		require.Nil(t, seriesEndDate, "series_end_date should be NULL for infinite series")
	})

	t.Run("empty TZID defaults to UTC", func(t *testing.T) {
		result, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-004",
			Name:        "No TZID",
			SeriesStart: seriesStart,
			SeriesEnd:   &seriesEnd,
			RRule:       "FREQ=WEEKLY;UNTIL=20260831T235959Z",
			ExDates:     nil,
			RDates:      nil,
			TZID:        "",
		})
		require.NoError(t, err)

		var tzid string
		err = pool.QueryRow(ctx,
			`SELECT schedule_timezone FROM event_series WHERE id = $1`,
			result.SeriesID,
		).Scan(&tzid)
		require.NoError(t, err)
		require.Equal(t, "UTC", tzid)
	})
}
