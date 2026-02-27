-- SQLc queries for scraper_submissions.

-- name: InsertScraperSubmission :one
-- Insert a new URL submission and return the full row.
INSERT INTO scraper_submissions (
  url,
  url_norm,
  submitter_ip,
  status
) VALUES (
  sqlc.arg('url'),
  sqlc.arg('url_norm'),
  sqlc.arg('submitter_ip'),
  'pending_validation'
)
RETURNING id, url, url_norm, submitted_at, submitter_ip, status,
          rejection_reason, notes, validated_at;

-- name: GetRecentSubmissionByURLNorm :one
-- Check if a url_norm was submitted within the given interval (for dedup).
-- Returns the most recent matching row if found.
SELECT id, url, url_norm, submitted_at, submitter_ip, status,
       rejection_reason, notes, validated_at
  FROM scraper_submissions
 WHERE url_norm     = sqlc.arg('url_norm')
   AND submitted_at > NOW() - sqlc.arg('interval')::INTERVAL
 ORDER BY submitted_at DESC
 LIMIT 1;

-- name: CountRecentSubmissionsByIP :one
-- Count submissions from a given IP within the provided interval (for rate limiting).
SELECT COUNT(*)
  FROM scraper_submissions
 WHERE submitter_ip = sqlc.arg('submitter_ip')::INET
   AND submitted_at > NOW() - sqlc.arg('interval')::INTERVAL;

-- name: ListPendingValidation :many
-- Fetch up to N rows awaiting async URL validation, oldest first.
SELECT id, url, url_norm, submitted_at, submitter_ip, status,
       rejection_reason, notes, validated_at
  FROM scraper_submissions
 WHERE status = 'pending_validation'
 ORDER BY submitted_at ASC
 LIMIT sqlc.arg('limit')::INT;

-- name: CountPendingValidation :one
-- Count rows currently awaiting async URL validation.
SELECT COUNT(*)
  FROM scraper_submissions
 WHERE status = 'pending_validation';

-- name: UpdateSubmissionStatus :exec
-- Update status, optional rejection_reason, and optional validated_at for a given row.
-- Used by the background validation worker.
UPDATE scraper_submissions
   SET status           = sqlc.arg('status'),
       rejection_reason = sqlc.narg('rejection_reason'),
       validated_at     = sqlc.narg('validated_at')
 WHERE id = sqlc.arg('id');

-- name: UpdateSubmissionAdminReview :one
-- Update status and optional notes for a given row (admin PATCH).
-- Returns the full updated row.
UPDATE scraper_submissions
   SET status = sqlc.arg('status'),
       notes  = sqlc.narg('notes')
 WHERE id = sqlc.arg('id')
RETURNING id, url, url_norm, submitted_at, submitter_ip, status,
          rejection_reason, notes, validated_at;

-- name: ListScraperSubmissions :many
-- Paginated list of submissions, optionally filtered by status (for admin).
SELECT id, url, url_norm, submitted_at, submitter_ip, status,
       rejection_reason, notes, validated_at
  FROM scraper_submissions
 WHERE (sqlc.narg('status')::TEXT IS NULL OR status = sqlc.narg('status'))
 ORDER BY submitted_at DESC
 LIMIT  sqlc.arg('limit')::INT
 OFFSET sqlc.arg('offset')::INT;

-- name: CountScraperSubmissions :one
-- Count submissions with optional status filter (for pagination total).
SELECT COUNT(*)
  FROM scraper_submissions
 WHERE (sqlc.narg('status')::TEXT IS NULL OR status = sqlc.narg('status'));
