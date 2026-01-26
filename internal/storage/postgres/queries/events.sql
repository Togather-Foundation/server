-- SQLc queries for events domain.

-- name: ListEvents :many
SELECT e.id,
       e.ulid,
       e.name,
       e.description,
       e.lifecycle_state,
       e.event_domain,
       e.organizer_id,
       e.primary_venue_id,
       e.keywords,
       e.created_at,
       e.updated_at,
       o.start_time
  FROM events e
  JOIN event_occurrences o ON o.event_id = e.id
  LEFT JOIN places p ON p.id = COALESCE(o.venue_id, e.primary_venue_id)
  LEFT JOIN organizations org ON org.id = e.organizer_id
 WHERE ($1::timestamptz IS NULL OR o.start_time >= $1::timestamptz)
   AND ($2::timestamptz IS NULL OR o.start_time <= $2::timestamptz)
   AND ($3 = '' OR p.address_locality ILIKE '%' || $3 || '%')
   AND ($4 = '' OR p.address_region ILIKE '%' || $4 || '%')
   AND ($5 = '' OR p.ulid = $5)
   AND ($6 = '' OR org.ulid = $6)
   AND ($7 = '' OR e.lifecycle_state = $7)
   AND ($8 = '' OR e.event_domain = $8)
   AND ($9 = '' OR (e.name ILIKE '%' || $9 || '%' OR e.description ILIKE '%' || $9 || '%'))
   AND (coalesce(cardinality($10::text[]), 0) = 0 OR e.keywords && $10::text[])
   AND (
     $11::timestamptz IS NULL OR
     o.start_time > $11::timestamptz OR
     (o.start_time = $11::timestamptz AND e.ulid > $12)
   )
 ORDER BY o.start_time ASC, e.ulid ASC
 LIMIT $13;

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
