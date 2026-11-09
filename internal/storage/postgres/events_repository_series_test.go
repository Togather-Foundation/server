package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/tests/testhelpers/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestUpsertEventSeries(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	seriesStart := clock.FutureTimeAt(1, 0, 0)
	seriesEnd := clock.FutureTimeAt(56, 0, 0)
	seriesRRule := "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=" + seriesEnd.Format("20060102") + "T235959Z"
	exdate := clock.FutureTime(1)

	t.Run("create new series", func(t *testing.T) {
		result, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-001",
			Name:        "Weekly Workshop",
			SeriesStart: seriesStart,
			SeriesEnd:   &seriesEnd,
			RRule:       seriesRRule,
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
		require.Equal(t, seriesRRule, rrule)
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
		newSeriesEnd := clock.FutureTimeAt(180, 0, 0)
		newSeriesRRule := "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=" + newSeriesEnd.Format("20060102") + "T235959Z"
		result2, err := repo.UpsertEventSeries(ctx, events.UpsertEventSeriesParams{
			ExternalKey: "myorg-ical:EVT-002",
			Name:        "Updated Name",
			SeriesStart: seriesStart,
			SeriesEnd:   &newSeriesEnd,
			RRule:       newSeriesRRule,
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
		require.Equal(t, newSeriesRRule, rrule)
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
			RRule:       "FREQ=WEEKLY;UNTIL=" + seriesEnd.Format("20060102") + "T235959Z",
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

// TestListSeriesCandidatePairsPredicates pins every edge predicate: venue,
// 7-day cadence, weekday/time window, name similarity, occurrence status, and
// occurrence venue. A broad negative control (14-day cadence) is included.
func TestListSeriesCandidatePairsPredicates(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	venueA := insertPlace(t, ctx, pool, "Main Branch", "Toronto", "ON")
	venueB := insertPlace(t, ctx, pool, "Other Branch", "Toronto", "ON")

	baseA := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(2, 18, 0))
	baseB := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(9, 18, 0))
	baseC := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(16, 18, 0))
	within30 := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(9, 18, 20))

	// Exactly 30 minutes after the day-16 event: inside the <= 1800s window.
	exactly30 := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(23, 18, 30))

	// 31 minutes after the day-16 event: outside the ±30-minute window.
	outsideTime := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(23, 18, 31))

	// pg_trgm similarity ~0.75: below the 0.80 gate but otherwise a perfect
	// venue/day/time match for the day-2 and day-16 events.
	similarBelowGate := insertSeriesTestEvent(t, ctx, pool, "Family Story Time", venueA, clock.FutureTimeAt(9, 18, 0))
	requireSeriesSimilarityRange(t, ctx, pool, "Family Storytime", "Family Story Time", 0.70, 0.80)

	// Identical name and time at another venue: only the venue predicate can
	// exclude it from venue A's cluster.
	otherVenue := insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueB, clock.FutureTimeAt(9, 18, 0))

	// Cancelled occurrence: same venue/name/time as the day-2 event.
	cancelled := insertSeriesTestEventOnly(t, ctx, pool, "Family Storytime", venueA)
	statusCancelled := "cancelled"
	insertSeriesTestOccurrence(t, ctx, pool, cancelled.eventID, clock.FutureTimeAt(2, 18, 0), venueA, &statusCancelled)

	// Primary venue is A but the occurrence points at venue B: it must not be
	// dragged into A's cluster and then merged by --transfer-occurrences.
	wrongOccurrenceVenue := insertSeriesTestEventOnly(t, ctx, pool, "Family Storytime", venueA)
	insertSeriesTestOccurrence(t, ctx, pool, wrongOccurrenceVenue.eventID, clock.FutureTimeAt(9, 18, 0), venueB, nil)

	now := time.Now().UTC()
	pairs, err := repo.ListSeriesCandidatePairs(ctx, events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowStart(now, 14),
		WindowEnd:   seriesTestWindowEnd(now, 90),
		Now:         now,
	})
	require.NoError(t, err)

	result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{
		Now:          now,
		Days:         90,
		LookbackDays: 14,
		MinEvents:    2,
		Limit:        50,
	})
	require.Len(t, result.Items, 1, "only the venue-A weekly events should cluster")
	cluster := result.Items[0]

	want := map[string]bool{
		baseA:     true,
		baseB:     true,
		baseC:     true,
		within30:  true,
		exactly30: true,
	}
	for _, member := range cluster.Members {
		require.Truef(t, want[member.ULID], "unexpected cluster member %s (%s)", member.ULID, member.Name)
		require.Equal(t, venueA.ULID, member.VenueULID)
		delete(want, member.ULID)
	}
	require.Empty(t, want, "expected all in-window, in-time, similar, non-cancelled venue-A events")

	for name, ulid := range map[string]string{
		"outside ±30 min":           outsideTime,
		"similarity below 0.80":     similarBelowGate,
		"different venue":           otherVenue,
		"cancelled occurrence":      cancelled.ulid,
		"occurrence at other venue": wrongOccurrenceVenue.ulid,
	} {
		require.NotContains(t, memberULIDs(cluster), ulid, "%s must not join the cluster", name)
	}

	require.Equal(t, "Family Storytime", cluster.Name)
	require.Equal(t, 5, cluster.EventCount)
	require.Equal(t, "18:00", cluster.LocalStartTime)
}

