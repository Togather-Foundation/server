package jobs

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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

	// Chain metadata for serial chaining:
	// When set, the worker will enqueue the next source after completion.
	SourceNames       []string `json:"source_names"`
	CurrentIndex      int      `json:"current_index"`
	RespectAutoScrape bool     `json:"respect_auto_scrape"`
	SkipUpToDate      bool     `json:"skip_up_to_date"`
}

func (ScrapeSourceArgs) Kind() string { return JobKindScrapeSource }

// IsChained returns true if this job is part of a chain (has chain metadata).
func (a ScrapeSourceArgs) IsChained() bool {
	return len(a.SourceNames) > 0
}

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
	// Slot is the deployment slot (blue/green). It is reserved for structured
	// logging; Prometheus metrics labels are controlled by the slot baked into
	// the Scraper instance at construction time (NewScraperWithSourceRepoAndSlot).
	Slot string
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
	// For chained jobs, respect RespectAutoScrape flag to bypass global check.
	opts := scraper.ScrapeOptions{}
	shouldCheckAutoScrape := true

	if job.Args.IsChained() && !job.Args.RespectAutoScrape {
		shouldCheckAutoScrape = false
		logger.DebugContext(ctx, "scrape_source: chained job bypassing auto_scrape check", "source", sourceName)
	}

	if shouldCheckAutoScrape && w.ConfigQueries != nil {
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

	// Chain to next source if this is a chained job and there are remaining sources
	if job.Args.IsChained() {
		nextIndex := job.Args.CurrentIndex + 1
		if nextIndex < len(job.Args.SourceNames) {
			nextSourceName := job.Args.SourceNames[nextIndex]
			logger.InfoContext(ctx, "scrape_source: enqueuing next chained source",
				"current", sourceName,
				"next", nextSourceName,
				"next_index", nextIndex,
				"total", len(job.Args.SourceNames),
			)

			riverClient, err := river.ClientFromContextSafely[pgx.Tx](ctx)
			if err != nil {
				logger.ErrorContext(ctx, "scrape_source: river client unavailable for chaining", "error", err)
				return fmt.Errorf("scrape_source: river client unavailable for chaining: %w", err)
			}

			_, err = riverClient.Insert(ctx, ScrapeSourceArgs{
				SourceName:        nextSourceName,
				SourceNames:       job.Args.SourceNames,
				CurrentIndex:      nextIndex,
				RespectAutoScrape: job.Args.RespectAutoScrape,
				SkipUpToDate:      job.Args.SkipUpToDate,
			}, nil)
			if err != nil {
				logger.ErrorContext(ctx, "scrape_source: failed to enqueue next chained source",
					"next_source", nextSourceName, "error", err)
				return fmt.Errorf("scrape_source: insert next chained job: %w", err)
			}

			logger.InfoContext(ctx, "scrape_source: chained next source",
				"next_source", nextSourceName,
				"next_index", nextIndex,
				"total", len(job.Args.SourceNames))
		} else {
			logger.InfoContext(ctx, "scrape_source: chain complete, no more sources",
				"total", len(job.Args.SourceNames))
		}
	}

	return nil
}

// Jitter windows for staggering periodic scrape jobs across sources.
const (
	dailyJitterWindow  = 2 * time.Hour
	weeklyJitterWindow = 4 * time.Hour
)

// sourceJitterOffset returns a deterministic duration offset in [0, window)
// derived from a hash of the source name. The same name always produces the
// same offset, so the schedule is stable across restarts.
func sourceJitterOffset(sourceName string, window time.Duration) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(sourceName))
	ratio := float64(h.Sum32()) / float64(1<<32)
	return time.Duration(ratio * float64(window))
}

// staggeredSchedule is a river.PeriodicSchedule that fires at a fixed offset
// within each period. Given an interval and an offset, the next run time is
// always: truncate(current, interval) + offset + interval
//
// This guarantees that every source with the same interval fires at its own
// deterministic sub-slot rather than all piling up on the same clock tick.
type staggeredSchedule struct {
	interval time.Duration
	offset   time.Duration
}

// Next returns the next run time that is strictly after current.
// It aligns to: floor(current / interval)*interval + offset, then advances
// by one period if that time is not strictly in the future.
func (s *staggeredSchedule) Next(current time.Time) time.Time {
	// Truncate to the start of the current period.
	periodStart := current.Truncate(s.interval)
	candidate := periodStart.Add(s.offset)
	// If the candidate is not strictly after current, move to the next period.
	if !candidate.After(current) {
		candidate = candidate.Add(s.interval)
	}
	return candidate
}

// NewPeriodicJobsFromSources returns the base periodic jobs plus one River
// PeriodicJob for every source whose Schedule is "daily" or "weekly" and
// whose Enabled flag is true.
//
// Each source receives a deterministic jitter offset so that jobs spread
// across a stagger window rather than all firing simultaneously:
//   - "daily"  → 24-hour interval, offset spread within 2 hours
//   - "weekly" → 7-day interval, offset spread within 4 hours
func NewPeriodicJobsFromSources(sources []scraper.SourceConfig) []*river.PeriodicJob {
	jobs := NewPeriodicJobs()

	for _, src := range sources {
		if !src.Enabled {
			continue
		}

		var interval, window time.Duration
		switch src.Schedule {
		case "daily":
			interval = 24 * time.Hour
			window = dailyJitterWindow
		case "weekly":
			interval = 7 * 24 * time.Hour
			window = weeklyJitterWindow
		default:
			// "manual" or unknown — skip
			continue
		}

		offset := sourceJitterOffset(src.Name, window)
		schedule := &staggeredSchedule{interval: interval, offset: offset}

		name := src.Name // capture for closure
		jobs = append(jobs, river.NewPeriodicJob(
			schedule,
			func() (river.JobArgs, *river.InsertOpts) {
				return ScrapeSourceArgs{SourceName: name}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		))
	}

	return jobs
}
