package scraper

import (
	"encoding/json"
	"fmt"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
)

// SourceConfigFromDomain converts a domain/scraper.Source (read from the DB)
// into a SourceConfig suitable for scraping. Selectors are JSON-decoded from
// the JSONB column; an empty/nil Selectors field is valid for Tier 0 sources.
// GraphQLConfig is JSON-decoded for Tier 3 sources; nil for all other tiers.
func SourceConfigFromDomain(src domainScraper.Source) (SourceConfig, error) {
	cfg := SourceConfig{
		Name:       src.Name,
		URL:        src.URL,
		Tier:       src.Tier,
		Schedule:   src.Schedule,
		TrustLevel: src.TrustLevel,
		License:    src.License,
		Enabled:    src.Enabled,
		MaxPages:   src.MaxPages,
		Notes:      src.Notes,
	}

	if len(src.Selectors) > 0 {
		if err := json.Unmarshal(src.Selectors, &cfg.Selectors); err != nil {
			return SourceConfig{}, fmt.Errorf("decode selectors for %q: %w", src.Name, err)
		}
	}

	cfg.Headless.WaitSelector = src.HeadlessWaitSelector
	cfg.Headless.WaitTimeoutMs = src.HeadlessWaitTimeoutMs
	cfg.Headless.PaginationBtn = src.HeadlessPaginationBtn
	cfg.Headless.RateLimitMs = src.HeadlessRateLimitMs
	if len(src.HeadlessHeaders) > 0 {
		if err := json.Unmarshal(src.HeadlessHeaders, &cfg.Headless.Headers); err != nil {
			return SourceConfig{}, fmt.Errorf("decode headless headers for %q: %w", src.Name, err)
		}
	}

	if len(src.GraphQLConfig) > 0 {
		var gql GraphQLConfig
		if err := json.Unmarshal(src.GraphQLConfig, &gql); err != nil {
			return SourceConfig{}, fmt.Errorf("decode graphql config for %q: %w", src.Name, err)
		}
		cfg.GraphQL = &gql
	}

	if len(src.RestConfig) > 0 {
		var rest RestConfig
		if err := json.Unmarshal(src.RestConfig, &rest); err != nil {
			return SourceConfig{}, fmt.Errorf("decode rest config for %q: %w", src.Name, err)
		}
		cfg.REST = &rest
	}

	return cfg, nil
}
