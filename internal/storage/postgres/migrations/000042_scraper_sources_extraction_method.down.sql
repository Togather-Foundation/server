ALTER TABLE scraper_sources
  DROP CONSTRAINT IF EXISTS scraper_sources_extraction_method_check;

ALTER TABLE scraper_sources
  DROP COLUMN IF EXISTS extraction_method;
