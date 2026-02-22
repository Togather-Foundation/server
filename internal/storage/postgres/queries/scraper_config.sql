-- name: GetScraperConfig :one
SELECT id, auto_scrape, max_concurrent_sources, request_timeout_seconds,
       retry_max_attempts, max_batch_size, rate_limit_ms, updated_at
FROM scraper_config
WHERE id = 1;

-- name: SetScraperConfig :exec
INSERT INTO scraper_config (id, auto_scrape, max_concurrent_sources, request_timeout_seconds,
                             retry_max_attempts, max_batch_size, rate_limit_ms)
VALUES (1, @auto_scrape, @max_concurrent_sources, @request_timeout_seconds,
        @retry_max_attempts, @max_batch_size, @rate_limit_ms)
ON CONFLICT (id) DO UPDATE SET
  auto_scrape             = EXCLUDED.auto_scrape,
  max_concurrent_sources  = EXCLUDED.max_concurrent_sources,
  request_timeout_seconds = EXCLUDED.request_timeout_seconds,
  retry_max_attempts      = EXCLUDED.retry_max_attempts,
  max_batch_size          = EXCLUDED.max_batch_size,
  rate_limit_ms           = EXCLUDED.rate_limit_ms,
  updated_at              = NOW();
