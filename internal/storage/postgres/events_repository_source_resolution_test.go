package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/tests/testhelpers/clock"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func sourceParams(name, baseURL, feedURL string) events.SourceLookupParams {
	return events.SourceLookupParams{Name: name, SourceType: "api", BaseURL: baseURL, FeedURL: feedURL,
		TrustLevel: 5, LicenseURL: "https://creativecommons.org/publicdomain/zero/1.0/", LicenseType: "CC0"}
}

func TestGetOrCreateSource_ConcurrentNames(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	const feed = "https://concurrent.example.org/feed.ics"
	var wg sync.WaitGroup
	ids := make([]string, 12)
	errs := make([]error, len(ids))
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = repo.GetOrCreateSource(ctx, sourceParams(fmt.Sprintf("name-%d", i), "https://concurrent.example.org", feed))
		}(i)
	}
	wg.Wait()
	for i := range ids {
		require.NoError(t, errs[i])
		require.Equal(t, ids[0], ids[i])
	}
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE feed_url = $1`, feed).Scan(&n))
	require.Equal(t, 1, n)
}

func TestGetOrCreateSource_LegacyAdoptionPreservesPolicy(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	const feed = "https://legacy.example.org/feed.ics"
	var legacy string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources (name,source_type,base_url,license_url,license_type,trust_level,config) VALUES ('old label','scraper',$1,'https://creativecommons.org/licenses/by/4.0/','CC-BY',9,'{"keep":true}') RETURNING id::text`, feed+"/?view=one").Scan(&legacy))
	got, err := repo.GetOrCreateSource(ctx, sourceParams("new label", "https://legacy.example.org", feed))
	require.NoError(t, err)
	require.Equal(t, legacy, got)
	var name, base, canonical, license, kind string
	var trust int
	var config []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT name,base_url,feed_url,license_url,license_type,trust_level,config FROM sources WHERE id=$1`, got).Scan(&name, &base, &canonical, &license, &kind, &trust, &config))
	require.Equal(t, "new label", name)
	require.Equal(t, feed, base)
	require.Equal(t, feed, canonical)
	require.Equal(t, 9, trust)
	require.Equal(t, "CC-BY", kind)
	require.JSONEq(t, `{"keep":true}`, string(config))
	require.Equal(t, "https://creativecommons.org/licenses/by/4.0/", license)
}

func TestGetOrCreateSource_DoesNotAdoptCatchAll(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	const host = "https://catchall.example.org"
	var legacy string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,license_url,license_type,trust_level) VALUES ('legacy','api',$1,'https://creativecommons.org/publicdomain/zero/1.0/','CC0',5) RETURNING id::text`, host).Scan(&legacy))
	org := insertOrganization(t, ctx, pool, "Catchall Org")
	place := insertPlace(t, ctx, pool, "Catchall Hall", "Toronto", "ON")
	for i, feed := range []string{host, host + "/other.ics"} {
		event := insertEvent(t, ctx, pool, "Catchall Event", "Attribution", org, place, "community", "published", nil, clock.FutureTime(i+3))
		insertFeedAttribution(t, ctx, pool, event, legacy, feed, fmt.Sprintf("e-%d", i))
	}
	_, err := repo.GetOrCreateSource(ctx, sourceParams("legacy", host, host))
	require.ErrorContains(t, err, "source label collision")
	// A distinct canonical label remains possible without changing the legacy
	// row while the old blue/green slot still relies on its unique pair key.
	got, err := repo.GetOrCreateSource(ctx, sourceParams("canonical", host, host))
	require.NoError(t, err)
	require.NotEqual(t, legacy, got)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM event_sources WHERE source_id=$1`, legacy).Scan(&n))
	require.Equal(t, 2, n, "automatic resolution must not alter historical attribution")
}

// A feed must have an identity before it produces its first event.
func TestGetOrCreateSource_CanonicalBeforeAttribution(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewEventRepository(pool, zerolog.Nop())
	first, err := repo.GetOrCreateSource(ctx, sourceParams("before", "https://canonical.example.org", "https://canonical.example.org/feed.ics?view=1"))
	require.NoError(t, err)
	second, err := repo.GetOrCreateSource(ctx, sourceParams("renamed", "https://canonical.example.org", "https://canonical.example.org/feed.ics/#section"))
	require.NoError(t, err)
	require.Equal(t, first, second, "a rename before any events must reuse the same feed row")
	other, err := repo.GetOrCreateSource(ctx, sourceParams("renamed", "https://canonical.example.org", "https://canonical.example.org/other.ics"))
	require.NoError(t, err)
	require.NotEqual(t, first, other, "same label and host must not collapse different feed paths")
}
