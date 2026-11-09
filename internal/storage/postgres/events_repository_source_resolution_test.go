package postgres

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/tests/testhelpers/clock"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func sourceParams(name, baseURL, feedURL string) events.SourceLookupParams {
	return events.SourceLookupParams{
		Name:        name,
		SourceType:  "api",
		BaseURL:     baseURL,
		FeedURL:     feedURL,
		TrustLevel:  5,
		LicenseURL:  "https://creativecommons.org/publicdomain/zero/1.0/",
		LicenseType: "CC0",
	}
}

func countSources(t *testing.T, ctx context.Context, pool *pgxpool.Pool, baseURL string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE base_url = $1`, baseURL).Scan(&n))
	return n
}

// TestGetOrCreateSource_ResolvesByFeedURL guards the rule that a feed has one
// provenance row: renaming a config must reuse the row that already owns the
// feed instead of minting a twin. Before this, one feed ingested under two
// names created two `sources` rows (the staging rows "Meetup", "Google
// Calendar", "Music TORONTO Events" and others hold 1,630 / 703 / 157 / ...
// events alongside the configured rows for the same feeds).
func TestGetOrCreateSource_ResolvesByFeedURL(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	org := insertOrganization(t, ctx, pool, "Source Resolution Org")
	place := insertPlace(t, ctx, pool, "Source Resolution Hall", "Toronto", "ON")
	start := clock.FutureTime(3)

	// newEventWithAttribution creates an event attributed to sourceID at feedURL,
	// which is what makes that source row the owner of the feed.
	newEventWithAttribution := func(t *testing.T, sourceID, feedURL, sourceEventID string) {
		t.Helper()
		ulidValue := insertEvent(t, ctx, pool, "Feed Event", "Attribution", org, place, "community", "published", nil, start)
		insertFeedAttribution(t, ctx, pool, ulidValue, sourceID, feedURL, sourceEventID)
	}

	t.Run("renamed config reuses the row that owns the feed", func(t *testing.T) {
		const base = "https://renamed.example.org"
		const feed = "https://renamed.example.org/events/ical/"

		original, err := repo.GetOrCreateSource(ctx, sourceParams("feed-under-old-name", base, ""))
		require.NoError(t, err)
		newEventWithAttribution(t, original, feed, "evt-1")

		resolved, err := repo.GetOrCreateSource(ctx, sourceParams("feed-under-new-name", base, feed))
		require.NoError(t, err)
		require.Equal(t, original, resolved, "a rename must reuse the feed's existing provenance row")
		require.Equal(t, 1, countSources(t, ctx, pool, base), "renaming a config must not create a second sources row")
	})

	t.Run("configured name wins over the product's feed owner", func(t *testing.T) {
		const base = "https://precedence.example.org"
		const feed = "https://precedence.example.org/calendar.ics"

		catchAll, err := repo.GetOrCreateSource(ctx, sourceParams("Catch All Events", base, ""))
		require.NoError(t, err)
		newEventWithAttribution(t, catchAll, feed, "evt-2")

		configured, err := repo.GetOrCreateSource(ctx, sourceParams("configured-feed", base, ""))
		require.NoError(t, err)
		require.NotEqual(t, catchAll, configured)

		// The configured row exists, so it owns its own name even though another
		// row has already ingested this feed.
		resolved, err := repo.GetOrCreateSource(ctx, sourceParams("configured-feed", base, feed))
		require.NoError(t, err)
		require.Equal(t, configured, resolved, "an existing (base_url, name) row must not be replaced by the feed owner")
	})

	t.Run("unknown rows are not adopted", func(t *testing.T) {
		const base = "https://unknown.example.org"
		const feed = "https://unknown.example.org/feed.xml"

		unnamed, err := repo.GetOrCreateSource(ctx, sourceParams("unknown", base, ""))
		require.NoError(t, err)
		newEventWithAttribution(t, unnamed, feed, "evt-3")

		named, err := repo.GetOrCreateSource(ctx, sourceParams("named-config", base, feed))
		require.NoError(t, err)
		require.NotEqual(t, unnamed, named, "adopting the unknown row would relabel a named feed as unknown")
	})

	t.Run("an unrelated feed still creates its own row", func(t *testing.T) {
		const base = "https://two-feeds.example.org"
		const feedOne = "https://two-feeds.example.org/one.ics"
		const feedTwo = "https://two-feeds.example.org/two.ics"

		one, err := repo.GetOrCreateSource(ctx, sourceParams("feed-one", base, ""))
		require.NoError(t, err)
		newEventWithAttribution(t, one, feedOne, "evt-4")

		two, err := repo.GetOrCreateSource(ctx, sourceParams("feed-two", base, feedTwo))
		require.NoError(t, err)
		require.NotEqual(t, one, two, "a different feed on the same host is a different provenance row")
		require.Equal(t, 2, countSources(t, ctx, pool, base))
	})

	t.Run("the row with the most attributions wins when several own the feed", func(t *testing.T) {
		const base = "https://split-owner.example.org"
		const feed = "https://split-owner.example.org/events"

		older, err := repo.GetOrCreateSource(ctx, sourceParams("older-owner", base, ""))
		require.NoError(t, err)
		newEventWithAttribution(t, older, feed, "evt-5a")

		bigger, err := repo.GetOrCreateSource(ctx, sourceParams("bigger-owner", base, ""))
		require.NoError(t, err)
		newEventWithAttribution(t, bigger, feed, "evt-5b")
		newEventWithAttribution(t, bigger, feed, "evt-5c")

		resolved, err := repo.GetOrCreateSource(ctx, sourceParams("third-name", base, feed))
		require.NoError(t, err)
		require.Equal(t, bigger, resolved, "resolution must be deterministic and prefer the row that owns more of the feed")
	})
}

// TestGetOrCreateSource_IsIdempotentWithFeedURL is the concurrency guard: the
// insert still uses ON CONFLICT so repeated resolution of the same new name
// yields one row.
func TestGetOrCreateSource_IsIdempotentWithFeedURL(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := &EventRepository{pool: pool, logger: zerolog.Nop()}

	const base = "https://repeat.example.org"
	const feed = "https://repeat.example.org/events"

	var first string
	for i := 0; i < 3; i++ {
		id, err := repo.GetOrCreateSource(ctx, sourceParams("repeat-config", base, feed))
		require.NoError(t, err)
		if i == 0 {
			first = id
		}
		require.Equal(t, first, id)
	}
	require.Equal(t, 1, countSources(t, ctx, pool, base))

	// The feed→row link comes from recorded attributions, so give the row one
	// before asserting that a later rename reuses it.
	org := insertOrganization(t, ctx, pool, "Repeat Org")
	place := insertPlace(t, ctx, pool, "Repeat Hall", "Toronto", "ON")
	ulidValue := insertEvent(t, ctx, pool, "Repeat Event", "Attribution", org, place, "community", "published", nil, clock.FutureTime(3))
	insertFeedAttribution(t, ctx, pool, ulidValue, first, feed, "evt-repeat")

	// And a later rename on the same feed lands on the same row.
	renamed, err := repo.GetOrCreateSource(ctx, sourceParams("repeat-config-v2", base, feed))
	require.NoError(t, err)
	require.Equal(t, first, renamed)
	require.Equal(t, 1, countSources(t, ctx, pool, base))
}
