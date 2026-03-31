package integration_batch

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Togather-Foundation/server/internal/jobs"
	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChainScraper implements scraperSourceScraper and records each call.
type mockChainScraper struct {
	results map[string]scraper.ScrapeResult
	errs    map[string]error
	calls   []string
}

func (m *mockChainScraper) ScrapeSource(ctx context.Context, sourceName string, opts scraper.ScrapeOptions) (scraper.ScrapeResult, error) {
	m.calls = append(m.calls, sourceName)
	if err, ok := m.errs[sourceName]; ok && err != nil {
		return m.results[sourceName], err
	}
	return m.results[sourceName], nil
}

// setupScrapeChainTest creates a River client with scrape workers wired to a mock scraper,
// starts the workers, and returns cleanup helpers.
func setupScrapeChainTest(t *testing.T, mock *mockChainScraper) (*river.Client[pgx.Tx], func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	initShared(t)
	resetDatabase(t, sharedPool)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	workers := river.NewWorkers()
	queries := postgres.New(sharedPool)
	river.AddWorker[jobs.ScrapeOrchestratorArgs](workers, &jobs.ScrapeOrchestratorWorker{
		ConfigQueries: queries,
		SourcesReader: queries,
		Logger:        logger,
		Slot:          "test",
	})
	river.AddWorker[jobs.ScrapeSourceArgs](workers, jobs.ScrapeSourceWorker{
		Scraper:       mock,
		ConfigQueries: queries,
		Logger:        logger,
		Slot:          "test",
	})

	riverClient, err := river.NewClient(riverpgxv5.New(sharedPool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	require.NoError(t, err)

	err = riverClient.Start(ctx)
	require.NoError(t, err)

	cleanup := func() {
		_ = riverClient.Stop(context.Background())
	}
	t.Cleanup(cleanup)

	return riverClient, cleanup
}

// insertScraperSourceForChain inserts a minimal scraper source row directly via SQL.
func insertScraperSourceForChain(t *testing.T, name, url, schedule string, enabled bool) {
	t.Helper()
	_, err := sharedPool.Exec(sharedCtx(t), `
		INSERT INTO scraper_sources (name, url, tier, schedule, enabled)
		VALUES ($1, $2, 0, $3, $4)
		ON CONFLICT (name) DO UPDATE SET schedule = EXCLUDED.schedule, enabled = EXCLUDED.enabled, url = EXCLUDED.url
	`, name, url, schedule, enabled)
	require.NoError(t, err)
}

// sharedCtx returns a background context for DB operations in shared setup.
func sharedCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// awaitChainCompletion polls until all expected sources have been scraped or timeout.
func awaitChainCompletion(t *testing.T, mock *mockChainScraper, expectedSources []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(mock.calls) >= len(expectedSources) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("chain did not complete within %v: got %d calls, expected %d (%v)", timeout, len(mock.calls), len(expectedSources), mock.calls)
}

func TestScrapeChain_SerialProgression(t *testing.T) {
	mock := &mockChainScraper{
		results: make(map[string]scraper.ScrapeResult),
		errs:    make(map[string]error),
	}

	riverClient, _ := setupScrapeChainTest(t, mock)

	// Insert 3 dummy scraper sources with daily schedule (eligible for scraping).
	sources := []string{"chain-source-a", "chain-source-b", "chain-source-c"}
	for _, name := range sources {
		insertScraperSourceForChain(t, name, "http://example.com/"+name, "daily", true)
		mock.results[name] = scraper.ScrapeResult{
			SourceName:    name,
			EventsFound:   1,
			EventsCreated: 1,
		}
	}

	// Enqueue the orchestrator job with respect_auto_scrape=false so it bypasses
	// the global scraper_config check and proceeds directly to listing sources.
	_, err := riverClient.Insert(context.Background(), jobs.ScrapeOrchestratorInitialArgs(false, false, nil), nil)
	require.NoError(t, err)

	// Wait for the chain to complete — all 3 sources should be scraped serially.
	awaitChainCompletion(t, mock, sources, 15*time.Second)

	// Assert all sources were called.
	assert.ElementsMatch(t, sources, mock.calls, "all sources should have been scraped")

	// Assert serial ordering: each source should be called after the previous one.
	// Since the chain enqueues the next source only after the current one finishes,
	// the call order should match the source order (sorted by name in the DB query).
	require.Len(t, mock.calls, 3, "expected exactly 3 scrape calls")
	for i, name := range sources {
		assert.Equal(t, name, mock.calls[i], "source %d should be %s", i, name)
	}
}

func TestScrapeChain_ContinueOnFailure(t *testing.T) {
	mock := &mockChainScraper{
		results: make(map[string]scraper.ScrapeResult),
		errs:    make(map[string]error),
	}

	riverClient, _ := setupScrapeChainTest(t, mock)

	// Insert 3 sources; the middle one will fail.
	sources := []string{"fail-source-a", "fail-source-b", "fail-source-c"}
	for _, name := range sources {
		insertScraperSourceForChain(t, name, "http://example.com/"+name, "daily", true)
		mock.results[name] = scraper.ScrapeResult{
			SourceName:  name,
			EventsFound: 1,
		}
	}
	// Make the middle source fail.
	mock.errs["fail-source-b"] = assert.AnError

	// Enqueue the orchestrator.
	_, err := riverClient.Insert(context.Background(), jobs.ScrapeOrchestratorInitialArgs(false, false, nil), nil)
	require.NoError(t, err)

	// Wait for the chain to complete — all 3 sources should be attempted despite the failure.
	awaitChainCompletion(t, mock, sources, 15*time.Second)

	// Assert all sources were called (best-effort continue-on-failure).
	assert.ElementsMatch(t, sources, mock.calls, "all sources should be attempted despite failure")

	// Assert ordering is preserved.
	require.Len(t, mock.calls, 3, "expected exactly 3 scrape calls")
	for i, name := range sources {
		assert.Equal(t, name, mock.calls[i], "source %d should be %s", i, name)
	}
}
