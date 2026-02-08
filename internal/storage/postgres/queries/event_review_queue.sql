-- SQLc queries for event_review_queue domain.
-- See docs/architecture/event-review-workflow.md for complete design.

-- name: FindReviewByDedup :one
-- Find existing review by deduplication keys (checks source_external_id or dedup_hash)
SELECT r.id,
       r.event_id,
       e.ulid AS event_ulid,
       r.original_payload,
       r.normalized_payload,
       r.warnings,
       r.source_id,
       r.source_external_id,
       r.dedup_hash,
       r.event_start_time,
       r.event_end_time,
       r.status,
       r.reviewed_by,
       r.reviewed_at,
       r.review_notes,
       r.rejection_reason,
       r.created_at,
       r.updated_at
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
 WHERE (
         (sqlc.narg('source_id')::text IS NOT NULL 
          AND sqlc.narg('source_external_id')::text IS NOT NULL 
          AND r.source_id = sqlc.narg('source_id') 
          AND r.source_external_id = sqlc.narg('source_external_id'))
         OR
         (sqlc.narg('dedup_hash')::text IS NOT NULL 
          AND r.dedup_hash = sqlc.narg('dedup_hash'))
       )
   AND r.status IN ('pending', 'rejected')
 ORDER BY r.created_at DESC
 LIMIT 1;

-- name: CreateReviewQueueEntry :one
-- Create new review queue entry
INSERT INTO event_review_queue (
  event_id,
  original_payload,
  normalized_payload,
  warnings,
  source_id,
  source_external_id,
  dedup_hash,
  event_start_time,
  event_end_time
) VALUES (
  sqlc.arg('event_id'),
  sqlc.arg('original_payload'),
  sqlc.arg('normalized_payload'),
  sqlc.arg('warnings'),
  sqlc.narg('source_id'),
  sqlc.narg('source_external_id'),
  sqlc.narg('dedup_hash'),
  sqlc.arg('event_start_time'),
  sqlc.narg('event_end_time')
)
RETURNING id,
          event_id,
          original_payload,
          normalized_payload,
          warnings,
          source_id,
          source_external_id,
          dedup_hash,
          event_start_time,
          event_end_time,
          status,
          reviewed_by,
          reviewed_at,
          review_notes,
          rejection_reason,
          created_at,
          updated_at;

-- name: UpdateReviewQueueEntry :one
-- Update existing review entry (for resubmissions with same issues)
UPDATE event_review_queue
   SET original_payload = COALESCE(sqlc.narg('original_payload'), original_payload),
       normalized_payload = COALESCE(sqlc.narg('normalized_payload'), normalized_payload),
       warnings = COALESCE(sqlc.narg('warnings'), warnings),
       updated_at = NOW()
 WHERE id = sqlc.arg('id')
RETURNING id,
          event_id,
          original_payload,
          normalized_payload,
          warnings,
          source_id,
          source_external_id,
          dedup_hash,
          event_start_time,
          event_end_time,
          status,
          reviewed_by,
          reviewed_at,
          review_notes,
          rejection_reason,
          created_at,
          updated_at;

-- name: GetReviewQueueEntry :one
-- Get single review by ID
SELECT id,
       event_id,
       original_payload,
       normalized_payload,
       warnings,
       source_id,
       source_external_id,
       dedup_hash,
       event_start_time,
       event_end_time,
       status,
       reviewed_by,
       reviewed_at,
       review_notes,
       rejection_reason,
       created_at,
       updated_at
  FROM event_review_queue
 WHERE id = sqlc.arg('id');

-- name: ListReviewQueue :many
-- List reviews with pagination and status filter
SELECT r.id,
       r.event_id,
       e.ulid AS event_ulid,
       r.original_payload,
       r.normalized_payload,
       r.warnings,
       r.source_id,
       r.source_external_id,
       r.dedup_hash,
       r.event_start_time,
       r.event_end_time,
       r.status,
       r.reviewed_by,
       r.reviewed_at,
       r.review_notes,
       r.rejection_reason,
       r.created_at,
       r.updated_at
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
 WHERE (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status'))
   AND (sqlc.narg('after_id')::integer IS NULL OR r.id > sqlc.narg('after_id'))
 ORDER BY r.id ASC
 LIMIT sqlc.arg('limit');

-- name: ApproveReview :one
-- Mark review as approved
UPDATE event_review_queue
   SET status = 'approved',
       reviewed_by = sqlc.arg('reviewed_by'),
       reviewed_at = NOW(),
       review_notes = sqlc.narg('notes'),
       updated_at = NOW()
 WHERE id = sqlc.arg('id')
   AND status = 'pending'
RETURNING id,
          event_id,
          original_payload,
          normalized_payload,
          warnings,
          source_id,
          source_external_id,
          dedup_hash,
          event_start_time,
          event_end_time,
          status,
          reviewed_by,
          reviewed_at,
          review_notes,
          rejection_reason,
          created_at,
          updated_at;

-- name: RejectReview :one
-- Mark review as rejected
UPDATE event_review_queue
   SET status = 'rejected',
       reviewed_by = sqlc.arg('reviewed_by'),
       reviewed_at = NOW(),
       rejection_reason = sqlc.arg('reason'),
       updated_at = NOW()
 WHERE id = sqlc.arg('id')
   AND status = 'pending'
RETURNING id,
          event_id,
          original_payload,
          normalized_payload,
          warnings,
          source_id,
          source_external_id,
          dedup_hash,
          event_start_time,
          event_end_time,
          status,
          reviewed_by,
          reviewed_at,
          review_notes,
          rejection_reason,
          created_at,
          updated_at;

-- name: CleanupExpiredRejections :exec
-- Delete rejected reviews for past events (7 day grace period)
DELETE FROM event_review_queue
 WHERE status = 'rejected'
   AND (
     event_end_time < NOW() - INTERVAL '7 days'
     OR (event_end_time IS NULL AND event_start_time < NOW() - INTERVAL '7 days')
   );

-- name: CleanupUnreviewedEvents :exec
-- Delete pending reviews for events that have already started (too late to review)
DELETE FROM event_review_queue
 WHERE status = 'pending'
   AND event_start_time < NOW();

-- name: CleanupArchivedReviews :exec
-- Archive old approved/superseded reviews (90 day retention)
DELETE FROM event_review_queue
 WHERE status IN ('approved', 'superseded')
   AND reviewed_at < NOW() - INTERVAL '90 days';

-- name: MarkUnreviewedEventsAsDeleted :exec
-- Mark events as deleted before cleaning up their pending reviews
UPDATE events
   SET lifecycle_state = 'deleted',
       deleted_at = NOW(),
       deletion_reason = 'Expired from review queue - event started before review',
       updated_at = NOW()
 WHERE id IN (
   SELECT event_id FROM event_review_queue
   WHERE status = 'pending' AND event_start_time < NOW()
 )
   AND lifecycle_state = 'pending_review';
