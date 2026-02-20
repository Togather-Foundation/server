-- SQLc queries for scraper runs tracking.

-- name: InsertScraperRun :one
-- Insert a new scraper run record and return its id.
INSERT INTO scraper_runs (source_name, source_url, tier)
VALUES (sqlc.arg('source_name'), sqlc.arg('source_url'), sqlc.arg('tier'))
RETURNING id;

-- name: UpdateScraperRunCompleted :exec
-- Mark a scraper run as completed with event counts.
UPDATE scraper_runs
   SET status        = 'completed',
       completed_at  = NOW(),
       events_found  = sqlc.arg('events_found'),
       events_new    = sqlc.arg('events_new'),
       events_dup    = sqlc.arg('events_dup'),
       events_failed = sqlc.arg('events_failed')
 WHERE id = sqlc.arg('id');

-- name: UpdateScraperRunFailed :exec
-- Mark a scraper run as failed with an error message.
UPDATE scraper_runs
   SET status        = 'failed',
       completed_at  = NOW(),
       error_message = sqlc.arg('error_message')
 WHERE id = sqlc.arg('id');

-- name: GetLatestScraperRunBySource :one
-- Get the most recent scraper run for a given source_name.
SELECT id, source_name, source_url, tier, started_at, completed_at, status,
       events_found, events_new, events_dup, events_failed, error_message, metadata
  FROM scraper_runs
 WHERE source_name = sqlc.arg('source_name')
 ORDER BY started_at DESC
 LIMIT 1;

-- name: ListRecentScraperRuns :many
-- List the N most recent scraper runs ordered by started_at DESC.
SELECT id, source_name, source_url, tier, started_at, completed_at, status,
       events_found, events_new, events_dup, events_failed, error_message, metadata
  FROM scraper_runs
 ORDER BY started_at DESC
 LIMIT sqlc.arg('limit');
