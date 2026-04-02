ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS default_location JSONB;
