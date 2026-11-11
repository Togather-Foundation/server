package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/tests/testhelpers/clock"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// insertFeedAttribution attaches one event to a source row as a feed ingestion.
func insertFeedAttribution(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventULID, sourceID, feedURL, sourceEventID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO event_sources (event_id, source_id, source_url, source_event_id, retrieved_at, payload, payload_hash)
VALUES ((SELECT id FROM events WHERE ulid = $1), $2, $3, NULLIF($4, ''), $5, '{}', $6)
`, eventULID, sourceID, feedURL, sourceEventID, time.Now().UTC(), "hash-"+eventULID)
	require.NoError(t, err)
}

func TestFindByFeedEventID(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	org := insertOrganization(t, ctx, pool, "Feed Identity Org")
	place := insertPlace(t, ctx, pool, "Feed Identity Hall", "Toronto", "ON")
	start := clock.FutureTime(3)

	const (
		feedURL       = "https://calendar.example.org/calendar/ical/cita/public/basic.ics"
		sourceEventID = "evt_R20250311T190000@example.org:2026-11-10T15:00:00-05:00"
	)

	// Seed historical fragmentation directly. The current registry prevents new
	// duplicates, but event lookup must continue to work across legacy rows.
	historicalSource := func(name string) string {
		t.Helper()
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,trust_level,license_url,license_type) VALUES ($1,'api','https://calendar.example.org',5,'https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id::text`, name).Scan(&id))
		return id
	}
	sourceA := historicalSource("Google Calendar")
	sourceB := historicalSource("cita-local-events")
	require.NotEqual(t, sourceA, sourceB, "fixture must preserve historical row fragmentation")

	t.Run("matches on feed URL and source event id", func(t *testing.T) {
		ulidValue := insertEvent(t, ctx, pool, "GW Focus Group", "Journal club", org, place, "education", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidValue, sourceA, feedURL, sourceEventID)

		found, err := repo.FindByFeedEventID(ctx, feedURL, sourceEventID)
		require.NoError(t, err)
		require.Equal(t, ulidValue, found.ULID)
	})

	t.Run("same source event id from a different feed does not match", func(t *testing.T) {
		// Both feeds live on the same host and are attributed by the same source
		// row; only the feed URL distinguishes them.
		otherFeed := "https://calendar.example.org/calendar/ical/seminars/public/basic.ics"
		ulidValue := insertEvent(t, ctx, pool, "Gravitational Waves: Michael Fedderke", "Research talk", org, place, "education", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidValue, sourceA, otherFeed, sourceEventID)

		found, err := repo.FindByFeedEventID(ctx, otherFeed, sourceEventID)
		require.NoError(t, err)
		require.Equal(t, ulidValue, found.ULID, "the other feed must resolve to its own event")

		found, err = repo.FindByFeedEventID(ctx, feedURL, sourceEventID)
		require.NoError(t, err)
		require.NotEqual(t, ulidValue, found.ULID, "a different feed must not steal this feed's event")
	})

	t.Run("prefers a live twin over a soft-deleted one", func(t *testing.T) {
		tombstoneFeed := "https://calendar.example.org/calendar/ical/tombstone/public/basic.ics"

		// Older live row: created first, so creation order alone would pick it.
		liveULID := insertEvent(t, ctx, pool, "GW Focus Group", "Journal club", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, liveULID, sourceA, tombstoneFeed, sourceEventID)
		_, err := pool.Exec(ctx, `UPDATE events SET created_at = now() - interval '1 hour' WHERE ulid = $1`, liveULID)
		require.NoError(t, err)

		tombstonedULID := insertEvent(t, ctx, pool, "Tombstoned Meetup", "Old row", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, tombstonedULID, sourceB, tombstoneFeed, sourceEventID)
		_, err = pool.Exec(ctx, `UPDATE events SET lifecycle_state = 'deleted', deleted_at = now() WHERE ulid = $1`, tombstonedULID)
		require.NoError(t, err)

		found, err := repo.FindByFeedEventID(ctx, tombstoneFeed, sourceEventID)
		require.NoError(t, err)
		require.Equal(t, liveULID, found.ULID, "prefers the live twin when both rows match the key")
	})

	t.Run("prefers a published event over a draft with the same key", func(t *testing.T) {
		draftFirstFeed := "https://calendar.example.org/calendar/ical/draft-first/public/basic.ics"

		draftULID := insertEvent(t, ctx, pool, "Draft Meetup", "Draft", org, place, "community", "draft", nil, start)
		insertFeedAttribution(t, ctx, pool, draftULID, sourceA, draftFirstFeed, sourceEventID)
		_, err := pool.Exec(ctx, `UPDATE events SET created_at = now() - interval '1 hour' WHERE ulid = $1`, draftULID)
		require.NoError(t, err)

		publishedULID := insertEvent(t, ctx, pool, "Published Meetup", "Published", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, publishedULID, sourceB, draftFirstFeed, sourceEventID)

		found, err := repo.FindByFeedEventID(ctx, draftFirstFeed, sourceEventID)
		require.NoError(t, err)
		require.Equal(t, publishedULID, found.ULID)
	})

	t.Run("legacy trailing slash: stored with slash, incoming without", func(t *testing.T) {
		// Every event_sources row stores the normalised form (query stripped);
		// many written before this rule carry a trailing slash. The lookup must
		// still find them without a backfill.
		stored := "https://legacy.example.org/eventslist/"
		incoming := "https://legacy.example.org/eventslist"

		ulidValue := insertEvent(t, ctx, pool, "Legacy Listing", "Stored with a slash", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidValue, sourceA, stored, sourceEventID)

		found, err := repo.FindByFeedEventID(ctx, incoming, sourceEventID)
		require.NoError(t, err)
		require.Equal(t, ulidValue, found.ULID, "a trailing slash must not make a legacy row invisible")
	})

	t.Run("legacy trailing slash: stored without slash, incoming with", func(t *testing.T) {
		stored := "https://legacy.example.org/program"
		incoming := "https://legacy.example.org/program/"

		ulidValue := insertEvent(t, ctx, pool, "Program Feed", "Stored without a slash", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidValue, sourceA, stored, sourceEventID)

		found, err := repo.FindByFeedEventID(ctx, incoming, sourceEventID)
		require.NoError(t, err)
		require.Equal(t, ulidValue, found.ULID, "the comparison must ignore trailing slashes on both sides")
	})

	t.Run("different feed path does not match", func(t *testing.T) {
		// Only trailing slashes are ignored; the rest of the stored path is
		// compared as-is, so two feeds on one host stay distinct.
		feedOne := "https://legacy.example.org/eventslist"
		feedTwo := "https://legacy.example.org/eventslist/archive"

		ulidValue := insertEvent(t, ctx, pool, "Listed Event", "Feed one", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidValue, sourceA, feedOne, sourceEventID)

		_, err := repo.FindByFeedEventID(ctx, feedTwo, sourceEventID)
		require.ErrorIs(t, err, events.ErrNotFound)
	})

	t.Run("provenance URL is stored once and upserts in place", func(t *testing.T) {
		stored := "https://legacy.example.org/eventslist/"
		ulidValue := insertEvent(t, ctx, pool, "Verbatim Listing", "Provenance check", org, place, "community", "published", nil, start)
		var eventID string
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidValue).Scan(&eventID))

		params := events.EventSourceCreateParams{
			EventID:       eventID,
			SourceID:      sourceB,
			SourceURL:     stored,
			SourceEventID: sourceEventID,
			Payload:       []byte("{}"),
			PayloadHash:   "verbatim-hash",
		}
		require.NoError(t, repo.CreateSource(ctx, params))
		// A re-scrape writes the same normalised URL; it must refresh the existing
		// row rather than insert a second one for the same upsert key.
		require.NoError(t, repo.CreateSource(ctx, params))

		var rows int
		var sourceURL string
		require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*), max(source_url) FROM event_sources WHERE event_id = $1 AND source_id = $2
