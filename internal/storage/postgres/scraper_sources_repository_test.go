package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/stretchr/testify/require"
)

// insertScraperSource is a test helper that upserts a minimal scraper source and
// returns the resulting domain Source (with its DB-assigned ID).
func insertScraperSource(t *testing.T, ctx context.Context, repo *ScraperSourceRepository, name string, tier int, enabled bool) *scraper.Source {
	t.Helper()
	src, err := repo.Upsert(ctx, scraper.UpsertParams{
		Name:       name,
		URL:        "https://example.org/" + name,
		Tier:       tier,
		Schedule:   "manual",
		TrustLevel: 5,
		License:    "CC0-1.0",
		Enabled:    enabled,
		MaxPages:   10,
	})
	require.NoError(t, err)
	require.NotNil(t, src)
	return src
}

func TestScraperSourceRepositoryUpsert(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	// Insert
	src, err := repo.Upsert(ctx, scraper.UpsertParams{
		Name:       "test-source",
		URL:        "https://example.org/events",
		Tier:       0,
		Schedule:   "daily",
		TrustLevel: 7,
		License:    "CC0-1.0",
		Enabled:    true,
		MaxPages:   5,
		Notes:      "main feed",
	})
	require.NoError(t, err)
	require.NotNil(t, src)
	require.Positive(t, src.ID)
	require.Equal(t, "test-source", src.Name)
	require.Equal(t, "https://example.org/events", src.URL)
	require.Equal(t, 0, src.Tier)
	require.Equal(t, "daily", src.Schedule)
	require.Equal(t, 7, src.TrustLevel)
	require.Equal(t, "CC0-1.0", src.License)
	require.True(t, src.Enabled)
	require.Equal(t, 5, src.MaxPages)
	require.Equal(t, "main feed", src.Notes)
	require.Nil(t, src.LastScrapedAt)
	require.False(t, src.CreatedAt.IsZero())
	require.False(t, src.UpdatedAt.IsZero())

	firstID := src.ID
	createdAt := src.CreatedAt

	// Update (same name, changed fields)
	updated, err := repo.Upsert(ctx, scraper.UpsertParams{
		Name:       "test-source",
		URL:        "https://example.org/events-v2",
		Tier:       1,
		Schedule:   "weekly",
		TrustLevel: 8,
		License:    "CC0-1.0",
		Enabled:    false,
		MaxPages:   20,
		Notes:      "updated feed",
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	// ID is stable across upserts
	require.Equal(t, firstID, updated.ID)
	require.Equal(t, "https://example.org/events-v2", updated.URL)
	require.Equal(t, 1, updated.Tier)
	require.Equal(t, "weekly", updated.Schedule)
	require.Equal(t, 8, updated.TrustLevel)
	require.False(t, updated.Enabled)
	require.Equal(t, 20, updated.MaxPages)
	require.Equal(t, "updated feed", updated.Notes)
	// created_at is preserved; updated_at advances
	require.Equal(t, createdAt.UTC().Truncate(time.Millisecond),
		updated.CreatedAt.UTC().Truncate(time.Millisecond))
	// updated_at is set to NOW() on update — it must be >= created_at
	require.False(t, updated.UpdatedAt.Before(updated.CreatedAt))
}

func TestScraperSourceRepositoryUpsertWithLastScrapedAt(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	ts := time.Now().UTC().Truncate(time.Millisecond)
	src, err := repo.Upsert(ctx, scraper.UpsertParams{
		Name:          "scraped-source",
		URL:           "https://example.org/scraped",
		Tier:          0,
		Schedule:      "daily",
		TrustLevel:    5,
		License:       "CC0-1.0",
		Enabled:       true,
		MaxPages:      10,
		LastScrapedAt: &ts,
	})
	require.NoError(t, err)
	require.NotNil(t, src.LastScrapedAt)
	require.Equal(t, ts, src.LastScrapedAt.UTC().Truncate(time.Millisecond))
}

func TestScraperSourceRepositoryGetByName(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	inserted := insertScraperSource(t, ctx, repo, "lookup-source", 0, true)

	// Found
	found, err := repo.GetByName(ctx, "lookup-source")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, inserted.ID, found.ID)
	require.Equal(t, "lookup-source", found.Name)

	// Not found → ErrNotFound
	missing, err := repo.GetByName(ctx, "does-not-exist")
	require.ErrorIs(t, err, scraper.ErrNotFound)
	require.Nil(t, missing)
}

func TestScraperSourceRepositoryList(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	insertScraperSource(t, ctx, repo, "enabled-a", 0, true)
	insertScraperSource(t, ctx, repo, "enabled-b", 1, true)
	insertScraperSource(t, ctx, repo, "disabled-c", 0, false)

	// All sources (nil filter)
	all, err := repo.List(ctx, nil)
	require.NoError(t, err)
	require.Len(t, all, 3)

	// Only enabled
	enabledTrue := true
	enabled, err := repo.List(ctx, &enabledTrue)
	require.NoError(t, err)
	require.Len(t, enabled, 2)
	for _, s := range enabled {
		require.True(t, s.Enabled)
	}

	// Only disabled
	enabledFalse := false
	disabled, err := repo.List(ctx, &enabledFalse)
	require.NoError(t, err)
	require.Len(t, disabled, 1)
	require.False(t, disabled[0].Enabled)
}

func TestScraperSourceRepositoryUpdateLastScraped(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	insertScraperSource(t, ctx, repo, "run-source", 0, true)

	before := time.Now().UTC().Add(-time.Second)
	require.NoError(t, repo.UpdateLastScraped(ctx, "run-source"))

	updated, err := repo.GetByName(ctx, "run-source")
	require.NoError(t, err)
	require.NotNil(t, updated.LastScrapedAt)
	require.True(t, updated.LastScrapedAt.UTC().After(before),
		"last_scraped_at should be set to ~NOW()")
}

func TestScraperSourceRepositoryDelete(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	insertScraperSource(t, ctx, repo, "delete-source", 0, true)

	require.NoError(t, repo.Delete(ctx, "delete-source"))

	// Should be gone
	_, err := repo.GetByName(ctx, "delete-source")
	require.ErrorIs(t, err, scraper.ErrNotFound)
}

func TestScraperSourceRepositoryLinkToOrg(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	// org.ID is the UUID (PK); org.ULID is the ULID text field.
	// The join table org_scraper_sources.organization_id is UUID, so we use org.ID.
	org := insertOrganization(t, ctx, pool, "Test Org")
	src := insertScraperSource(t, ctx, repo, "org-source", 0, true)

	require.NoError(t, repo.LinkToOrg(ctx, org.ID, src.ID))

	sources, err := repo.ListByOrg(ctx, org.ID)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	require.Equal(t, src.ID, sources[0].ID)
	require.Equal(t, "org-source", sources[0].Name)
}

func TestScraperSourceRepositoryListByOrg(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	orgA := insertOrganization(t, ctx, pool, "Org A")
	orgB := insertOrganization(t, ctx, pool, "Org B")

	srcA := insertScraperSource(t, ctx, repo, "source-for-a", 0, true)
	srcB := insertScraperSource(t, ctx, repo, "source-for-b", 1, true)
	srcShared := insertScraperSource(t, ctx, repo, "shared-source", 0, true)

	require.NoError(t, repo.LinkToOrg(ctx, orgA.ID, srcA.ID))
	require.NoError(t, repo.LinkToOrg(ctx, orgA.ID, srcShared.ID))
	require.NoError(t, repo.LinkToOrg(ctx, orgB.ID, srcB.ID))
	require.NoError(t, repo.LinkToOrg(ctx, orgB.ID, srcShared.ID))

	forA, err := repo.ListByOrg(ctx, orgA.ID)
	require.NoError(t, err)
	require.Len(t, forA, 2)

	forB, err := repo.ListByOrg(ctx, orgB.ID)
	require.NoError(t, err)
	require.Len(t, forB, 2)

	// Org with no links returns empty slice (not error)
	orgC := insertOrganization(t, ctx, pool, "Org C")
	forC, err := repo.ListByOrg(ctx, orgC.ID)
	require.NoError(t, err)
	require.Empty(t, forC)
}

func TestScraperSourceRepositoryUnlinkFromOrg(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	org := insertOrganization(t, ctx, pool, "Unlink Org")
	src := insertScraperSource(t, ctx, repo, "unlink-source-org", 0, true)

	require.NoError(t, repo.LinkToOrg(ctx, org.ID, src.ID))
	require.NoError(t, repo.UnlinkFromOrg(ctx, org.ID, src.ID))

	sources, err := repo.ListByOrg(ctx, org.ID)
	require.NoError(t, err)
	require.Empty(t, sources)
}

func TestScraperSourceRepositoryLinkToPlace(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	// place.ID is the UUID (PK); place.ULID is the ULID text field.
	// The join table place_scraper_sources.place_id is UUID, so we use place.ID.
	place := insertPlace(t, ctx, pool, "Test Venue", "Toronto", "ON")
	src := insertScraperSource(t, ctx, repo, "place-source", 0, true)

	require.NoError(t, repo.LinkToPlace(ctx, place.ID, src.ID))

	sources, err := repo.ListByPlace(ctx, place.ID)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	require.Equal(t, src.ID, sources[0].ID)
	require.Equal(t, "place-source", sources[0].Name)
}

func TestScraperSourceRepositoryListByPlace(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	placeA := insertPlace(t, ctx, pool, "Venue A", "Toronto", "ON")
	placeB := insertPlace(t, ctx, pool, "Venue B", "Ottawa", "ON")

	srcA := insertScraperSource(t, ctx, repo, "source-for-place-a", 0, true)
	srcB := insertScraperSource(t, ctx, repo, "source-for-place-b", 1, true)
	srcShared := insertScraperSource(t, ctx, repo, "shared-place-source", 0, true)

	require.NoError(t, repo.LinkToPlace(ctx, placeA.ID, srcA.ID))
	require.NoError(t, repo.LinkToPlace(ctx, placeA.ID, srcShared.ID))
	require.NoError(t, repo.LinkToPlace(ctx, placeB.ID, srcB.ID))
	require.NoError(t, repo.LinkToPlace(ctx, placeB.ID, srcShared.ID))

	forA, err := repo.ListByPlace(ctx, placeA.ID)
	require.NoError(t, err)
	require.Len(t, forA, 2)

	forB, err := repo.ListByPlace(ctx, placeB.ID)
	require.NoError(t, err)
	require.Len(t, forB, 2)

	// Place with no links returns empty slice (not error)
	placeC := insertPlace(t, ctx, pool, "Venue C", "Toronto", "ON")
	forC, err := repo.ListByPlace(ctx, placeC.ID)
	require.NoError(t, err)
	require.Empty(t, forC)
}

func TestScraperSourceRepositoryUnlinkFromPlace(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	place := insertPlace(t, ctx, pool, "Unlink Venue", "Toronto", "ON")
	src := insertScraperSource(t, ctx, repo, "unlink-source-place", 0, true)

	require.NoError(t, repo.LinkToPlace(ctx, place.ID, src.ID))
	require.NoError(t, repo.UnlinkFromPlace(ctx, place.ID, src.ID))

	sources, err := repo.ListByPlace(ctx, place.ID)
	require.NoError(t, err)
	require.Empty(t, sources)
}

func TestScraperSourceRepositoryInvalidOrgID(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	_, err := repo.ListByOrg(ctx, "not-a-uuid")
	require.Error(t, err)

	err = repo.LinkToOrg(ctx, "not-a-uuid", 1)
	require.Error(t, err)

	err = repo.UnlinkFromOrg(ctx, "not-a-uuid", 1)
	require.Error(t, err)
}

func TestScraperSourceRepositoryInvalidPlaceID(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPostgres(t, ctx)
	repo := NewScraperSourceRepository(pool)

	_, err := repo.ListByPlace(ctx, "not-a-uuid")
	require.Error(t, err)

	err = repo.LinkToPlace(ctx, "not-a-uuid", 1)
	require.Error(t, err)

	err = repo.UnlinkFromPlace(ctx, "not-a-uuid", 1)
	require.Error(t, err)
}
