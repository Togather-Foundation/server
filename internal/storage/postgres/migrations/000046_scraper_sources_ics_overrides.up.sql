-- Add ICS-specific per-source override columns to scraper_sources.
-- These fields allow individual sources to override global ICS fetch settings.
ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS insecure_skip_verify        BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS request_timeout_seconds     INT     NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS max_body_bytes              BIGINT  NOT NULL DEFAULT 0;
