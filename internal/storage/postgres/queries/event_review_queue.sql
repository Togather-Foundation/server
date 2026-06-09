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
       r.updated_at,
       r.duplicate_of_event_id,
       dup.ulid AS duplicate_of_event_ulid
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
  LEFT JOIN events dup ON dup.id = r.duplicate_of_event_id
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
  event_end_time,
  duplicate_of_event_id
) VALUES (
  sqlc.arg('event_id'),
  sqlc.arg('original_payload'),
  sqlc.arg('normalized_payload'),
  sqlc.arg('warnings'),
  sqlc.narg('source_id'),
  sqlc.narg('source_external_id'),
  sqlc.narg('dedup_hash'),
  sqlc.arg('event_start_time'),
  sqlc.narg('event_end_time'),
  sqlc.narg('duplicate_of_event_id')
)
RETURNING *;

-- name: UpdateReviewQueueEntry :one
-- Update existing review entry (for resubmissions with same issues).
-- Pass clear_duplicate_of=TRUE to set duplicate_of_event_id to NULL;
-- otherwise pass a new UUID via duplicate_of_event_id or leave both NULL to keep the existing value.
UPDATE event_review_queue
   SET original_payload = COALESCE(sqlc.narg('original_payload'), original_payload),
       normalized_payload = COALESCE(sqlc.narg('normalized_payload'), normalized_payload),
       warnings = COALESCE(sqlc.narg('warnings'), warnings),
       duplicate_of_event_id = CASE
                                 WHEN sqlc.narg('clear_duplicate_of')::boolean IS TRUE THEN NULL
                                 ELSE COALESCE(sqlc.narg('duplicate_of_event_id'), duplicate_of_event_id)
                               END,
       updated_at = NOW()
 WHERE id = sqlc.arg('id')
RETURNING *;

-- name: GetReviewQueueEntry :one
-- Get single review by ID
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
       r.updated_at,
       r.duplicate_of_event_id,
       dup.ulid AS duplicate_of_event_ulid
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
  LEFT JOIN events dup ON dup.id = r.duplicate_of_event_id
 WHERE r.id = sqlc.arg('id');

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
       r.updated_at,
       r.duplicate_of_event_id,
       dup.ulid AS duplicate_of_event_ulid
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
  LEFT JOIN events dup ON dup.id = r.duplicate_of_event_id
 WHERE (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status'))
   AND (sqlc.narg('after_id')::integer IS NULL OR r.id > sqlc.narg('after_id'))
 ORDER BY r.id ASC
 LIMIT sqlc.arg('limit');

-- name: CountReviewQueueByStatus :one
-- Count total reviews by status (for badge display)
SELECT COUNT(*) as total
  FROM event_review_queue
 WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));

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
RETURNING *;

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
RETURNING *;

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
-- Archive old approved/superseded/merged/dismissed reviews (90 day retention)
DELETE FROM event_review_queue
 WHERE status IN ('approved', 'superseded', 'merged', 'dismissed')
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

-- name: GetPendingReviewByEventUlid :one
-- Get the pending review queue entry for an event by its ULID, if any.
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
       r.updated_at,
       r.duplicate_of_event_id,
       dup.ulid AS duplicate_of_event_ulid
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
  LEFT JOIN events dup ON dup.id = r.duplicate_of_event_id
 WHERE e.ulid = sqlc.arg('event_ulid')
   AND r.status = 'pending'
 LIMIT 1;

-- name: GetPendingReviewByEventUlidAndDuplicateUlid :one
-- Get the pending review queue entry for an event by its ULID, narrowed to the
-- specific companion whose duplicate_of_event_id points to the counterpart event.
-- Used by the add-occurrence workflow to avoid picking an unrelated pending review
-- when the same event has multiple pending review rows.
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
       r.updated_at,
       r.duplicate_of_event_id,
       dup.ulid AS duplicate_of_event_ulid
  FROM event_review_queue r
  JOIN events e ON e.id = r.event_id
  LEFT JOIN events dup ON dup.id = r.duplicate_of_event_id
 WHERE e.ulid = sqlc.arg('event_ulid')
   AND r.status = 'pending'
   AND dup.ulid = sqlc.arg('duplicate_ulid')
 LIMIT 1;

