-- Remove missing fields from scraper_sources table
ALTER TABLE scraper_sources
    DROP COLUMN IF EXISTS urls,
    DROP COLUMN IF EXISTS event_url_pattern,
    DROP COLUMN IF EXISTS skip_multi_session_check,
    DROP COLUMN IF EXISTS multi_session_duration_threshold,
    DROP COLUMN IF EXISTS follow_event_urls,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS headless_wait_network_idle,
    DROP COLUMN IF EXISTS headless_undetected,
    DROP COLUMN IF EXISTS headless_iframe,
    DROP COLUMN IF EXISTS headless_intercept;