func TestListSeriesCandidatePairsVenueFilter(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	venueA := insertPlace(t, ctx, pool, "Main Branch", "Toronto", "ON")
	venueB := insertPlace(t, ctx, pool, "Other Branch", "Toronto", "ON")

	for i := 0; i < 3; i++ {
		insertSeriesTestEvent(t, ctx, pool, "Family Storytime", venueA, clock.FutureTimeAt(2+i*7, 18, 0))
		insertSeriesTestEvent(t, ctx, pool, "Toddler Art", venueB, clock.FutureTimeAt(2+i*7, 10, 0))
	}

	now := time.Now().UTC()
	query := events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowStart(now, 14),
		WindowEnd:   seriesTestWindowEnd(now, 90),
		Now:         now,
		VenueULID:   venueB.ULID,
	}
	pairs, err := repo.ListSeriesCandidatePairs(ctx, query)
	require.NoError(t, err)
	require.NotEmpty(t, pairs)

	result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
	require.Len(t, result.Items, 1)
	require.Equal(t, venueB.ULID, result.Items[0].VenueID)
	for _, member := range result.Items[0].Members {
		require.Equal(t, venueB.ULID, member.VenueULID)
	}

	// No filter returns both venues.
	query.VenueULID = ""
	pairs, err = repo.ListSeriesCandidatePairs(ctx, query)
	require.NoError(t, err)
	all := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
	require.Len(t, all.Items, 2)
}

// TestListSeriesCandidatePairsTransitive documents the known behaviour: edges
// are transitive. A~B and B~C collapse A, B, and C into one cluster even though
// A and C are directly compared (A day 2 -> C day 9) and fail the name gate.
func TestListSeriesCandidatePairsTransitive(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	venue := insertPlace(t, ctx, pool, "Transitive Hall", "Toronto", "ON")

	const (
		nameA = "Family Storytime for Toddlers"
		nameB = "Family Storytime for Toddlers Session"
		nameC = "Family Storytime Toddler Session"
	)
	requireSeriesSimilarityRange(t, ctx, pool, nameA, nameB, 0.80, 1.0)
	requireSeriesSimilarityRange(t, ctx, pool, nameB, nameC, 0.80, 1.0)
	requireSeriesSimilarityRange(t, ctx, pool, nameA, nameC, 0.0, 0.80)

	a := insertSeriesTestEvent(t, ctx, pool, nameA, venue, clock.FutureTimeAt(2, 18, 0))
	b := insertSeriesTestEvent(t, ctx, pool, nameB, venue, clock.FutureTimeAt(9, 18, 0))
	c := insertSeriesTestEventOnly(t, ctx, pool, nameC, venue)
	insertSeriesTestOccurrence(t, ctx, pool, c.eventID, clock.FutureTimeAt(9, 18, 0), venue, nil)
	insertSeriesTestOccurrence(t, ctx, pool, c.eventID, clock.FutureTimeAt(16, 18, 0), venue, nil)

	now := time.Now().UTC()
	pairs, err := repo.ListSeriesCandidatePairs(ctx, events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowStart(now, 14),
		WindowEnd:   seriesTestWindowEnd(now, 90),
		Now:         now,
	})
	require.NoError(t, err)

	result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 3, Limit: 50})
	require.Len(t, result.Items, 1)
	cluster := result.Items[0]
	require.Equal(t, 3, cluster.EventCount)
	require.ElementsMatch(t, []string{a, b, c.ulid}, memberULIDs(cluster))
}

