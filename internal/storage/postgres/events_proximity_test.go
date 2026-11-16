package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/tests/testhelpers/clock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEventRepositoryListProximityEffectiveVenue(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	org := insertOrganization(t, ctx, pool, "Proximity organizer")
	near := insertPlaceWithCoords(t, ctx, pool, "Nearby", "Toronto", "ON", 43.6532, -79.3832)
	far := insertPlaceWithCoords(t, ctx, pool, "Far away", "Ottawa", "ON", 45.4215, -75.6972)
	unknown := insertPlace(t, ctx, pool, "No coordinates", "Toronto", "ON")
	start := clock.FutureTime(10)
	var expected, unfiltered []string
	for _, tc := range []struct {
		name       string
		primary    seededEntity
		override   *seededEntity
		state      string
		attendance string
		deleted    bool
		matches    bool
	}{
		{name: "primary fallback", primary: near, state: "published", matches: true},
		{name: "far primary", primary: far, state: "published"},
		{name: "unknown primary", primary: unknown, state: "published"},
		{name: "near override", primary: far, override: &near, state: "published", matches: true},
		{name: "far override", primary: near, override: &far, state: "published"},
		{name: "unknown override", primary: near, override: &unknown, state: "published"},
		{name: "virtual only", primary: near, state: "published", attendance: "https://schema.org/OnlineEventAttendanceMode"},
		{name: "hybrid nearby", primary: near, state: "published", attendance: "https://schema.org/MixedEventAttendanceMode", matches: true},
		{name: "draft nearby", primary: near, state: "draft"},
		{name: "pending review nearby", primary: near, state: "pending_review"},
		{name: "cancelled nearby", primary: near, state: "cancelled", matches: true},
		{name: "completed nearby", primary: near, state: "completed", matches: true},
		{name: "deleted nearby", primary: near, state: "published", deleted: true},
	} {
		ulid := insertEvent(t, ctx, pool, tc.name, "Proximity fixture", org, tc.primary, "arts", tc.state, nil, start)
		var venueID any
		if tc.override != nil {
			venueID = tc.override.ID
		}
		_, err := pool.Exec(ctx, `UPDATE event_occurrences SET venue_id = $2, virtual_url = 'https://venue.test/online' WHERE event_id = (SELECT id FROM events WHERE ulid = $1)`, ulid, venueID)
		require.NoError(t, err)
		if tc.attendance != "" {
			_, err = pool.Exec(ctx, `UPDATE events SET attendance_mode = $2 WHERE ulid = $1`, ulid, tc.attendance)
			require.NoError(t, err)
		}
		if tc.attendance == "https://schema.org/OnlineEventAttendanceMode" {
			_, err = pool.Exec(ctx, `UPDATE events SET primary_venue_id = NULL, virtual_url = 'https://venue.test/online' WHERE ulid = $1`, ulid)
			require.NoError(t, err)
		}
		if tc.deleted {
			_, err = pool.Exec(ctx, `UPDATE events SET deleted_at = NOW() WHERE ulid = $1`, ulid)
			require.NoError(t, err)
		}
		if tc.matches {
			expected = append(expected, ulid)
		}
		if !tc.deleted && events.PubliclyVisible(tc.state) {
			unfiltered = append(unfiltered, ulid)
		}
	}
	lat, lon, radius := 43.6532, -79.3832, 5.0
	result, err := repo.List(ctx, events.Filters{Latitude: &lat, Longitude: &lon, RadiusKm: &radius, PublicOnly: true}, events.Pagination{Limit: 100})
	require.NoError(t, err)
	var actual []string
	for _, event := range result.Events {
		actual = append(actual, event.ULID)
	}
	require.ElementsMatch(t, expected, actual)

	// Without proximity, missing coordinates, virtual events, and far venues remain visible.
	result, err = repo.List(ctx, events.Filters{PublicOnly: true}, events.Pagination{Limit: 100})
	require.NoError(t, err)
	actual = nil
	for _, event := range result.Events {
		actual = append(actual, event.ULID)
	}
	require.ElementsMatch(t, unfiltered, actual)
}

