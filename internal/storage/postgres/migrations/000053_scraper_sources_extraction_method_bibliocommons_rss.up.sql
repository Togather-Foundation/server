-- Allow extraction_method 'bibliocommons_rss' (BiblioCommons library events RSS feed).
ALTER TABLE scraper_sources
  DROP CONSTRAINT IF EXISTS scraper_sources_extraction_method_check;

ALTER TABLE scraper_sources
  ADD CONSTRAINT scraper_sources_extraction_method_check
  CHECK (extraction_method IN ('', 'scraper', 'ics', 'bibliocommons_rss'));