func TestListSeriesCandidatePairsWindowBoundary(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	venue := insertPlace(t, ctx, pool, "Edge Hall", "Toronto", "ON")

	// Requested window: [today+10, today+20]. The SQL deliberately widens each
	// side by one day, so only dates [today+9, today+21] may form edges.
	windowStartOffset := 10
	windowEndOffset := 20

	// Inside the widened left edge, outside the request: must pair.
	leftInsidePrev := insertSeriesTestEvent(t, ctx, pool, "Alpha Weekly", venue, clock.FutureTimeAt(windowStartOffset-1, 18, 0))
	leftInsideNext := insertSeriesTestEvent(t, ctx, pool, "Alpha Weekly", venue, clock.FutureTimeAt(windowStartOffset+6, 18, 0))
	// Two days past the left edge: must not pair (would prove a -2 widening).
	leftOutsidePrev := insertSeriesTestEvent(t, ctx, pool, "Bravo Weekly", venue, clock.FutureTimeAt(windowStartOffset-2, 18, 0))
	leftOutsideNext := insertSeriesTestEvent(t, ctx, pool, "Bravo Weekly", venue, clock.FutureTimeAt(windowStartOffset+5, 18, 0))

	// Inside the widened right edge, outside the request: must pair.
	rightInsidePrev := insertSeriesTestEvent(t, ctx, pool, "Charlie Weekly", venue, clock.FutureTimeAt(windowEndOffset-6, 18, 0))
	rightInsideNext := insertSeriesTestEvent(t, ctx, pool, "Charlie Weekly", venue, clock.FutureTimeAt(windowEndOffset+1, 18, 0))
	// Two days past the right edge: must not pair (would prove a +2 widening).
	rightOutsidePrev := insertSeriesTestEvent(t, ctx, pool, "Delta Weekly", venue, clock.FutureTimeAt(windowEndOffset-5, 18, 0))
	rightOutsideNext := insertSeriesTestEvent(t, ctx, pool, "Delta Weekly", venue, clock.FutureTimeAt(windowEndOffset+2, 18, 0))

	now := time.Now().UTC()
	query := events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowEnd(now, windowStartOffset),
		WindowEnd:   seriesTestWindowEnd(now, windowEndOffset),
		Now:         now,
	}
	pairs, err := repo.ListSeriesCandidatePairs(ctx, query)
	require.NoError(t, err)

	result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
	require.Len(t, result.Items, 2, "only the ±1-day boundary pairs should cluster")

	byName := make(map[string][]string, len(result.Items))
	for _, cluster := range result.Items {
		byName[cluster.Name] = memberULIDs(cluster)
	}
	require.ElementsMatch(t, []string{leftInsidePrev, leftInsideNext}, byName["Alpha Weekly"])
	require.ElementsMatch(t, []string{rightInsidePrev, rightInsideNext}, byName["Charlie Weekly"])

	allMembers := make([]string, 0, 4)
	for _, ulids := range byName {
		allMembers = append(allMembers, ulids...)
	}
	require.NotContains(t, allMembers, leftOutsidePrev, "a -2-day widening must not include the left-outside pair")
	require.NotContains(t, allMembers, leftOutsideNext)
	require.NotContains(t, allMembers, rightOutsidePrev, "a +2-day widening must not include the right-outside pair")
	require.NotContains(t, allMembers, rightOutsideNext)
}