`, eventID, sourceB).Scan(&rows, &sourceURL))
		require.Equal(t, 1, rows, "re-scrape must not add a second provenance row")
		require.Equal(t, stored, sourceURL, "the stored URL must not be rewritten")
	})

	t.Run("tied duplicates resolve deterministically by id", func(t *testing.T) {
		// Two live published rows share the key and the same created_at, so only
		// the id tie-break decides. The ULID text order matches PostgreSQL's
		// uuid byte order, so the smaller id is the one `e.id ASC` picks.
		tiedFeed := "https://calendar.example.org/calendar/ical/tied/public/basic.ics"

		ulidA := insertEvent(t, ctx, pool, "Tied Meetup A", "Tie", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidA, sourceA, tiedFeed, sourceEventID)
		ulidB := insertEvent(t, ctx, pool, "Tied Meetup B", "Tie", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidB, sourceB, tiedFeed, sourceEventID)

		var idA, idB string
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidA).Scan(&idA))
		require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM events WHERE ulid = $1`, ulidB).Scan(&idB))
		_, err := pool.Exec(ctx, `UPDATE events SET created_at = now() - interval '2 hours' WHERE ulid IN ($1, $2)`, ulidA, ulidB)
		require.NoError(t, err)

		wantULID := ulidA
		if idB < idA {
			wantULID = ulidB
		}

		for i := 0; i < 3; i++ {
			found, err := repo.FindByFeedEventID(ctx, tiedFeed, sourceEventID)
			require.NoError(t, err)
			require.Equal(t, wantULID, found.ULID, "tie-break must not depend on row order")
		}
	})

	t.Run("empty arguments return ErrNotFound", func(t *testing.T) {
		_, err := repo.FindByFeedEventID(ctx, "", sourceEventID)
		require.ErrorIs(t, err, events.ErrNotFound)

		_, err = repo.FindByFeedEventID(ctx, feedURL, "")
		require.ErrorIs(t, err, events.ErrNotFound)
	})
}
