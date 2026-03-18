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

-- name: DeleteOccurrencesByEventULID :exec
-- Remove all occurrence rows for a soft-deleted event.  Called after absorbing an
-- occurrence into a target series so the source event's orphaned rows are cleaned up.
-- Soft-delete (UPDATE) does not trigger ON DELETE CASCADE, so explicit cleanup is needed.
DELETE FROM event_occurrences
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

-- name: ResolveCanonicalEventULID :one
-- Follow the merged_into_id chain from a given ULID to find the final canonical event.
-- Uses a recursive CTE with a max depth of 10 to prevent infinite loops.
-- Returns the ULID of the final canonical event (the one that is not itself merged).
WITH RECURSIVE chain AS (
    SELECT e.id, e.ulid, e.merged_into_id, 1 AS depth
      FROM events e
     WHERE e.ulid = $1
    UNION ALL
    SELECT e.id, e.ulid, e.merged_into_id, c.depth + 1
      FROM events e
      JOIN chain c ON c.merged_into_id = e.id
     WHERE c.merged_into_id IS NOT NULL
       AND c.depth < 10
)
SELECT ulid FROM chain
 WHERE merged_into_id IS NULL
    OR depth = 10
 ORDER BY depth DESC
 LIMIT 1;

-- name: UpdateMergedIntoChain :exec
-- Flatten existing merge chains: update all events that point to an old target
-- to point to the new canonical target instead. This prevents transitive chains.
-- $1 = old target event ULID (intermediate node being re-pointed)
-- $2 = new canonical target event ULID (final destination)
UPDATE events e_upd
   SET merged_into_id = (SELECT e_new.id FROM events e_new WHERE e_new.ulid = $2),
       updated_at = now()
 WHERE e_upd.merged_into_id = (SELECT e_old.id FROM events e_old WHERE e_old.ulid = $1)
    AND e_upd.ulid != $2;

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

-- name: InsertOccurrence :one
-- Insert a single occurrence and return the created row (including generated UUID).
INSERT INTO event_occurrences (
    event_id,
    start_time,
    end_time,
    timezone,
    door_time,
    venue_id,
    virtual_url,
    ticket_url,
    price_min,
    price_max,
    price_currency,
    availability
) VALUES (
    sqlc.arg('event_id')::uuid,
    sqlc.arg('start_time'),
    sqlc.narg('end_time'),
    sqlc.arg('timezone'),
    sqlc.narg('door_time'),
    sqlc.narg('venue_id')::uuid,
    sqlc.narg('virtual_url'),
    sqlc.narg('ticket_url'),
    sqlc.narg('price_min'),
    sqlc.narg('price_max'),
    NULLIF(sqlc.arg('price_currency'), ''),
    NULLIF(sqlc.arg('availability'), '')
)
RETURNING id, event_id, start_time, end_time, timezone, door_time, venue_id, virtual_url,
          ticket_url, price_min, price_max, price_currency, availability, created_at, updated_at;

-- name: GetOccurrenceByID :one
-- Fetch a single occurrence row by its UUID, scoped to the given event.
SELECT o.id,
       o.event_id,
       o.start_time,
       o.end_time,
       o.timezone,
       o.door_time,
       o.venue_id,
       p.ulid AS venue_ulid,
       o.virtual_url,
       o.ticket_url,
       o.price_min,
       o.price_max,
       o.price_currency,
       o.availability,
       o.created_at,
       o.updated_at
  FROM event_occurrences o
  LEFT JOIN places p ON p.id = o.venue_id
 WHERE o.id = sqlc.arg('id')::uuid
   AND o.event_id = sqlc.arg('event_id')::uuid;

-- name: UpdateOccurrenceByID :one
-- Partial-update a single occurrence row, scoped to the given event.
-- Only non-NULL arguments are applied (COALESCE pattern).
-- venue_id, virtual_url, ticket_url use explicit NULLability via CASE WHEN *_set pattern.
UPDATE event_occurrences
   SET start_time     = COALESCE(sqlc.narg('start_time'), start_time),
       end_time       = CASE WHEN sqlc.narg('end_time_set')::boolean THEN sqlc.narg('end_time') ELSE end_time END,
       timezone       = COALESCE(sqlc.narg('timezone'), timezone),
       door_time      = CASE WHEN sqlc.narg('door_time_set')::boolean THEN sqlc.narg('door_time') ELSE door_time END,
       venue_id       = CASE WHEN sqlc.narg('venue_id_set')::boolean THEN sqlc.narg('venue_id')::uuid ELSE venue_id END,
       virtual_url    = CASE WHEN sqlc.narg('virtual_url_set')::boolean THEN sqlc.narg('virtual_url') ELSE virtual_url END,
       ticket_url     = CASE WHEN sqlc.narg('ticket_url_set')::boolean THEN sqlc.narg('ticket_url') ELSE ticket_url END,
       price_min      = CASE WHEN sqlc.narg('price_min_set')::boolean THEN sqlc.narg('price_min') ELSE price_min END,
       price_max      = CASE WHEN sqlc.narg('price_max_set')::boolean THEN sqlc.narg('price_max') ELSE price_max END,
       price_currency = COALESCE(sqlc.narg('price_currency'), price_currency),
       availability   = COALESCE(sqlc.narg('availability'), availability),
       updated_at     = now()
 WHERE id = sqlc.arg('id')::uuid
   AND event_id = sqlc.arg('event_id')::uuid
RETURNING id, event_id, start_time, end_time, timezone, door_time, venue_id, virtual_url,
          ticket_url, price_min, price_max, price_currency, availability, created_at, updated_at;

-- name: DeleteOccurrenceByID :one
-- Delete a single occurrence by its UUID, scoped to the given event. Returns the deleted ID
-- so callers can detect when no row matched (event mismatch or already deleted).
DELETE FROM event_occurrences
 WHERE id = sqlc.arg('id')::uuid
   AND event_id = sqlc.arg('event_id')::uuid
RETURNING id;

-- name: CountOccurrencesByEventID :one
-- Count occurrences for a given event UUID. Used to enforce last-occurrence guard.
SELECT COUNT(*)::bigint AS count
  FROM event_occurrences
 WHERE event_id = sqlc.arg('event_id')::uuid;