func TestResolveSourceIdentifier(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	namedID := insertSeriesTestSource(t, ctx, pool, "tpl-events", nil)

	// A source name that happens to be UUID-shaped must still resolve by name:
	// the UUID lookup only matches when some source actually owns that id.
	uuidShapedName := uuid.NewString()
	uuidShapedID := insertSeriesTestSource(t, ctx, pool, uuidShapedName, nil)

	// Same name on two different base URLs is legal (identity is base_url+name)
	// and therefore ambiguous by name alone.
	baseA := "https://a.example.com/feed"
	baseB := "https://b.example.com/feed"
	insertSeriesTestSource(t, ctx, pool, "duplicated", &baseA)
	insertSeriesTestSource(t, ctx, pool, "duplicated", &baseB)

	t.Run("name", func(t *testing.T) {
		got, err := repo.ResolveSourceIdentifier(ctx, "tpl-events")
		require.NoError(t, err)
		require.Equal(t, namedID, got)
	})

	t.Run("uuid", func(t *testing.T) {
		got, err := repo.ResolveSourceIdentifier(ctx, namedID)
		require.NoError(t, err)
		require.Equal(t, namedID, got)
	})

	t.Run("uuid is case-insensitive", func(t *testing.T) {
		got, err := repo.ResolveSourceIdentifier(ctx, strings.ToUpper(namedID))
		require.NoError(t, err)
		require.Equal(t, namedID, got)
	})

	t.Run("uuid-shaped source name resolves by name", func(t *testing.T) {
		// Safe behaviour: UUID parsing must not shadow an exact name match.
		got, err := repo.ResolveSourceIdentifier(ctx, uuidShapedName)
		require.NoError(t, err)
		require.Equal(t, uuidShapedID, got)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		got, err := repo.ResolveSourceIdentifier(ctx, "  tpl-events  ")
		require.NoError(t, err)
		require.Equal(t, namedID, got)
	})

	t.Run("unknown", func(t *testing.T) {
		_, err := repo.ResolveSourceIdentifier(ctx, "does-not-exist")
		require.Error(t, err)
		require.True(t, errors.Is(err, events.ErrSourceNotFound))
		require.Contains(t, err.Error(), "expected a source name or UUID")
	})

	t.Run("ulid is rejected with guidance", func(t *testing.T) {
		_, err := repo.ResolveSourceIdentifier(ctx, ulid.Make().String())
		require.Error(t, err)
		require.True(t, errors.Is(err, events.ErrSourceNotFound))
		require.Contains(t, err.Error(), "looks like a ULID")
		require.Contains(t, err.Error(), "expected a source name or UUID")
	})

	t.Run("ambiguous name", func(t *testing.T) {
		_, err := repo.ResolveSourceIdentifier(ctx, "duplicated")
		require.Error(t, err)
		require.True(t, errors.Is(err, events.ErrSourceAmbiguous))
	})
}

// TestListSeriesCandidatePairsSourceFilter proves strict source scoping: a
// source-scoped run can only form clusters from that source's events, and an
// event with no attribution never joins one.
func TestListSeriesCandidatePairsSourceFilter(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	venue := insertPlace(t, ctx, pool, "Mixed Source Hall", "Toronto", "ON")

	sourceA := insertSeriesTestSource(t, ctx, pool, "source-a", nil)
	sourceB := insertSeriesTestSource(t, ctx, pool, "source-b", nil)

	a1 := insertSeriesTestEventWithSource(t, ctx, pool, "Mixed Storytime", venue, clock.FutureTimeAt(2, 18, 0), sourceA)
	a2 := insertSeriesTestEventWithSource(t, ctx, pool, "Mixed Storytime", venue, clock.FutureTimeAt(9, 18, 0), sourceA)
	a3 := insertSeriesTestEventWithSource(t, ctx, pool, "Mixed Storytime", venue, clock.FutureTimeAt(16, 18, 0), sourceA)

	// Same venue/name/cadence but a different source: forms edges in an
	// unscoped run, but cannot be its own cluster when scoped.
	b := insertSeriesTestEventWithSource(t, ctx, pool, "Mixed Storytime", venue, clock.FutureTimeAt(9, 18, 0), sourceB)

	// No event_sources attribution: excluded by any source-scoped run.
	unattributed := insertSeriesTestEvent(t, ctx, pool, "Mixed Storytime", venue, clock.FutureTimeAt(9, 18, 0))

	now := time.Now().UTC()
	baseQuery := events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowStart(now, 14),
		WindowEnd:   seriesTestWindowEnd(now, 90),
		Now:         now,
	}

	t.Run("unscoped reports both sources", func(t *testing.T) {
		pairs, err := repo.ListSeriesCandidatePairs(ctx, baseQuery)
		require.NoError(t, err)

		result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 1)
		require.Equal(t, 5, result.Items[0].EventCount)
		require.ElementsMatch(t, []string{a1, a2, a3, b, unattributed}, memberULIDs(result.Items[0]))
	})

	t.Run("scoped cluster is narrowed to the source", func(t *testing.T) {
		query := baseQuery
		query.SourceID = sourceA
		pairs, err := repo.ListSeriesCandidatePairs(ctx, query)
		require.NoError(t, err)

		result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Len(t, result.Items, 1)
		require.Equal(t, 3, result.Items[0].EventCount)
		require.ElementsMatch(t, []string{a1, a2, a3}, memberULIDs(result.Items[0]))
	})

	t.Run("single matching event is not reported", func(t *testing.T) {
		query := baseQuery
		query.SourceID = sourceB
		pairs, err := repo.ListSeriesCandidatePairs(ctx, query)
		require.NoError(t, err)
		require.Empty(t, pairs)

		result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
		require.Empty(t, result.Items, "a cluster of one must not be reported")
	})
}

