-- The UNIQUE constraint on scraper_sources.name already creates an implicit index.
-- The explicit idx_scraper_sources_name is therefore redundant and wastes space.
DROP INDEX IF EXISTS idx_scraper_sources_name;
