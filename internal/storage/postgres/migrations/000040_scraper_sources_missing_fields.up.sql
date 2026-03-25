-- Add missing fields to scraper_sources table
ALTER TABLE scraper_sources
    ADD COLUMN IF NOT EXISTS urls                          TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS event_url_pattern             TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS skip_multi_session_check      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS multi_session_duration_threshold TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS follow_event_urls             BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS timezone                      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS headless_wait_network_idle    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS headless_undetected           BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS headless_iframe               JSONB,
    ADD COLUMN IF NOT EXISTS headless_intercept            JSONB;