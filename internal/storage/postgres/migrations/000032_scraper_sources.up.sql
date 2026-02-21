CREATE TABLE scraper_sources (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT UNIQUE NOT NULL,
  url             TEXT NOT NULL,
  tier            INT NOT NULL DEFAULT 0,
  schedule        TEXT NOT NULL DEFAULT 'manual'
                    CHECK (schedule IN ('daily', 'weekly', 'manual')),
  trust_level     INT NOT NULL DEFAULT 5,
  license         TEXT NOT NULL DEFAULT 'CC0-1.0',
  enabled         BOOL NOT NULL DEFAULT true,
  max_pages       INT NOT NULL DEFAULT 10,
  selectors       JSONB,
  notes           TEXT,
  last_scraped_at TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scraper_sources_name    ON scraper_sources(name);
CREATE INDEX idx_scraper_sources_enabled ON scraper_sources(enabled);

CREATE TABLE org_scraper_sources (
  organization_id   UUID   NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  scraper_source_id BIGINT NOT NULL REFERENCES scraper_sources(id) ON DELETE CASCADE,
  PRIMARY KEY (organization_id, scraper_source_id)
);

CREATE TABLE place_scraper_sources (
  place_id          UUID   NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  scraper_source_id BIGINT NOT NULL REFERENCES scraper_sources(id) ON DELETE CASCADE,
  PRIMARY KEY (place_id, scraper_source_id)
);