func TestEventRepositoryListProximityMatchesSameOccurrence(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	org := insertOrganization(t, ctx, pool, "Proximity organizer")
	near := insertPlaceWithCoords(t, ctx, pool, "Nearby", "Toronto", "ON", 43.6532, -79.3832)
	far := insertPlaceWithCoords(t, ctx, pool, "Far away", "Ottawa", "ON", 45.4215, -75.6972)
	before, inside, later, end := clock.FutureTime(9), clock.FutureTime(11), clock.FutureTime(12), clock.FutureTime(13)
	start := clock.FutureTime(10)
	// Near before the window, far inside: satisfying the two filters on separate
	// occurrences must never make this event eligible.
	crossed := insertEvent(t, ctx, pool, "Crossed conditions", "", org, near, "arts", "published", nil, before)
	_, err := pool.Exec(ctx, `INSERT INTO event_occurrences (event_id, start_time, venue_id) SELECT id, $2, $3 FROM events WHERE ulid = $1`, crossed, inside, far.ID)
	require.NoError(t, err)
	// The first in-window occurrence is far; return the later nearby one.
	matching := insertEvent(t, ctx, pool, "Later matching occurrence", "", org, far, "arts", "published", nil, inside)
	_, err = pool.Exec(ctx, `INSERT INTO event_occurrences (event_id, start_time, venue_id) SELECT id, $2, $3 FROM events WHERE ulid = $1`, matching, later, near.ID)
	require.NoError(t, err)
	// endDate is exclusive even when that occurrence is nearby.
	_ = insertEvent(t, ctx, pool, "At exclusive end", "", org, near, "arts", "published", nil, end)
	lat, lon, radius := 43.6532, -79.3832, 5.0
	result, err := repo.List(ctx, events.Filters{StartDate: &start, EndDate: &end, Latitude: &lat, Longitude: &lon, RadiusKm: &radius}, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	require.Equal(t, matching, result.Events[0].ULID)
	require.Equal(t, later, result.Events[0].Occurrences[0].StartTime.UTC())
	require.NotNil(t, result.Events[0].Occurrences[0].VenueULID)
	require.Equal(t, near.ULID, *result.Events[0].Occurrences[0].VenueULID)
}

func TestEventRepositoryListPaginationChoosesOccurrenceBeforeCursor(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	org := insertOrganization(t, ctx, pool, "Proximity organizer")
	near := insertPlaceWithCoords(t, ctx, pool, "Nearby", "Toronto", "ON", 43.6532, -79.3832)
	start := clock.FutureTime(10)
	var tied []string
	for _, name := range []string{"Recurring A", "Recurring B"} {
		ulid := insertEvent(t, ctx, pool, name, "", org, near, "arts", "published", nil, start)
		tied = append(tied, ulid)
		_, err := pool.Exec(ctx, `INSERT INTO event_occurrences (event_id, start_time, venue_id) SELECT id, $2, $3 FROM events WHERE ulid = $1`, ulid, start.Add(48*time.Hour), near.ID)
		require.NoError(t, err)
	}
	sort.Strings(tied)
	last := insertEvent(t, ctx, pool, "Middle occurrence", "", org, near, "arts", "published", nil, start.Add(24*time.Hour))
	expected := append(tied, last)
	lat, lon, radius := 43.6532, -79.3832, 5.0
	for _, tc := range []struct {
		name    string
		filters events.Filters
	}{
		{name: "without proximity"},
		{name: "with proximity", filters: events.Filters{Latitude: &lat, Longitude: &lon, RadiusKm: &radius}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			var after string
			for page := 0; ; page++ {
				require.Less(t, page, len(expected)+1, "pagination must terminate without replaying later occurrences")
				result, err := repo.List(ctx, tc.filters, events.Pagination{Limit: 1, After: after})
				require.NoError(t, err)
				require.Len(t, result.Events, 1)
				got = append(got, result.Events[0].ULID)
				if result.NextCursor == "" {
					break
				}
				after = result.NextCursor
			}
			require.Equal(t, expected, got)
		})
	}
}

