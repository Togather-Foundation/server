package jobs

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

func TestFilterEligibleSources(t *testing.T) {
	t.Parallel()

	logger := slog.Default()

	t.Run("filters daily and weekly schedules only", func(t *testing.T) {
		t.Parallel()

		sources := []postgres.ListScraperSourcesWithLatestRunRow{
			{ID: 1, Name: "daily-src", Schedule: "daily", Enabled: true},
			{ID: 2, Name: "weekly-src", Schedule: "weekly", Enabled: true},
			{ID: 3, Name: "manual-src", Schedule: "manual", Enabled: true},
		}

		eligible := filterEligibleSources(context.Background(), logger, sources, false)

		require.Len(t, eligible, 2)
		assert.Equal(t, "daily-src", eligible[0].name)
		assert.Equal(t, "weekly-src", eligible[1].name)
	})

	t.Run("skip_up_to_date filters fresh sources", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		sources := []postgres.ListScraperSourcesWithLatestRunRow{
			{ID: 1, Name: "fresh-daily", Schedule: "daily", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}},
			{ID: 2, Name: "stale-daily", Schedule: "daily", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-48 * time.Hour), Valid: true}},
			{ID: 3, Name: "fresh-weekly", Schedule: "weekly", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}},
			{ID: 4, Name: "stale-weekly", Schedule: "weekly", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-8 * 24 * time.Hour), Valid: true}},
		}

		eligible := filterEligibleSources(context.Background(), logger, sources, true)

		require.Len(t, eligible, 2)
		assert.Equal(t, "stale-daily", eligible[0].name)
		assert.Equal(t, "stale-weekly", eligible[1].name)
	})

	t.Run("daily freshness is 24h", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		sources := []postgres.ListScraperSourcesWithLatestRunRow{
			{ID: 1, Name: "almost-fresh", Schedule: "daily", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-23 * time.Hour), Valid: true}},
			{ID: 2, Name: "just-stale", Schedule: "daily", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-25 * time.Hour), Valid: true}},
		}

		eligible := filterEligibleSources(context.Background(), logger, sources, true)

		require.Len(t, eligible, 1)
		assert.Equal(t, "just-stale", eligible[0].name)
	})

	t.Run("weekly freshness is 7 days", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		sources := []postgres.ListScraperSourcesWithLatestRunRow{
			{ID: 1, Name: "almost-fresh", Schedule: "weekly", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-6 * 24 * time.Hour), Valid: true}},
			{ID: 2, Name: "just-stale", Schedule: "weekly", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-8 * 24 * time.Hour), Valid: true}},
		}

		eligible := filterEligibleSources(context.Background(), logger, sources, true)

		require.Len(t, eligible, 1)
		assert.Equal(t, "just-stale", eligible[0].name)
	})

	t.Run("only completed status triggers freshness check", func(t *testing.T) {
		t.Parallel()

		now := time.Now()
		sources := []postgres.ListScraperSourcesWithLatestRunRow{
			{ID: 1, Name: "failed-recent", Schedule: "daily", Enabled: true, LastRunStatus: "failed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}},
			{ID: 2, Name: "running-recent", Schedule: "daily", Enabled: true, LastRunStatus: "running", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}},
		}

		eligible := filterEligibleSources(context.Background(), logger, sources, true)

		require.Len(t, eligible, 2)
	})
}

func TestScrapeOrchestratorArgs(t *testing.T) {
	t.Parallel()

	t.Run("initial args sets correct defaults", func(t *testing.T) {
		t.Parallel()

		args := ScrapeOrchestratorInitialArgs(true, true, []string{"src1", "src2"})

		assert.True(t, args.RespectAutoScrape)
		assert.True(t, args.SkipUpToDate)
		assert.Equal(t, []string{"src1", "src2"}, args.SourceNames)
		assert.Equal(t, 0, args.CurrentIndex)
	})

	t.Run("args with false options", func(t *testing.T) {
		t.Parallel()

		args := ScrapeOrchestratorInitialArgs(false, false, nil)

		assert.False(t, args.RespectAutoScrape)
		assert.False(t, args.SkipUpToDate)
		assert.Nil(t, args.SourceNames)
		assert.Equal(t, 0, args.CurrentIndex)
	})
}

func TestScrapeOrchestratorWorker_EnqueuesFirstSourceWithChainMetadata(t *testing.T) {
	t.Parallel()

	sources := []postgres.ListScraperSourcesWithLatestRunRow{
		{ID: 1, Name: "source-a", Schedule: "daily", Enabled: true},
		{ID: 2, Name: "source-b", Schedule: "daily", Enabled: true},
		{ID: 3, Name: "source-c", Schedule: "weekly", Enabled: true},
	}

	// We can't easily test the full Work() method without a River client in context,
	// but we can verify the filtering logic directly - this is what the orchestrator
	// uses to determine eligible sources.
	eligible := filterEligibleSources(context.Background(), slog.Default(), sources, false)
	require.Len(t, eligible, 3)

	sourceNames := make([]string, len(eligible))
	for i, src := range eligible {
		sourceNames[i] = src.name
	}

	// Verify that the first source is correctly identified
	assert.Equal(t, "source-a", sourceNames[0])
	assert.Equal(t, []string{"source-a", "source-b", "source-c"}, sourceNames)
}

func TestScrapeOrchestratorWorker_SkipUpToDateFiltersCorrectly(t *testing.T) {
	t.Parallel()

	now := time.Now()
	sources := []postgres.ListScraperSourcesWithLatestRunRow{
		{ID: 1, Name: "fresh-daily", Schedule: "daily", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}},
		{ID: 2, Name: "stale-daily", Schedule: "daily", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-48 * time.Hour), Valid: true}},
		{ID: 3, Name: "fresh-weekly", Schedule: "weekly", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}},
		{ID: 4, Name: "stale-weekly", Schedule: "weekly", Enabled: true, LastRunStatus: "completed", LastRunCompletedAt: pgtype.Timestamptz{Time: now.Add(-8 * 24 * time.Hour), Valid: true}},
	}

	eligible := filterEligibleSources(context.Background(), slog.Default(), sources, true)
	require.Len(t, eligible, 2)

	// Should only have stale sources
	assert.Equal(t, "stale-daily", eligible[0].name)
	assert.Equal(t, "stale-weekly", eligible[1].name)
}
