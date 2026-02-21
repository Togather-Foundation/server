-- Restore the explicit index that was dropped (it duplicated the UNIQUE constraint index).
CREATE INDEX IF NOT EXISTS idx_scraper_sources_name ON scraper_sources(name);
