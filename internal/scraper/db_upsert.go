package scraper

import (
	"encoding/json"
	"fmt"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
)

// marshalJSON returns the JSON encoding of v, or nil if v is nil.
// Returns an error if marshalling fails.
func marshalJSON(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SourceConfigToUpsertParams converts a scraper.SourceConfig (from YAML) to
// the domain UpsertParams. Selectors are JSON-encoded for the JSONB column.
// Headless headers and GraphQL config are JSON-encoded when present.
func SourceConfigToUpsertParams(cfg SourceConfig) (domainScraper.UpsertParams, error) {
	var selectorsJSON []byte
	if cfg.Tier == 1 || cfg.Tier == 2 {
		var encErr error
		selectorsJSON, encErr = json.Marshal(cfg.Selectors)
		if encErr != nil {
			return domainScraper.UpsertParams{}, fmt.Errorf("encode selectors: %w", encErr)
		}
	}

	var headlessHeadersJSON []byte
	if len(cfg.Headless.Headers) > 0 {
		var encErr error
		headlessHeadersJSON, encErr = json.Marshal(cfg.Headless.Headers)
		if encErr != nil {
			return domainScraper.UpsertParams{}, fmt.Errorf("encode headless headers: %w", encErr)
		}
	}

	var graphqlConfigJSON []byte
	if cfg.Tier == 3 && cfg.GraphQL != nil {
		var encErr error
		graphqlConfigJSON, encErr = json.Marshal(cfg.GraphQL)
		if encErr != nil {
			return domainScraper.UpsertParams{}, fmt.Errorf("encode graphql config: %w", encErr)
		}
	}

	var restConfigJSON []byte
	if cfg.Tier == 3 && cfg.REST != nil {
		var encErr error
		restConfigJSON, encErr = json.Marshal(cfg.REST)
		if encErr != nil {
			return domainScraper.UpsertParams{}, fmt.Errorf("encode rest config: %w", encErr)
		}
	}

	var sitemapConfigJSON []byte
	if cfg.Sitemap != nil {
		var encErr error
		sitemapConfigJSON, encErr = json.Marshal(cfg.Sitemap)
		if encErr != nil {
			return domainScraper.UpsertParams{}, fmt.Errorf("encode sitemap config: %w", encErr)
		}
	}

	iframeJSON, err := marshalJSON(cfg.Headless.Iframe)
	if err != nil {
		return domainScraper.UpsertParams{}, fmt.Errorf("encode headless iframe: %w", err)
	}
	interceptJSON, err := marshalJSON(cfg.Headless.Intercept)
	if err != nil {
		return domainScraper.UpsertParams{}, fmt.Errorf("encode headless intercept: %w", err)
	}

	return domainScraper.UpsertParams{
		Name:                          cfg.Name,
		URL:                           cfg.URL,
		URLs:                          cfg.URLs,
		Tier:                          cfg.Tier,
		Schedule:                      cfg.Schedule,
		TrustLevel:                    cfg.TrustLevel,
		License:                       cfg.License,
		Enabled:                       cfg.Enabled,
		MaxPages:                      cfg.MaxPages,
		Selectors:                     selectorsJSON,
		Notes:                         cfg.Notes,
		EventURLPattern:               cfg.EventURLPattern,
		SkipMultiSessionCheck:         cfg.SkipMultiSessionCheck,
		MultiSessionDurationThreshold: cfg.MultiSessionDurationThreshold,
		FollowEventURLs:               cfg.FollowEventURLs,
		Timezone:                      cfg.Timezone,
		HeadlessWaitSelector:          cfg.Headless.WaitSelector,
		HeadlessWaitTimeoutMs:         cfg.Headless.WaitTimeoutMs,
		HeadlessPaginationBtn:         cfg.Headless.PaginationBtn,
		HeadlessHeaders:               headlessHeadersJSON,
		HeadlessRateLimitMs:           cfg.Headless.RateLimitMs,
		HeadlessWaitNetworkIdle:       cfg.Headless.WaitNetworkIdle,
		HeadlessUndetected:            cfg.Headless.Undetected,
		HeadlessIframe:                iframeJSON,
		HeadlessIntercept:             interceptJSON,
		GraphQLConfig:                 graphqlConfigJSON,
		RestConfig:                    restConfigJSON,
		SitemapConfig:                 sitemapConfigJSON,
	}, nil
}
