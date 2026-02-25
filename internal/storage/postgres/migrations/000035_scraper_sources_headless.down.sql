ALTER TABLE scraper_sources
  DROP COLUMN IF EXISTS headless_rate_limit_ms,
  DROP COLUMN IF EXISTS headless_headers,
  DROP COLUMN IF EXISTS headless_pagination_btn,
  DROP COLUMN IF EXISTS headless_wait_timeout_ms,
  DROP COLUMN IF EXISTS headless_wait_selector;
