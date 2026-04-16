ALTER TABLE scraper_sources
  DROP COLUMN IF EXISTS insecure_skip_verify,
  DROP COLUMN IF EXISTS request_timeout_seconds,
  DROP COLUMN IF EXISTS max_body_bytes;