-- name: UpdateReviewWarnings :exec
-- Update only the warnings JSON of a review queue entry (used for companion warning dismissal).
UPDATE event_review_queue
   SET warnings = sqlc.arg('warnings'),
       updated_at = NOW()
 WHERE id = sqlc.arg('id');

-- name: DismissCompanionWarningMatch :exec
-- Atomically remove any potential_duplicate match entry whose ulid equals event_ulid
-- from the companion's pending review queue entry, identified by the companion's event ULID.
-- Rebuilds the warnings JSONB in one UPDATE — no read-modify-write race.
UPDATE event_review_queue
   SET warnings = (
         SELECT COALESCE(jsonb_agg(new_w ORDER BY idx), '[]'::jsonb)
         FROM (
           SELECT idx,
                  CASE
                    WHEN w->>'code' = 'potential_duplicate' THEN
                      jsonb_set(
                        w,
                        '{details,matches}',
                        COALESCE((
                          SELECT jsonb_agg(m)
                          FROM jsonb_array_elements(w->'details'->'matches') m
                          WHERE m->>'ulid' <> sqlc.arg('event_ulid')::text
                        ), '[]'::jsonb)
                      )
                    ELSE w
                  END AS new_w
           FROM jsonb_array_elements(warnings) WITH ORDINALITY AS t(w, idx)
         ) sub
       ),
       updated_at = NOW()
 WHERE event_id = (
         SELECT e.id FROM events e WHERE e.ulid = sqlc.arg('companion_ulid')::text LIMIT 1
       )
   AND status = 'pending';

-- name: DismissWarningMatchByReviewID :exec
-- Atomically remove any potential_duplicate match entry whose ulid equals event_ulid
-- from a specific review queue entry identified by its primary key id.
-- Narrower than DismissCompanionWarningMatch: targets exactly one row, preventing
-- accidental modification of unrelated pending reviews on the same companion event.
UPDATE event_review_queue
   SET warnings = (
         SELECT COALESCE(jsonb_agg(new_w ORDER BY idx), '[]'::jsonb)
         FROM (
           SELECT idx,
                  CASE
                    WHEN w->>'code' = 'potential_duplicate' THEN
                      jsonb_set(
                        w,
                        '{details,matches}',
                        COALESCE((
                          SELECT jsonb_agg(m)
                          FROM jsonb_array_elements(w->'details'->'matches') m
                          WHERE m->>'ulid' <> sqlc.arg('event_ulid')::text
                        ), '[]'::jsonb)
                      )
                    ELSE w
                  END AS new_w
           FROM jsonb_array_elements(warnings) WITH ORDINALITY AS t(w, idx)
         ) sub
       ),
       updated_at = NOW()
 WHERE id = sqlc.arg('review_id')::int;

-- name: DismissAllCompanionWarnings :one
-- Atomically strips all companion warning entries referencing the given event_ulid
-- from a specific review row. Handles three warning types:
--   near_duplicate_of_new_event  — stripped when duplicate_of_event_id matches
--   potential_duplicate          — specific match entries filtered; warning nullified when matches empty
--   cross_week_series_companion  — stripped when details->>'companion_ulid' matches
-- Also clears duplicate_of_event_id if it points to the given event.
-- Returns true (warnings_empty) when the resulting warnings array is empty after stripping.
WITH target_event AS (
  SELECT id FROM events WHERE ulid = sqlc.arg('event_ulid') LIMIT 1
),
stripped AS (
  SELECT COALESCE(jsonb_agg(new_w ORDER BY idx), '[]'::jsonb) AS warnings
  FROM (
    SELECT idx,
           CASE
             WHEN w->>'code' = 'potential_duplicate' THEN (
               SELECT CASE WHEN jsonb_array_length(rebuilt) = 0 THEN NULL
                           ELSE jsonb_set(w, '{details,matches}', rebuilt)
                      END
               FROM (
                 SELECT COALESCE(jsonb_agg(m), '[]'::jsonb) AS rebuilt
                 FROM jsonb_array_elements(w->'details'->'matches') m
                 WHERE m->>'ulid' <> sqlc.arg('event_ulid')::text
               ) _
             )
             WHEN w->>'code' = 'cross_week_series_companion'
                  AND w->'details'->>'companion_ulid' = sqlc.arg('event_ulid')::text THEN NULL
             ELSE w
           END AS new_w
    FROM event_review_queue rq,
         jsonb_array_elements(rq.warnings) WITH ORDINALITY AS t(w, idx)
    WHERE rq.id = sqlc.arg('review_id')::int
      AND NOT (
        w->>'code' = 'near_duplicate_of_new_event'
        AND rq.duplicate_of_event_id = (SELECT id FROM target_event)
      )
  ) sub
  WHERE new_w IS NOT NULL
)
UPDATE event_review_queue
   SET warnings = stripped.warnings,
       duplicate_of_event_id = CASE
         WHEN duplicate_of_event_id = (SELECT id FROM target_event) THEN NULL
         ELSE duplicate_of_event_id
       END,
       updated_at = NOW()
  FROM stripped
 WHERE id = sqlc.arg('review_id')::int
   AND status = 'pending'