func TestEventRepositoryListProximityGeographyDistances(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	org := insertOrganization(t, ctx, pool, "Equator organizer")
	var expected string
	for _, distance := range []float64{4990, 5010} {
		place := insertPlaceWithCoords(t, ctx, pool, "Boundary venue", "", "", 0, 0)
		// Geography distances are metres. Project east across the antimeridian
		// so a geometry-in-degrees or longitude-order mistake cannot pass.
		_, err := pool.Exec(ctx, `UPDATE places SET
			latitude = ST_Y(ST_Project(ST_SetSRID(ST_MakePoint(179.99, 0), 4326)::geography, $2::float8, pi()/2)::geometry),
			longitude = ST_X(ST_Project(ST_SetSRID(ST_MakePoint(179.99, 0), 4326)::geography, $2::float8, pi()/2)::geometry)
			WHERE id = $1`, place.ID, distance)
		require.NoError(t, err)
		ulid := insertEvent(t, ctx, pool, "Boundary event", "", org, place, "arts", "published", nil, clock.FutureTime(10))
		if distance < 5000 {
			expected = ulid
		}
	}
	lat, lon, radius := 0.0, 179.99, 5.0
	result, err := repo.List(ctx, events.Filters{Latitude: &lat, Longitude: &lon, RadiusKm: &radius}, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	require.Equal(t, expected, result.Events[0].ULID)
}

// Capture the repository's actual SQL so EXPLAIN evidence cannot drift from the
// event-list projection, joins, or parameter types.
type eventListQueryTrace struct {
	sql  string
	args []any
}

func (trace *eventListQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	trace.sql = data.SQL
	trace.args = append([]any(nil), data.Args...)
	return ctx
}

func (*eventListQueryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestEventRepositoryListProximityIndexPlan(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	var definition string
	require.NoError(t, pool.QueryRow(ctx, `SELECT pg_get_indexdef('idx_places_geography'::regclass)`).Scan(&definition))
	require.Contains(t, definition, "USING gist")
	require.Contains(t, definition, "geo_point")
	require.Contains(t, definition, "::geography")

	// A selective 5km search among 5,000 geocoded venues/events is large enough
	// to give the planner a realistic choice, without forcing enable_seqscan off.
	_, err := pool.Exec(ctx, `
		WITH venues AS (
			INSERT INTO places (ulid, name, latitude, longitude)
			SELECT lpad(g::text, 26, '0'), 'Plan venue ' || g,
				CASE WHEN g = 1 THEN 43.6532 ELSE 10 + (g % 6000) / 100.0 END,
				CASE WHEN g = 1 THEN -79.3832 ELSE 10 + (g % 15000) / 100.0 END
			FROM generate_series(1, 5000) g RETURNING id, ulid
		), seeded_events AS (
			INSERT INTO events (ulid, name, primary_venue_id, lifecycle_state)
			SELECT ulid, 'Plan event ' || ulid, id, 'published' FROM venues
			RETURNING id, primary_venue_id
		)
		INSERT INTO event_occurrences (event_id, start_time, venue_id)
		SELECT id, $1::timestamptz, primary_venue_id FROM seeded_events`, clock.FutureTime(10))
	require.NoError(t, err)
	for _, table := range []string{"places", "events", "event_occurrences"} {
		_, err = pool.Exec(ctx, "ANALYZE "+table)
		require.NoError(t, err)
	}
	trace := &eventListQueryTrace{}
	poolConfig := pool.Config()
	poolConfig.ConnConfig.Tracer = trace
	tracedPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	require.NoError(t, err)
	defer tracedPool.Close()
	lat, lon, radius := 43.6532, -79.3832, 5.0
	result, err := NewEventRepository(tracedPool, zerolog.Nop()).List(ctx,
		events.Filters{Latitude: &lat, Longitude: &lon, RadiusKm: &radius, PublicOnly: true}, events.Pagination{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	require.Equal(t, "00000000000000000000000001", result.Events[0].ULID)
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+trace.sql, trace.args...)
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan = append(plan, line)
	}
	require.NoError(t, rows.Err())
	// Do not assert a planner snapshot or chosen strategy: those vary with
	// PostgreSQL versions/statistics. The migration definition above guards the
	// expression index; log the real query's natural plan for performance review.
	t.Log("Event proximity plan (5,000 venues/events):\n" + strings.Join(plan, "\n"))

	t.Run("migration reverses and reapplies", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		for _, direction := range []string{"down", "up"} {
			sql, err := os.ReadFile(filepath.Join(projectRoot(), DefaultMigrationsPath, "000062_places_geography_index."+direction+".sql"))
			require.NoError(t, err)
			_, err = tx.Exec(ctx, string(sql))
			require.NoError(t, err)
			var exists bool
			require.NoError(t, tx.QueryRow(ctx, `SELECT to_regclass('idx_places_geography') IS NOT NULL`).Scan(&exists))
			require.Equal(t, direction == "up", exists)
		}
		var reapplied string
		require.NoError(t, tx.QueryRow(ctx, `SELECT pg_get_indexdef('idx_places_geography'::regclass)`).Scan(&reapplied))
		require.Equal(t, definition, reapplied)
	})
}
