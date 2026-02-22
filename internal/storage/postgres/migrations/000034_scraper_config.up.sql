CREATE TABLE scraper_config (
  id                      INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  auto_scrape             BOOLEAN NOT NULL DEFAULT true,
  max_concurrent_sources  INT NOT NULL DEFAULT 3,
  request_timeout_seconds INT NOT NULL DEFAULT 30,
  retry_max_attempts      INT NOT NULL DEFAULT 3,
  max_batch_size          INT NOT NULL DEFAULT 100,
  rate_limit_ms           INT NOT NULL DEFAULT 1000,
  updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO scraper_config DEFAULT VALUES;
