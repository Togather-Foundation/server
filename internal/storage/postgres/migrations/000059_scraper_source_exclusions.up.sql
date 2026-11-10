ALTER TABLE scraper_sources
    ADD COLUMN exclude_url_pattern TEXT NOT NULL DEFAULT '',
    ADD COLUMN exclude_title_pattern TEXT NOT NULL DEFAULT '';
