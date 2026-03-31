package jobs

import (
	"context"

	"github.com/rs/zerolog"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/Togather-Foundation/server/internal/scraper"
)

type sourceRepository interface {
	List(ctx context.Context, enabled *bool) ([]domainScraper.Source, error)
}

func LoadSourcesForPeriodicJobs(ctx context.Context, repo sourceRepository, logger zerolog.Logger) ([]scraper.SourceConfig, error) {
	enabled := true
	sources, err := repo.List(ctx, &enabled)
	if err != nil {
		logger.Warn().Err(err).Msg("periodic sources: failed to load from DB, falling back to YAML")
		return nil, err
	}

	if len(sources) == 0 {
		logger.Warn().Msg("periodic sources: DB returned no enabled sources")
		return nil, nil
	}

	configs := make([]scraper.SourceConfig, 0, len(sources))
	for _, src := range sources {
		if src.Schedule != "daily" && src.Schedule != "weekly" {
			continue
		}

		cfg, err := scraper.SourceConfigFromDomain(src)
		if err != nil {
			logger.Warn().Err(err).Str("source", src.Name).Msg("periodic sources: skipping source with conversion error")
			continue
		}
		configs = append(configs, cfg)
	}

	logger.Info().Int("count", len(configs)).Msg("periodic sources: loaded from DB")
	return configs, nil
}
