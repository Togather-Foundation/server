package postgres

import (
	"context"
	"fmt"

	"github.com/Togather-Foundation/server/internal/domain/events"
	"github.com/Togather-Foundation/server/internal/domain/scraper"
	"github.com/rs/zerolog"
)

// SyncConfig is the YAML sync path: persist the scraper configuration and
// register its feed identities before any events are ingested. Config names
// remain scraper labels; the provenance registry resolves each URL once.
func (r *ScraperSourceRepository) SyncConfig(ctx context.Context, params scraper.UpsertParams) (*scraper.Source, error) {
	source, err := r.Upsert(ctx, params)
	if err != nil {
		return nil, err
	}
	feeds := params.URLs
	if len(feeds) == 0 && params.URL != "" {
		feeds = []string{params.URL}
	}
	registry := NewEventRepository(r.pool, zerolog.Nop())
	for _, feed := range feeds {
		lookup := events.SourceParamsForFeed(params.Name, feed, params.License, params.TrustLevel)
		lookup.SourceType = "scraper"
		lookup.RequirePolicyMatch = true
		if _, err := registry.GetOrCreateSource(ctx, lookup); err != nil {
			return nil, fmt.Errorf("register feed %q for config %q: %w", feed, params.Name, err)
		}
	}
	return source, nil
}
