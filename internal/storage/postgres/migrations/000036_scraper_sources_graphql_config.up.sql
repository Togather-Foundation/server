ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS graphql_config JSONB;
