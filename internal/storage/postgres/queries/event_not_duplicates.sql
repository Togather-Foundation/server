-- SQLc queries for event_not_duplicates table.
-- Tracks pairs of events that an admin has confirmed are NOT duplicates,
-- preventing them from being re-flagged during near-duplicate detection.

-- name: InsertNotDuplicate :exec
-- Record that two events are confirmed as NOT duplicates.
-- Uses canonical ordering (smaller ULID first) to prevent storing both (A,B) and (B,A).
-- ON CONFLICT DO NOTHING handles the case where the pair already exists.
INSERT INTO event_not_duplicates (event_id_a, event_id_b, created_by)
VALUES (
  LEAST(sqlc.arg('event_id_a'), sqlc.arg('event_id_b')),
  GREATEST(sqlc.arg('event_id_a'), sqlc.arg('event_id_b')),
  sqlc.narg('created_by')
)
ON CONFLICT (event_id_a, event_id_b) DO NOTHING;

-- name: IsNotDuplicate :one
-- Check if a pair of events has been marked as not-duplicates.
-- Uses canonical ordering to match regardless of argument order.
SELECT EXISTS(
  SELECT 1 FROM event_not_duplicates
  WHERE event_id_a = LEAST(sqlc.arg('event_id_a'), sqlc.arg('event_id_b'))
    AND event_id_b = GREATEST(sqlc.arg('event_id_a'), sqlc.arg('event_id_b'))
) AS is_not_duplicate;

-- name: ListNotDuplicatesForEvent :many
-- List all events that have been confirmed as NOT duplicates of a given event.
-- Returns both sides of the pair (the given event could be event_id_a or event_id_b).
SELECT event_id_a, event_id_b, created_at, created_by
  FROM event_not_duplicates
 WHERE event_id_a = sqlc.arg('event_id')
    OR event_id_b = sqlc.arg('event_id')
 ORDER BY created_at DESC;