// TestListSeriesCandidatePairsSourceFilterDualAttribution documents that an
// event attributed to two provenance sources appears in both scoped runs. A
// scoped consolidate can therefore retire an event that also carries another
// source's provenance, which is why the member list must be verified.
func TestListSeriesCandidatePairsSourceFilterDualAttribution(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}
	venue := insertPlace(t, ctx, pool, "Dual Attribution Hall", "Toronto", "ON")

	sourceA := insertSeriesTestSource(t, ctx, pool, "dual-a", nil)
	sourceB := insertSeriesTestSource(t, ctx, pool, "dual-b", nil)

	a1 := insertSeriesTestEventWithSource(t, ctx, pool, "Dual Storytime", venue, clock.FutureTimeAt(2, 18, 0), sourceA)

	dual := insertSeriesTestEventOnly(t, ctx, pool, "Dual Storytime", venue)
	insertSeriesTestOccurrence(t, ctx, pool, dual.eventID, clock.FutureTimeAt(9, 18, 0), venue, nil)
	insertSeriesTestEventSource(t, ctx, pool, dual.eventID, sourceA)
	insertSeriesTestEventSource(t, ctx, pool, dual.eventID, sourceB)

	a3 := insertSeriesTestEventWithSource(t, ctx, pool, "Dual Storytime", venue, clock.FutureTimeAt(16, 18, 0), sourceA)
	b2 := insertSeriesTestEventWithSource(t, ctx, pool, "Dual Storytime", venue, clock.FutureTimeAt(16, 18, 0), sourceB)

	now := time.Now().UTC()
	baseQuery := events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowStart(now, 14),
		WindowEnd:   seriesTestWindowEnd(now, 90),
		Now:         now,
	}

	for name, tc := range map[string]struct {
		sourceID string
		want     []string
	}{
		"source A": {sourceID: sourceA, want: []string{a1, dual.ulid, a3}},
		"source B": {sourceID: sourceB, want: []string{dual.ulid, b2}},
	} {
		t.Run(name, func(t *testing.T) {
			query := baseQuery
			query.SourceID = tc.sourceID
			pairs, err := repo.ListSeriesCandidatePairs(ctx, query)
			require.NoError(t, err)

			result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
			require.Len(t, result.Items, 1)
			require.ElementsMatch(t, tc.want, memberULIDs(result.Items[0]))
			require.Contains(t, memberULIDs(result.Items[0]), dual.ulid,
				"the dual-attributed event appears in both scopes")
		})
	}
}