RETURNING (stripped.warnings = '[]'::jsonb) AS warnings_empty;

-- name: StripRetiredDupWarnings :one
-- Atomically strips all duplicate warning entries referencing any of the given retire_ulids
-- from a specific review row. Handles three warning types:
--   near_duplicate_of_new_event  — stripped when duplicate_of_event_id points to a retired event
--   potential_duplicate          — specific match entries filtered; warning nullified when matches empty
--   cross_week_series_companion  — stripped when details->>'companion_ulid' is in the retire set
-- Also clears duplicate_of_event_id if it points to a retired event.
-- Returns true (warnings_empty) when the resulting warnings array is empty after stripping.
-- Note: companion replacement is handled in Go after SQL returns.
WITH stripped AS (
  SELECT COALESCE(jsonb_agg(new_w ORDER BY idx), '[]'::jsonb) AS warnings
  FROM (
    SELECT idx,
           CASE
             WHEN w->>'code' = 'potential_duplicate' THEN (
               SELECT CASE WHEN jsonb_array_length(rebuilt) = 0 THEN NULL
                           ELSE jsonb_set(w, '{details,matches}', rebuilt)
                      END
               FROM (
                 SELECT COALESCE(jsonb_agg(m), '[]'::jsonb) AS rebuilt
                 FROM jsonb_array_elements(w->'details'->'matches') m
                 WHERE NOT (m->>'ulid' = ANY(sqlc.arg('retire_ulids')::text[]))
               ) _
             )
             WHEN w->>'code' = 'cross_week_series_companion'
                  AND w->'details'->>'companion_ulid' = ANY(sqlc.arg('retire_ulids')::text[]) THEN NULL
             ELSE w
           END AS new_w
    FROM event_review_queue rq,
         jsonb_array_elements(rq.warnings) WITH ORDINALITY AS t(w, idx)
    WHERE rq.id = sqlc.arg('review_id')::int
      AND NOT (
        w->>'code' = 'near_duplicate_of_new_event'
        AND EXISTS (
          SELECT 1 FROM events WHERE id = rq.duplicate_of_event_id
          AND ulid = ANY(sqlc.arg('retire_ulids')::text[])
        )
      )
  ) sub
  WHERE new_w IS NOT NULL
)
UPDATE event_review_queue
   SET warnings = stripped.warnings,
       duplicate_of_event_id = CASE
       WHEN EXISTS (
            SELECT 1 FROM events WHERE id = event_review_queue.duplicate_of_event_id
            AND ulid = ANY(sqlc.arg('retire_ulids')::text[])
          ) THEN NULL
          ELSE duplicate_of_event_id
        END,
        updated_at = NOW()
   FROM stripped
  WHERE id = sqlc.arg('review_id')::int
    AND status = 'pending'
RETURNING (stripped.warnings = '[]'::jsonb) AS warnings_empty;

-- name: FindCrossWeekCompanionTargets :many
-- Find all pending review entries whose cross_week_series_companion warnings
-- reference any of the given retire ULIDs. Returns the review ID and event ULID
-- so callers can update the warning details to point to a surviving canonical.
SELECT rq.id AS review_id, e.ulid AS event_ulid
FROM event_review_queue rq
JOIN events e ON e.id = rq.event_id
CROSS JOIN jsonb_array_elements(rq.warnings) w
WHERE rq.status = 'pending'
  AND w->>'code' = 'cross_week_series_companion'
  AND w->'details'->>'companion_ulid' = ANY(sqlc.arg('retire_ulids')::text[]);


