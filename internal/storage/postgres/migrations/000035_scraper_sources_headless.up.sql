ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS headless_wait_selector   TEXT,
  ADD COLUMN IF NOT EXISTS headless_wait_timeout_ms INT  NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS headless_pagination_btn  TEXT,
  ADD COLUMN IF NOT EXISTS headless_headers         JSONB,
  ADD COLUMN IF NOT EXISTS headless_rate_limit_ms   INT  NOT NULL DEFAULT 0;
