-- Run cleanup takes the registry lock first, then parent tables before children.
-- name: LockLoadtestRunTables :exec
LOCK TABLE events, event_occurrences, event_sources, field_provenance,
 event_changes, event_review_queue, event_not_duplicates, idempotency_keys,
 event_tombstones, sources, api_keys, scraper_sources IN SHARE ROW EXCLUSIVE MODE;

-- name: ListLoadtestRunCandidates :many
SELECT sqlc.embed(es), sqlc.embed(e)
FROM event_sources es JOIN events e ON e.id=es.event_id
WHERE rtrim(split_part(split_part(es.source_url, '#', 1), '?', 1), '/')=sqlc.arg('source_url')::text
ORDER BY e.id, es.id;

-- name: ListLoadtestRunEventSources :many
SELECT * FROM event_sources WHERE event_id=sqlc.arg('event_id')::uuid ORDER BY id;

-- name: ListLoadtestRunOccurrences :many
SELECT * FROM event_occurrences WHERE event_id=sqlc.arg('event_id')::uuid ORDER BY id;

-- name: ListLoadtestRunReviews :many
SELECT * FROM event_review_queue WHERE event_id=sqlc.arg('event_id')::uuid ORDER BY id;

-- name: GetLoadtestRunLifecycle :one
SELECT
 (SELECT count(*) FROM event_changes ec WHERE ec.event_id=e.id)::bigint AS change_count,
 (SELECT count(*) FROM event_changes ec WHERE ec.event_id=e.id AND ec.action='create'
   AND ec.source_id IS NULL AND ec.user_id IS NULL AND ec.snapshot=to_jsonb(e))::bigint AS owned_create_count,
 (SELECT count(*) FROM field_provenance fp WHERE fp.event_id=e.id)::bigint AS field_count,
 (EXISTS(SELECT 1 FROM events other WHERE other.merged_into_id=e.id)
  OR EXISTS(SELECT 1 FROM event_review_queue rq WHERE rq.duplicate_of_event_id=e.id)
  OR EXISTS(SELECT 1 FROM event_not_duplicates nd WHERE nd.event_id_a=e.ulid OR nd.event_id_b=e.ulid)
  OR EXISTS(SELECT 1 FROM idempotency_keys ik WHERE ik.event_id=e.id OR ik.event_ulid=e.ulid)
  OR EXISTS(SELECT 1 FROM event_tombstones et WHERE et.event_id=e.id)
  OR EXISTS(SELECT 1 FROM source_repair_attribution_archive a
    WHERE a.original_attribution->>'event_id'=e.id::text
      OR a.canonical_attribution_id IN (SELECT es.id FROM event_sources es WHERE es.event_id=e.id)))::boolean AS protected_reference
FROM events e WHERE e.id=sqlc.arg('event_id')::uuid;

-- name: DeleteLoadtestRunChanges :execrows
DELETE FROM event_changes WHERE event_id=ANY(sqlc.arg('event_ids')::uuid[]);

-- name: DeleteLoadtestRunEvents :execrows
DELETE FROM events WHERE id=ANY(sqlc.arg('event_ids')::uuid[]);
