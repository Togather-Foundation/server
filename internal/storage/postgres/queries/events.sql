-- SQLc queries for events domain.

-- name: GetEventByULID :many
SELECT e.id,
       e.ulid,
       e.name,
       e.description,
       e.lifecycle_state,
       e.event_domain,
       e.organizer_id,
       e.primary_venue_id,
       e.keywords,
       e.federation_uri,
       e.created_at,
       e.updated_at,
       o.id AS occurrence_id,
       o.start_time,
       o.end_time,
       o.timezone,
       o.venue_id,
       o.virtual_url
  FROM events e
  LEFT JOIN event_occurrences o ON o.event_id = e.id
 WHERE e.ulid = $1
  ORDER BY o.start_time ASC;

-- name: GetIdempotencyKey :one
SELECT key,
       request_hash,
       event_id,
       event_ulid
  FROM idempotency_keys
 WHERE key = $1;

-- name: InsertIdempotencyKey :one
INSERT INTO idempotency_keys (key, request_hash, event_id, event_ulid)
VALUES ($1, $2, $3, $4)
RETURNING key, request_hash, event_id, event_ulid;

-- name: UpdateEvent :one
UPDATE events
   SET name = COALESCE(sqlc.narg('name'), name),
       description = COALESCE(sqlc.narg('description'), description),
       lifecycle_state = COALESCE(sqlc.narg('lifecycle_state'), lifecycle_state),
       image_url = COALESCE(sqlc.narg('image_url'), image_url),
       public_url = COALESCE(sqlc.narg('public_url'), public_url),
       event_domain = COALESCE(sqlc.narg('event_domain'), event_domain),
       keywords = COALESCE(sqlc.narg('keywords'), keywords),
       updated_at = now()
 WHERE ulid = $1
RETURNING id, ulid, name, description, lifecycle_state, event_domain, image_url, public_url, keywords, created_at, updated_at;

-- name: UpdateOccurrenceDatesByEventULID :exec
-- Update the start_time and end_time of all occurrences for an event identified by ULID.
-- Used by the FixReview workflow to correct occurrence dates during admin review.
UPDATE event_occurrences
   SET start_time = sqlc.arg('start_time'),
       end_time = sqlc.narg('end_time'),
       updated_at = now()
 WHERE event_id = (SELECT id FROM events WHERE ulid = sqlc.arg('event_ulid'));

-- name: SoftDeleteEvent :exec
UPDATE events
   SET deleted_at = now(),
       deletion_reason = $2,
       lifecycle_state = 'deleted',
       updated_at = now()
 WHERE ulid = $1
   AND deleted_at IS NULL;

-- name: MergeEventIntoDuplicate :exec
UPDATE events e1
   SET merged_into_id = (SELECT e2.id FROM events e2 WHERE e2.ulid = $2),
       deleted_at = now(),
       lifecycle_state = 'deleted',
       updated_at = now()
 WHERE e1.ulid = $1
   AND e1.deleted_at IS NULL;

-- name: CreateEventTombstone :exec
INSERT INTO event_tombstones (event_id, event_uri, deleted_at, deletion_reason, superseded_by_uri, payload)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetEventTombstoneByEventID :one
SELECT id,
       event_id,
       event_uri,
       deleted_at,
       deletion_reason,
       superseded_by_uri,
       payload
  FROM event_tombstones
 WHERE event_id = $1
 ORDER BY deleted_at DESC
 LIMIT 1;

-- name: GetEventTombstoneByEventULID :one
SELECT t.id,
       t.event_id,
       t.event_uri,
       t.deleted_at,
       t.deletion_reason,
       t.superseded_by_uri,
       t.payload
  FROM event_tombstones t
  JOIN events e ON e.id = t.event_id
 WHERE e.ulid = $1
 ORDER BY t.deleted_at DESC
 LIMIT 1;

-- name: CountEventsByLifecycleState :one
SELECT COUNT(*)::bigint AS count
  FROM events
 WHERE lifecycle_state = $1
   AND deleted_at IS NULL;

-- name: CountAllEvents :one
SELECT COUNT(*)::bigint AS count
  FROM events
 WHERE deleted_at IS NULL;

-- name: CountEventsCreatedSince :one
SELECT COUNT(*)::bigint AS count
  FROM events
 WHERE created_at >= $1
   AND deleted_at IS NULL;

-- name: CountUpcomingEvents :one
SELECT COUNT(DISTINCT e.id)::bigint AS count
  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
 WHERE o.start_time > NOW()
   AND e.deleted_at IS NULL;

-- name: CountPastEvents :one
SELECT COUNT(DISTINCT e.id)::bigint AS count
  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
 WHERE o.start_time <= NOW()
   AND e.deleted_at IS NULL;

-- name: GetEventDateRange :one
SELECT MIN(o.start_time) AS oldest_event_date,
       MAX(o.start_time) AS newest_event_date
  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
 WHERE e.deleted_at IS NULL;
