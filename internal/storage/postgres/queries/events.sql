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
