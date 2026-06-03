ALTER TABLE scraper_sources ADD COLUMN event_domain TEXT DEFAULT '' CHECK (event_domain IN ('', 'arts', 'music', 'culture', 'sports', 'community', 'education', 'general'));