// TestListSeriesCandidatePairsDescriptionFingerprints proves the pair rows
// carry only an opaque description fingerprint and that it collapses trimmed
// whitespace but not punctuation. The description text never reaches the pairs.
func TestListSeriesCandidatePairsDescriptionFingerprints(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)

	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	// Same text, one with surrounding whitespace: collapses to one fingerprint.
	trimmedVenue := insertPlace(t, ctx, pool, "Trimmed Hall", "Toronto", "ON")
	insertSeriesTestEventWithDescription(t, ctx, pool, "Trimmed Storytime", "Weekly storytime", trimmedVenue, clock.FutureTimeAt(2, 18, 0))
	insertSeriesTestEventWithDescription(t, ctx, pool, "Trimmed Storytime", "  Weekly storytime  ", trimmedVenue, clock.FutureTimeAt(9, 18, 0))

	// Punctuation differs: must remain two distinct fingerprints.
	punctVenue := insertPlace(t, ctx, pool, "Punctuation Hall", "Toronto", "ON")
	insertSeriesTestEventWithDescription(t, ctx, pool, "Punctuation Storytime", "Weekly storytime.", punctVenue, clock.FutureTimeAt(2, 18, 0))
	insertSeriesTestEventWithDescription(t, ctx, pool, "Punctuation Storytime", "Weekly storytime", punctVenue, clock.FutureTimeAt(9, 18, 0))

	// No descriptions at all: empty fingerprints that never count.
	emptyVenue := insertPlace(t, ctx, pool, "Empty Hall", "Toronto", "ON")
	insertSeriesTestEvent(t, ctx, pool, "Empty Storytime", emptyVenue, clock.FutureTimeAt(2, 18, 0))
	insertSeriesTestEvent(t, ctx, pool, "Empty Storytime", emptyVenue, clock.FutureTimeAt(9, 18, 0))

	// Whitespace-only descriptions (spaces, tabs, newline) must fingerprint as
	// empty: one-argument btrim strips spaces only, so this pins the full
	// character set used by the query.
	whitespaceVenue := insertPlace(t, ctx, pool, "Whitespace Hall", "Toronto", "ON")
	insertSeriesTestEventWithDescription(t, ctx, pool, "Whitespace Storytime", "   ", whitespaceVenue, clock.FutureTimeAt(2, 18, 0))
	insertSeriesTestEventWithDescription(t, ctx, pool, "Whitespace Storytime", "\t\n", whitespaceVenue, clock.FutureTimeAt(9, 18, 0))

	now := time.Now().UTC()
	pairs, err := repo.ListSeriesCandidatePairs(ctx, events.SeriesCandidatesQuery{
		WindowStart: seriesTestWindowStart(now, 14),
		WindowEnd:   seriesTestWindowEnd(now, 90),
		Now:         now,
	})
	require.NoError(t, err)
	require.NotEmpty(t, pairs)

	// Pair rows carry fingerprints, never the description text.
	for _, pair := range pairs {
		for _, member := range []events.SeriesCandidateMember{pair.A, pair.B} {
			require.NotContains(t, member.DescriptionFingerprint, "Weekly storytime",
				"storage must not carry description text into pair rows")
			if member.VenueULID == emptyVenue.ULID || member.VenueULID == whitespaceVenue.ULID {
				require.Empty(t, member.DescriptionFingerprint, "blank description yields an empty fingerprint")
				continue
			}
			require.Len(t, member.DescriptionFingerprint, 32, "non-empty descriptions are md5 fingerprints")
		}
	}

	result := events.BuildSeriesCandidates(pairs, events.SeriesCandidatesParams{MinEvents: 2, Limit: 50})
	require.Len(t, result.Items, 4)

	byVenue := make(map[string]events.SeriesCandidate, len(result.Items))
	for _, cluster := range result.Items {
		byVenue[cluster.VenueID] = cluster
	}

	trimmed, ok := byVenue[trimmedVenue.ULID]
	require.True(t, ok)
	require.Equal(t, 1, trimmed.DistinctDescriptionCount, "trimmed-identical descriptions collapse")
	require.False(t, trimmed.ContentVaries)

	punct, ok := byVenue[punctVenue.ULID]
	require.True(t, ok)
	require.Equal(t, 2, punct.DistinctDescriptionCount, "punctuation changes are distinct")
	require.True(t, punct.ContentVaries)

	empty, ok := byVenue[emptyVenue.ULID]
	require.True(t, ok)
	require.Equal(t, 0, empty.DistinctDescriptionCount)
	require.False(t, empty.ContentVaries)

	whitespaceOnly, ok := byVenue[whitespaceVenue.ULID]
	require.True(t, ok)
	require.Equal(t, 0, whitespaceOnly.DistinctDescriptionCount, "whitespace-only descriptions fingerprint as empty")
	require.False(t, whitespaceOnly.ContentVaries, "blank descriptions are not content divergence")
}

