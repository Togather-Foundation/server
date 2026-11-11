package postgres

import (
	"context"
	"testing"

	"github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/stretchr/testify/require"
)

// Exercise the same operation as `server scrape sync`, including config storage,
// URL-list precedence, registry registration, and rename before the first event.
func TestScraperSourceSync_RenameBeforeAttribution(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)
	params := scraper.UpsertParams{Name: "original", URL: "ignored-metadata", URLs: []string{"https://sync.example.org/feed.ics?view=1", "https://sync.example.org/other.ics"}, Schedule: "manual", TrustLevel: 8, License: "CC-BY-4.0", Enabled: true, MaxPages: 5, SuppressReviewWarnings: []string{"missing_image"}}
	_, err := repo.SyncConfig(ctx, params)
	require.NoError(t, err)
	var original string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM sources WHERE feed_url='https://sync.example.org/feed.ics'`).Scan(&original))
	params.Name = "renamed"
	params.URLs[0] = "https://sync.example.org/feed.ics/#part"
	_, err = repo.SyncConfig(ctx, params)
	require.NoError(t, err)
	var renamed string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM sources WHERE feed_url='https://sync.example.org/feed.ics'`).Scan(&renamed))
	require.Equal(t, original, renamed)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources`).Scan(&n))
	require.Equal(t, 2, n, "each actual feed has one row across a config rename")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM event_sources`).Scan(&n))
	require.Zero(t, n, "rename identity must work before any attribution")

	// A conflicting query-view policy must be reported instead of silently
	// succeeding with a different canonical trust/license. Config policy remains
	// stored for operator reconciliation, and canonical policy is preserved.
	view := params
	view.Name = "query-view"
	view.URLs = []string{"https://sync.example.org/feed.ics?view=2"}
	view.TrustLevel = 3
	view.License = "CC0-1.0"
	view.SuppressReviewWarnings = []string{"missing_description"}
	_, err = repo.SyncConfig(ctx, view)
	require.ErrorContains(t, err, "source policy conflict")
	stored, err := repo.GetByName(ctx, view.Name)
	require.NoError(t, err)
	require.Equal(t, 3, stored.TrustLevel)
	require.Equal(t, view.License, stored.License)
	require.Equal(t, view.SuppressReviewWarnings, stored.SuppressReviewWarnings)
	var trust int
	var license string
	require.NoError(t, pool.QueryRow(ctx, `SELECT trust_level,license_type FROM sources WHERE id=$1`, original).Scan(&trust, &license))
	require.Equal(t, 8, trust)
	require.Equal(t, "CC-BY", license)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources`).Scan(&n))
	require.Equal(t, 2, n)
}

func TestScraperSourceSync_ConfiguredPolicyWinsOverOlderGeneric(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)
	const feed = "https://policy.example.org/feed.ics"
	var generic, configured string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,trust_level,license_url,license_type,created_at) VALUES ('generic','api',$1,5,'CC0-1.0','CC0',now()-interval '1 day') RETURNING id::text`, feed).Scan(&generic))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sources(name,source_type,base_url,trust_level,license_url,license_type) VALUES ('configured','scraper',$1,9,'https://creativecommons.org/publicdomain/zero/1.0/','CC0') RETURNING id::text`, feed).Scan(&configured))
	params := scraper.UpsertParams{Name: "configured", URL: feed, Schedule: "manual", TrustLevel: 9, License: "CC0-1.0", Enabled: true, MaxPages: 5}
	_, err := repo.SyncConfig(ctx, params)
	require.NoError(t, err, "CC0 URI and alias have the same policy")
	var chosen string
	require.NoError(t, pool.QueryRow(ctx, `SELECT id::text FROM sources WHERE feed_url=$1`, feed).Scan(&chosen))
	require.Equal(t, configured, chosen, "configured trust9 wins over oldest generic trust5")
	require.NotEqual(t, generic, chosen)
	params.TrustLevel = 5
	_, err = repo.SyncConfig(ctx, params)
	require.ErrorContains(t, err, "source policy conflict", "existing canonical policy mismatch must be explicit")
}

func TestScraperSourceSync_ConflictingConfiguredPoliciesRefused(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)
	const feed = "https://ambiguous.example.org/feed.ics"
	for i, name := range []string{"one", "two"} {
		trust := 5 + i
		_, err := repo.Upsert(ctx, scraper.UpsertParams{Name: name, URL: feed, Schedule: "manual", TrustLevel: trust, License: "CC0-1.0", Enabled: true, MaxPages: 5})
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO sources(name,source_type,base_url,trust_level,license_url,license_type) VALUES ($1,'scraper',$2,$3,'CC0-1.0','CC0')`, name, feed, trust)
		require.NoError(t, err)
	}
	_, err := repo.SyncConfig(ctx, scraper.UpsertParams{Name: "one", URL: feed, Schedule: "manual", TrustLevel: 5, License: "CC0-1.0", Enabled: true, MaxPages: 5})
	require.ErrorContains(t, err, "source policy conflict")
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sources WHERE feed_url IS NOT NULL`).Scan(&n))
	require.Zero(t, n, "ambiguous policies must not claim a row")
}

func TestSourceIdentityMigration_PreviousSlotUpsertRemainsUsable(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	// This is the previous active binary's INSERT conflict target. Migration60
	// runs before traffic switches, so the statement must remain valid afterwards.
	for range 2 {
		_, err := pool.Exec(ctx, `INSERT INTO sources(name,source_type,base_url,trust_level,license_url,license_type) VALUES ('old-slot','api','https://old-slot.example.org',5,'CC0-1.0','CC0') ON CONFLICT (base_url,name) WHERE base_url IS NOT NULL DO UPDATE SET name=EXCLUDED.name RETURNING id`)
		require.NoError(t, err)
	}
}

func TestSourceLicenseKey_PreservesPolicyVersionAndVariant(t *testing.T) {
	require.Equal(t, sourceLicenseKey("CC0-1.0"), sourceLicenseKey("https://creativecommons.org/publicdomain/zero/1.0/"))
	require.Equal(t, sourceLicenseKey("CC-BY-4.0"), sourceLicenseKey("https://creativecommons.org/licenses/by/4.0/"))
	require.NotEqual(t, sourceLicenseKey("CC-BY-3.0"), sourceLicenseKey("CC-BY-4.0"))
	require.NotEqual(t, sourceLicenseKey("CC-BY-SA-4.0"), sourceLicenseKey("CC-BY-4.0"))
}
