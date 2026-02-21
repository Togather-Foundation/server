package scraper

import (
	"encoding/json"
	"fmt"

	domainScraper "github.com/Togather-Foundation/server/internal/domain/scraper"
)

// SourceConfigFromDomain converts a domain/scraper.Source (read from the DB)
// into a SourceConfig suitable for scraping. Selectors are JSON-decoded from
// the JSONB column; an empty/nil Selectors field is valid for Tier 0 sources.
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

	return cfg, nil
}
