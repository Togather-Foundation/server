ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS rest_config JSONB;
