-- Source reconciliation uses the same snapshot for its dry run and apply.
-- Locks prevent concurrent ingest/config writes from invalidating that snapshot.
-- name: LockSourceRegistryExclusive :exec
SELECT pg_advisory_xact_lock(hashtextextended('togather:sources:registry', 0));

-- name: LockSourceRepairTables :exec
LOCK TABLE sources, event_sources, field_provenance, event_changes, api_keys, scraper_sources IN SHARE ROW EXCLUSIVE MODE;

-- name: ListSourceRepairRegistry :many
SELECT sqlc.embed(s),
       (SELECT count(*) FROM event_sources es WHERE es.source_id=s.id) AS event_references,
       (EXISTS(SELECT 1 FROM field_provenance fp WHERE fp.source_id=s.id)
        OR EXISTS(SELECT 1 FROM event_changes ec WHERE ec.source_id=s.id)
        OR EXISTS(SELECT 1 FROM api_keys ak WHERE ak.source_id=s.id))::boolean AS other_references,
       (s.trust_level <> 5 OR s.license_type <> 'CC0'
        OR s.license_url <> 'https://creativecommons.org/publicdomain/zero/1.0/'
        OR coalesce(s.config, '{}'::jsonb) <> '{}'::jsonb
        OR s.api_endpoint IS NOT NULL OR s.api_key_encrypted IS NOT NULL
        OR coalesce(s.requires_authentication,false)
        OR s.rate_limit_requests IS NOT NULL OR s.rate_limit_window_seconds IS NOT NULL
        OR s.contact_email IS NOT NULL OR s.contact_url IS NOT NULL
        OR s.notes IS NOT NULL OR NOT coalesce(s.is_active,true))::boolean AS metadata_protected
FROM sources s ORDER BY s.created_at, s.id;

-- name: ListSourceRepairConfigs :many
SELECT name, url, urls, trust_level, license, suppress_review_warnings
FROM scraper_sources ORDER BY name;

-- name: ListSourceRepairAttributions :many
SELECT sqlc.embed(es) FROM event_sources es ORDER BY es.id;

-- name: CountSourceRepairArchivedAttributions :one
SELECT count(*) FROM source_repair_attribution_archive;

-- name: ArchiveSourceRepairAttribution :execrows
INSERT INTO source_repair_attribution_archive (
  original_attribution_id, canonical_attribution_id, original_attribution, original_source
)
SELECT es.id, sqlc.arg('canonical_attribution_id')::uuid, to_jsonb(es), to_jsonb(s)
FROM event_sources es JOIN sources s ON s.id=es.source_id
WHERE es.id=sqlc.arg('original_attribution_id')
ON CONFLICT (original_attribution_id) DO NOTHING;

-- name: DeleteArchivedSourceRepairAttribution :execrows
DELETE FROM event_sources es
WHERE es.id=sqlc.arg('attribution_id')
AND EXISTS (
  SELECT 1 FROM source_repair_attribution_archive archived
  WHERE archived.original_attribution_id=es.id AND archived.original_attribution=to_jsonb(es)
);

-- name: MoveSourceRepairAttribution :execrows
UPDATE event_sources SET source_id=sqlc.arg('target_source')
WHERE id=sqlc.arg('attribution_id');

-- name: UseConfiguredSourceTrust :execrows
UPDATE sources SET trust_level=sqlc.arg('new_trust')
WHERE id=sqlc.arg('source_id') AND feed_url=sqlc.arg('feed_url')::text
  AND trust_level=sqlc.arg('old_trust')
  AND license_url=sqlc.arg('license_url') AND license_type=sqlc.arg('license_type');

-- name: MoveSourceRepairFields :execrows
UPDATE field_provenance fp SET source_id=sqlc.arg('target_source')
WHERE fp.event_id=sqlc.arg('event_id') AND fp.source_id=sqlc.arg('old_source')
AND NOT EXISTS (
  SELECT 1 FROM field_provenance twin
  WHERE twin.event_id=fp.event_id AND twin.field_path=fp.field_path
    AND twin.source_id=sqlc.arg('target_source') AND twin.observed_at=fp.observed_at
);

-- name: MoveSourceRepairChanges :execrows
UPDATE event_changes SET source_id=sqlc.arg('target_source')
WHERE event_id=sqlc.arg('event_id') AND source_id=sqlc.arg('old_source');

-- name: CountSourceRepairRetainedFields :one
SELECT count(*) FROM field_provenance fp
WHERE fp.event_id=sqlc.arg('event_id') AND fp.source_id=sqlc.arg('old_source')
AND EXISTS (
  SELECT 1 FROM field_provenance twin
  WHERE twin.event_id=fp.event_id AND twin.field_path=fp.field_path
    AND twin.source_id=sqlc.arg('target_source') AND twin.observed_at=fp.observed_at
);

-- name: ListSourceAuditFragments :many
WITH frag AS (
  SELECT rtrim(split_part(split_part(es.source_url, '#', 1), '?', 1), '/') AS feed,
         count(DISTINCT es.source_id) AS source_rows,
         count(DISTINCT es.event_id) AS events
    FROM event_sources es GROUP BY 1 HAVING count(DISTINCT es.source_id) > 1
)
SELECT f.feed::text AS feed, f.source_rows, f.events,
       bool_and(EXISTS (SELECT 1 FROM scraper_sources ss WHERE ss.name=s.name))::boolean AS all_configured
  FROM frag f
  JOIN event_sources es ON rtrim(split_part(split_part(es.source_url, '#', 1), '?', 1), '/')=f.feed
  JOIN sources s ON s.id=es.source_id
 GROUP BY f.feed,f.source_rows,f.events ORDER BY f.events DESC,f.feed;

-- name: DeleteSourceRepairOrphan :execrows
DELETE FROM sources s WHERE s.id=sqlc.arg('source_id')
  AND NOT EXISTS (SELECT 1 FROM event_sources es WHERE es.source_id=s.id)
  AND NOT EXISTS (SELECT 1 FROM field_provenance fp WHERE fp.source_id=s.id)
  AND NOT EXISTS (SELECT 1 FROM event_changes ec WHERE ec.source_id=s.id)
  AND NOT EXISTS (SELECT 1 FROM api_keys ak WHERE ak.source_id=s.id);
