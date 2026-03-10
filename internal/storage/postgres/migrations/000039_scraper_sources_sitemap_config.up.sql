ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS sitemap_config JSONB;
