-- Sources using the removed method cannot run any more: disable them and reset the
-- method so the narrower constraint can be restored.
UPDATE scraper_sources
   SET enabled = false, extraction_method = ''
 WHERE extraction_method = 'bibliocommons_rss';

ALTER TABLE scraper_sources
  DROP CONSTRAINT IF EXISTS scraper_sources_extraction_method_check;

ALTER TABLE scraper_sources
  ADD CONSTRAINT scraper_sources_extraction_method_check
  CHECK (extraction_method IN ('', 'scraper', 'ics'));