type seriesTestEvent struct {
	ulid    string
	eventID string
}

func insertSeriesTestEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, place seededEntity, start time.Time) string {
	t.Helper()
	event := insertSeriesTestEventOnly(t, ctx, pool, name, place)
	insertSeriesTestOccurrence(t, ctx, pool, event.eventID, start, place, nil)
	return event.ulid
}

func insertSeriesTestEventOnly(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, place seededEntity) seriesTestEvent {
	t.Helper()
	ulidValue := ulid.Make().String()
	var eventID string
	err := pool.QueryRow(ctx,
		`INSERT INTO events (ulid, name, event_domain, lifecycle_state, primary_venue_id)
		 VALUES ($1, $2, 'arts', 'published', $3) RETURNING id`,
		ulidValue, name, place.ID,
	).Scan(&eventID)
	require.NoError(t, err)
	return seriesTestEvent{ulid: ulidValue, eventID: eventID}
}

func insertSeriesTestEventWithDescription(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, description string, place seededEntity, start time.Time) string {
	t.Helper()
	ulidValue := ulid.Make().String()
	var eventID string
	err := pool.QueryRow(ctx,
		`INSERT INTO events (ulid, name, description, event_domain, lifecycle_state, primary_venue_id)
		 VALUES ($1, $2, $3, 'arts', 'published', $4) RETURNING id`,
		ulidValue, name, description, place.ID,
	).Scan(&eventID)
	require.NoError(t, err)
	insertSeriesTestOccurrence(t, ctx, pool, eventID, start, place, nil)
	return ulidValue
}

func insertSeriesTestEventWithSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, place seededEntity, start time.Time, sourceID string) string {
	t.Helper()
	event := insertSeriesTestEventOnly(t, ctx, pool, name, place)
	insertSeriesTestOccurrence(t, ctx, pool, event.eventID, start, place, nil)
	insertSeriesTestEventSource(t, ctx, pool, event.eventID, sourceID)
	return event.ulid
}

func insertSeriesTestEventSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID, sourceID string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO event_sources (event_id, source_id, source_url, payload, payload_hash)
		 VALUES ($1, $2, $3, '{}'::jsonb, $4)`,
		eventID, sourceID, "https://example.com/events/"+eventID+"/"+sourceID, ulid.Make().String(),
	)
	require.NoError(t, err)
}

func insertSeriesTestSource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, baseURL *string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO sources (name, source_type, base_url, license_url, license_type)
		 VALUES ($1, 'scraper', $2, 'https://example.com/license', 'CC0') RETURNING id`,
		name, baseURL,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertSeriesTestOccurrence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, start time.Time, place seededEntity, statusOverride *string) {
	t.Helper()
	// Use UTC so the generated local_date/local_start_time columns are stable
	// across DST boundaries for a 7-day cadence.
	_, err := pool.Exec(ctx,
		`INSERT INTO event_occurrences (event_id, start_time, end_time, timezone, venue_id, status_override)
		 VALUES ($1, $2, $3, 'UTC', $4, $5)`,
		eventID, start, start.Add(time.Hour), place.ID, statusOverride,
	)
	require.NoError(t, err)
}

func memberULIDs(cluster events.SeriesCandidate) []string {
	ulids := make([]string, 0, len(cluster.Members))
	for _, member := range cluster.Members {
		ulids = append(ulids, member.ULID)
	}
	return ulids
}

func requireSeriesSimilarityRange(t *testing.T, ctx context.Context, pool *pgxpool.Pool, a, b string, min, max float64) {
	t.Helper()
	var similarity float64
	err := pool.QueryRow(ctx,
		`SELECT similarity(normalize_name($1), normalize_name($2))`, a, b,
	).Scan(&similarity)
	require.NoError(t, err)
	require.GreaterOrEqual(t, similarity, min, "%q vs %q similarity", a, b)
	require.Less(t, similarity, max, "%q vs %q similarity", a, b)
}

func seriesTestWindowStart(now time.Time, lookbackDays int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()-lookbackDays, 0, 0, 0, 0, time.UTC)
}

func seriesTestWindowEnd(now time.Time, days int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, time.UTC)
}
