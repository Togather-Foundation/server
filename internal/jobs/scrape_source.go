package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/Togather-Foundation/server/internal/scraper"
	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

const (
	// JobKindScrapeSource identifies periodic scrape jobs for a single source.
	JobKindScrapeSource = "scrape_source"
)

// ScrapeSourceArgs holds the job arguments for a periodic source scrape.
type ScrapeSourceArgs struct {
	SourceName string `json:"source_name"`
}

func (ScrapeSourceArgs) Kind() string { return JobKindScrapeSource }

// scraperConfigReader is the subset of postgres.Queries used by ScrapeSourceWorker.
type scraperConfigReader interface {
	GetScraperConfig(ctx context.Context) (postgres.ScraperConfig, error)
}

// scraperSourceScraper is the subset of scraper.Scraper used by ScrapeSourceWorker.
type scraperSourceScraper interface {
	ScrapeSource(ctx context.Context, sourceName string, opts scraper.ScrapeOptions) (scraper.ScrapeResult, error)
}

// ScrapeSourceWorker executes a single-source scrape as a River periodic job.
type ScrapeSourceWorker struct {
	river.WorkerDefaults[ScrapeSourceArgs]
	Scraper       scraperSourceScraper
	ConfigQueries scraperConfigReader
	Logger        *slog.Logger
}

func (w ScrapeSourceWorker) Work(ctx context.Context, job *river.Job[ScrapeSourceArgs]) error {
	if w.Scraper == nil {
		return fmt.Errorf("scrape_source worker: scraper not configured")
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sourceName := job.Args.SourceName

	// Read global scraper config. Proceed on read error (log only); fall back to
	// zero-value ScrapeOptions which use package-level defaults everywhere.
	opts := scraper.ScrapeOptions{}
	if w.ConfigQueries != nil {
		cfg, err := w.ConfigQueries.GetScraperConfig(ctx)
		if err != nil {
			logger.WarnContext(ctx, "scrape_source: failed to read scraper config, proceeding with defaults", "source", sourceName, "error", err)
		} else {
			if !cfg.AutoScrape {
				logger.DebugContext(ctx, "scrape_source: auto_scrape disabled, skipping", "source", sourceName)
				return nil
			}
			if cfg.MaxBatchSize > 0 {
				opts.Limit = int(cfg.MaxBatchSize)
			}
			if cfg.RequestTimeoutSeconds > 0 {
				opts.RequestTimeout = time.Duration(cfg.RequestTimeoutSeconds) * time.Second
			}
			if cfg.RateLimitMs > 0 {
				opts.RateLimitMs = cfg.RateLimitMs
			}
			// TODO: cfg.RetryMaxAttempts — wire into retry logic when a retry
			// wrapper is added to ScrapeSource (srv-ephoo follow-up).
			// TODO: cfg.MaxConcurrentSources — wire into ScrapeAll fan-out
			// concurrency when a semaphore is added there (srv-ephoo follow-up).
		}
	}

	logger.InfoContext(ctx, "scrape_source: starting periodic scrape", "source", sourceName, "attempt", job.Attempt)

	start := time.Now()
	result, err := w.Scraper.ScrapeSource(ctx, sourceName, opts)
	if err != nil {
		return fmt.Errorf("scrape_source %s: %w", sourceName, err)
	}

	logger.InfoContext(ctx, "scrape_source: periodic scrape completed",
		"source", sourceName,
		"events_found", result.EventsFound,
		"events_created", result.EventsCreated,
		"events_duplicate", result.EventsDuplicate,
		"duration", time.Since(start),
	)

	return nil
}

// NewPeriodicJobsFromSources returns the base periodic jobs plus one River
// PeriodicJob for every source whose Schedule is "daily" or "weekly" and
// whose Enabled flag is true.
//
// Schedules:
//   - "daily"  → every 24 hours
//   - "weekly" → every 7 days
func NewPeriodicJobsFromSources(sources []scraper.SourceConfig) []*river.PeriodicJob {
	jobs := NewPeriodicJobs()

	for _, src := range sources {
		if !src.Enabled {
			continue
		}

		var interval time.Duration
		switch src.Schedule {
		case "daily":
			interval = 24 * time.Hour
		case "weekly":
			interval = 7 * 24 * time.Hour
		default:
			// "manual" or unknown — skip
			continue
		}

		name := src.Name // capture for closure
		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) {
				return ScrapeSourceArgs{SourceName: name}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		))
	}

	return jobs
}
