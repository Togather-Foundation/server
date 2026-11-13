-- SQLc queries for sources registry.

-- name: CountAllSources :one
SELECT COUNT(*) FROM sources;

-- name: LockSourceRegistry :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended('togather:sources:registry', 0));

-- name: LockSourceFeed :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(feed_url)::text, 0));

-- name: GetCanonicalSourcePolicy :one
SELECT id::text, trust_level, license_url, license_type
FROM sources WHERE feed_url = sqlc.arg(feed_url)::text;

-- name: ListLegacyFeedSources :many
SELECT s.id::text, s.trust_level, s.license_url, s.license_type,
 EXISTS (
   SELECT 1 FROM scraper_sources ss,
   LATERAL unnest(CASE WHEN cardinality(ss.urls)>0 THEN ss.urls ELSE ARRAY[ss.url] END) u(url)
   WHERE ss.name=s.name
     AND rtrim(split_part(split_part(u.url, '#', 1), '?', 1), '/') = sqlc.arg(feed_url)::text
 )::boolean AS configured
FROM sources s
WHERE s.feed_url IS NULL
 AND rtrim(split_part(split_part(s.base_url, '#', 1), '?', 1), '/') = sqlc.arg(feed_url)::text
 AND NOT EXISTS (
   SELECT 1 FROM event_sources es WHERE es.source_id=s.id
    AND rtrim(split_part(split_part(es.source_url, '#', 1), '?', 1), '/') <> sqlc.arg(feed_url)::text
 )
ORDER BY configured DESC, s.created_at, s.id;

-- name: AdoptLegacyFeedSource :one
UPDATE sources SET feed_url = sqlc.arg(feed_url)::text,
                   base_url = sqlc.arg(feed_url)::text
WHERE id = sqlc.arg(id)::uuid AND feed_url IS NULL
RETURNING id::text;

-- name: RelabelCanonicalSource :exec
UPDATE sources SET name = sqlc.arg(name)::text
WHERE feed_url = sqlc.arg(feed_url)::text AND name <> sqlc.arg(name)::text;

-- name: InsertCanonicalSource :one
INSERT INTO sources (name, source_type, base_url, feed_url, license_url, license_type, trust_level)
VALUES (sqlc.arg(name), sqlc.arg(source_type), sqlc.arg(feed_url)::text, sqlc.arg(feed_url)::text,
        sqlc.arg(license_url), sqlc.arg(license_type), sqlc.arg(trust_level))
ON CONFLICT (feed_url) WHERE feed_url IS NOT NULL DO UPDATE SET name = sources.name
RETURNING id::text;

-- name: UpsertSourceWithoutURL :one
INSERT INTO sources (name, source_type, base_url, license_url, license_type, trust_level)
VALUES (sqlc.arg(name), sqlc.arg(source_type), NULL, sqlc.arg(license_url), sqlc.arg(license_type), sqlc.arg(trust_level))
ON CONFLICT (name) WHERE base_url IS NULL DO UPDATE SET name = EXCLUDED.name
RETURNING id::text;
