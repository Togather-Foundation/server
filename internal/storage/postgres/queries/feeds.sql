-- SQLc queries for change feeds.

-- name: ListEventChanges :many
SELECT ec.id,
       ec.event_id,
       ec.action,
       ec.changed_fields,
       ec.snapshot,
       ec.changed_at,
       ec.sequence_number,
       e.ulid AS event_ulid,
       e.federation_uri,
       e.license_url,
       e.license_status,
       es.source_timestamp,
       e.created_at AS received_timestamp
  FROM event_changes ec
  JOIN events e ON e.id = ec.event_id
  LEFT JOIN (
    SELECT event_id,
           MAX(retrieved_at)::timestamptz AS source_timestamp
      FROM event_sources
     GROUP BY event_id
  ) es ON es.event_id = ec.event_id
 WHERE (sqlc.narg('after_sequence')::bigint IS NULL OR ec.sequence_number > sqlc.narg('after_sequence')::bigint)
   AND (sqlc.narg('after_timestamp')::timestamptz IS NULL OR ec.changed_at >= sqlc.narg('after_timestamp')::timestamptz)
   AND (sqlc.narg('action')::text = '' OR ec.action = sqlc.narg('action')::text)
 ORDER BY ec.sequence_number ASC
 LIMIT sqlc.arg('limit');

-- name: GetEventChangeByID :one
SELECT ec.id,
       ec.event_id,
       ec.action,
       ec.changed_fields,
       ec.snapshot,
       ec.changed_at,
       ec.sequence_number,
       e.ulid AS event_ulid,
       e.federation_uri,
       e.license_url,
       e.license_status,
       es.source_timestamp,
       e.created_at AS received_timestamp
  FROM event_changes ec
  JOIN events e ON e.id = ec.event_id
  LEFT JOIN (
    SELECT event_id,
           MAX(retrieved_at)::timestamptz AS source_timestamp
      FROM event_sources
     GROUP BY event_id
  ) es ON es.event_id = ec.event_id
 WHERE ec.id = $1
 LIMIT 1;

-- name: GetLatestEventChange :one
SELECT ec.sequence_number,
       ec.changed_at
  FROM event_changes ec
 ORDER BY ec.sequence_number DESC
 LIMIT 1;

-- name: ListEventTombstones :many
SELECT et.id,
       et.event_id,
       et.event_uri,
       et.deleted_at,
       et.deletion_reason,
       et.superseded_by_uri,
       et.payload
  FROM event_tombstones et
 WHERE (sqlc.narg('after_timestamp')::timestamptz IS NULL OR et.deleted_at >= sqlc.narg('after_timestamp')::timestamptz)
 ORDER BY et.deleted_at ASC
 LIMIT sqlc.arg('limit');

-- name: GetEventTombstoneByURI :one
SELECT et.id,
       et.event_id,
       et.event_uri,
       et.deleted_at,
       et.deletion_reason,
       et.superseded_by_uri,
       et.payload
  FROM event_tombstones et
 WHERE et.event_uri = $1
 LIMIT 1;
