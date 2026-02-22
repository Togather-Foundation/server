package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/rs/zerolog"

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
	Logger        zerolog.Logger
}

func (ScrapeSourceWorker) Kind() string { return JobKindScrapeSource }

// Work runs a scrape for the source named in job.Args.SourceName.
//
// Behaviour:
//   - Reads scraper_config.auto_scrape from the DB. If it is false, the job
//     skips silently (no error, no scrape).
//   - If the config read fails, the worker logs a warning and proceeds anyway
//     so that a transient DB blip does not silently drop scheduled scrapes.
//   - Returns an error (causing River to retry) if the underlying scrape fails.
//   - Returns an error immediately if Scraper is nil.
func (w ScrapeSourceWorker) Work(ctx context.Context, job *river.Job[ScrapeSourceArgs]) error {
	if w.Scraper == nil {
		return fmt.Errorf("scrape_source worker: scraper not configured")
	}

	sourceName := job.Args.SourceName

	// Check global auto_scrape toggle. Proceed on config read error (log only).
	if w.ConfigQueries != nil {
		cfg, err := w.ConfigQueries.GetScraperConfig(ctx)
		if err != nil {
			w.Logger.Warn().Err(err).Str("source", sourceName).Msg("scrape_source: failed to read scraper config, proceeding anyway")
		} else if !cfg.AutoScrape {
			w.Logger.Debug().Str("source", sourceName).Msg("scrape_source: auto_scrape disabled, skipping")
			return nil
		}
	}

	w.Logger.Info().Str("source", sourceName).Int("attempt", job.Attempt).Msg("scrape_source: starting periodic scrape")

	start := time.Now()
	result, err := w.Scraper.ScrapeSource(ctx, sourceName, scraper.ScrapeOptions{})
	if err != nil {
		return fmt.Errorf("scrape_source %s: %w", sourceName, err)
	}

	w.Logger.Info().
		Str("source", sourceName).
		Int("events_found", result.EventsFound).
		Int("events_created", result.EventsCreated).
		Int("events_duplicate", result.EventsDuplicate).
		Dur("duration", time.Since(start)).
		Msg("scrape_source: periodic scrape completed")

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
