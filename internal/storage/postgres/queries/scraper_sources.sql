-- SQLc queries for scraper_sources and linkage tables.

-- name: UpsertScraperSource :one
-- Insert or update a scraper source by name (used by 'server scrape sync').
INSERT INTO scraper_sources (
  name, url, tier, schedule, trust_level, license, enabled,
  max_pages, selectors, notes, last_scraped_at, updated_at
) VALUES (
  sqlc.arg('name'),
  sqlc.arg('url'),
  sqlc.arg('tier'),
  sqlc.arg('schedule'),
  sqlc.arg('trust_level'),
  sqlc.arg('license'),
  sqlc.arg('enabled'),
  sqlc.arg('max_pages'),
  sqlc.arg('selectors'),
  sqlc.arg('notes'),
  sqlc.arg('last_scraped_at'),
  NOW()
)
ON CONFLICT (name) DO UPDATE SET
  url             = EXCLUDED.url,
  tier            = EXCLUDED.tier,
  schedule        = EXCLUDED.schedule,
  trust_level     = EXCLUDED.trust_level,
  license         = EXCLUDED.license,
  enabled         = EXCLUDED.enabled,
  max_pages       = EXCLUDED.max_pages,
  selectors       = EXCLUDED.selectors,
  notes           = EXCLUDED.notes,
  last_scraped_at = COALESCE(EXCLUDED.last_scraped_at, scraper_sources.last_scraped_at),
  updated_at      = NOW()
RETURNING id, name, url, tier, schedule, trust_level, license, enabled,
          max_pages, selectors, notes, last_scraped_at, created_at, updated_at;

-- name: GetScraperSourceByName :one
-- Get a single scraper source by unique name.
SELECT id, name, url, tier, schedule, trust_level, license, enabled,
       max_pages, selectors, notes, last_scraped_at, created_at, updated_at
  FROM scraper_sources
 WHERE name = sqlc.arg('name');

-- name: GetScraperSourceByID :one
-- Get a single scraper source by primary key.
SELECT id, name, url, tier, schedule, trust_level, license, enabled,
       max_pages, selectors, notes, last_scraped_at, created_at, updated_at
  FROM scraper_sources
 WHERE id = sqlc.arg('id');

-- name: ListScraperSources :many
-- List all scraper sources, optionally filtered by enabled flag.
SELECT id, name, url, tier, schedule, trust_level, license, enabled,
       max_pages, selectors, notes, last_scraped_at, created_at, updated_at
  FROM scraper_sources
 WHERE (sqlc.narg('enabled')::boolean IS NULL OR enabled = sqlc.narg('enabled'))
 ORDER BY name ASC;

-- name: UpdateScraperSourceLastScraped :exec
-- Update last_scraped_at timestamp after a successful scrape run.
UPDATE scraper_sources
   SET last_scraped_at = NOW(),
       updated_at      = NOW()
 WHERE name = sqlc.arg('name');

-- name: DeleteScraperSource :exec
-- Delete a scraper source by name.
DELETE FROM scraper_sources WHERE name = sqlc.arg('name');

-- name: LinkOrgScraperSource :exec
-- Associate an organization with a scraper source.
INSERT INTO org_scraper_sources (organization_id, scraper_source_id)
VALUES (sqlc.arg('organization_id'), sqlc.arg('scraper_source_id'))
ON CONFLICT DO NOTHING;

-- name: UnlinkOrgScraperSource :exec
-- Remove an organization↔scraper source association.
DELETE FROM org_scraper_sources
 WHERE organization_id   = sqlc.arg('organization_id')
   AND scraper_source_id = sqlc.arg('scraper_source_id');

-- name: ListScraperSourcesByOrg :many
-- List all scraper sources linked to a given organization.
SELECT s.id, s.name, s.url, s.tier, s.schedule, s.trust_level, s.license, s.enabled,
       s.max_pages, s.selectors, s.notes, s.last_scraped_at, s.created_at, s.updated_at
  FROM scraper_sources s
  JOIN org_scraper_sources l ON l.scraper_source_id = s.id
 WHERE l.organization_id = sqlc.arg('organization_id')
 ORDER BY s.name ASC;

-- name: LinkPlaceScraperSource :exec
-- Associate a place with a scraper source.
INSERT INTO place_scraper_sources (place_id, scraper_source_id)
VALUES (sqlc.arg('place_id'), sqlc.arg('scraper_source_id'))
ON CONFLICT DO NOTHING;

-- name: UnlinkPlaceScraperSource :exec
-- Remove a place↔scraper source association.
DELETE FROM place_scraper_sources
 WHERE place_id           = sqlc.arg('place_id')
   AND scraper_source_id  = sqlc.arg('scraper_source_id');

-- name: ListScraperSourcesByPlace :many
-- List all scraper sources linked to a given place.
SELECT s.id, s.name, s.url, s.tier, s.schedule, s.trust_level, s.license, s.enabled,
       s.max_pages, s.selectors, s.notes, s.last_scraped_at, s.created_at, s.updated_at
  FROM scraper_sources s
  JOIN place_scraper_sources l ON l.scraper_source_id = s.id
 WHERE l.place_id = sqlc.arg('place_id')
 ORDER BY s.name ASC;
