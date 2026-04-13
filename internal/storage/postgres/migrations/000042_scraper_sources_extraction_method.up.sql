-- Add extraction_method column to scraper_sources.
-- Values: '' (default, tier-based dispatch), 'scraper' (explicit tier-based),
-- 'ics' (ICS feed extraction).
-- This is distinct from provenance sources.source_type (migration 000002),
-- which tracks how data entered SEL. extraction_method tracks which
-- extraction pipeline the scraper uses for this source.
ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS extraction_method TEXT NOT NULL DEFAULT '';

-- Constrain to known values. Empty string is the default for existing rows
-- that predate the ICS feature; at runtime it triggers tier-based dispatch.
ALTER TABLE scraper_sources
  ADD CONSTRAINT scraper_sources_extraction_method_check
  CHECK (extraction_method IN ('', 'scraper', 'ics'));
