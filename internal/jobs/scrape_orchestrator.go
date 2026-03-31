package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/Togather-Foundation/server/internal/storage/postgres"
)

const (
	JobKindScrapeOrchestrator = "scrape_orchestrator"
)

type ScrapeOrchestratorArgs struct {
	RespectAutoScrape bool     `json:"respect_auto_scrape"`
	SkipUpToDate      bool     `json:"skip_up_to_date"`
	SourceNames       []string `json:"source_names"`
	CurrentIndex      int      `json:"current_index"`
}

func (ScrapeOrchestratorArgs) Kind() string { return JobKindScrapeOrchestrator }

type ScrapeOrchestratorWorker struct {
	river.WorkerDefaults[ScrapeOrchestratorArgs]
	ConfigQueries scrapeOrchestratorConfigReader
	SourcesReader scrapeOrchestratorSourcesReader
	Logger        *slog.Logger
	Slot          string
}

type scrapeOrchestratorConfigReader interface {
	GetScraperConfig(ctx context.Context) (postgres.ScraperConfig, error)
}

type scrapeOrchestratorSourcesReader interface {
	ListScraperSourcesWithLatestRun(ctx context.Context, enabled pgtype.Bool) ([]postgres.ListScraperSourcesWithLatestRunRow, error)
}

func (w *ScrapeOrchestratorWorker) Work(ctx context.Context, job *river.Job[ScrapeOrchestratorArgs]) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if w.ConfigQueries == nil {
		return fmt.Errorf("scrape_orchestrator: config queries not configured")
	}
	if w.SourcesReader == nil {
		return fmt.Errorf("scrape_orchestrator: sources reader not configured")
	}

	respectAutoScrape := job.Args.RespectAutoScrape
	skipUpToDate := job.Args.SkipUpToDate

	logger.InfoContext(ctx, "scrape_orchestrator: starting",
		"respect_auto_scrape", respectAutoScrape,
		"skip_up_to_date", skipUpToDate,
		"current_index", job.Args.CurrentIndex)

	if respectAutoScrape {
		cfg, err := w.ConfigQueries.GetScraperConfig(ctx)
		if err != nil {
			if err != context.Canceled {
				logger.ErrorContext(ctx, "scrape_orchestrator: failed to read config", "error", err)
			}
			return fmt.Errorf("scrape_orchestrator: read config: %w", err)
		}
		if !cfg.AutoScrape {
			logger.InfoContext(ctx, "scrape_orchestrator: auto_scrape disabled, aborting run")
			return nil
		}
	}

	sources, err := w.SourcesReader.ListScraperSourcesWithLatestRun(ctx, pgtype.Bool{Valid: true, Bool: true})
	if err != nil {
		if err != context.Canceled {
			logger.ErrorContext(ctx, "scrape_orchestrator: failed to list sources", "error", err)
		}
		return fmt.Errorf("scrape_orchestrator: list sources: %w", err)
	}

	eligible := filterEligibleSources(ctx, logger, sources, skipUpToDate)

	if len(eligible) == 0 {
		logger.InfoContext(ctx, "scrape_orchestrator: no eligible sources to scrape")
		return nil
	}

	sourceNames := make([]string, len(eligible))
	for i, src := range eligible {
		sourceNames[i] = src.name
	}

	currentSource := eligible[0]
	logger.InfoContext(ctx, "scrape_orchestrator: enqueuing first source to start chain",
		"source", currentSource.name,
		"total", len(eligible))

	riverClient, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		logger.ErrorContext(ctx, "scrape_orchestrator: river client unavailable", "error", err)
		return fmt.Errorf("scrape_orchestrator: river client unavailable: %w", err)
	}

	_, err = riverClient.Insert(ctx, ScrapeSourceArgs{
		SourceName:        currentSource.name,
		SourceNames:       sourceNames,
		CurrentIndex:      0,
		RespectAutoScrape: respectAutoScrape,
		SkipUpToDate:      skipUpToDate,
	}, nil)
	if err != nil {
		logger.ErrorContext(ctx, "scrape_orchestrator: failed to enqueue first source",
			"source", currentSource.name, "error", err)
		return fmt.Errorf("scrape_orchestrator: insert first source job: %w", err)
	}

	logger.InfoContext(ctx, "scrape_orchestrator: enqueued first source to start chain",
		"source", currentSource.name,
		"total", len(eligible))

	return nil
}

type eligibleSource struct {
	name     string
	schedule string
	lastRun  *postgres.ListScraperSourcesWithLatestRunRow
}

func filterEligibleSources(ctx context.Context, logger *slog.Logger, sources []postgres.ListScraperSourcesWithLatestRunRow, skipUpToDate bool) []eligibleSource {
	var eligible []eligibleSource
	for _, src := range sources {
		if src.Schedule != "daily" && src.Schedule != "weekly" {
			continue
		}

		if skipUpToDate && src.LastRunStatus == "completed" && src.LastRunCompletedAt.Valid {
			freshDuration := 24 * time.Hour
			if src.Schedule == "weekly" {
				freshDuration = 7 * 24 * time.Hour
			}
			if time.Since(src.LastRunCompletedAt.Time) < freshDuration {
				logger.DebugContext(ctx, "scrape_orchestrator: skipping up-to-date source",
					"source", src.Name, "schedule", src.Schedule, "last_completed", src.LastRunCompletedAt.Time)
				continue
			}
		}

		eligible = append(eligible, eligibleSource{
			name:     src.Name,
			schedule: src.Schedule,
			lastRun:  &src,
		})
	}
	return eligible
}

func ScrapeOrchestratorInitialArgs(respectAutoScrape, skipUpToDate bool, sourceNames []string) ScrapeOrchestratorArgs {
	return ScrapeOrchestratorArgs{
		RespectAutoScrape: respectAutoScrape,
		SkipUpToDate:      skipUpToDate,
		SourceNames:       sourceNames,
		CurrentIndex:      0,
	}
}
