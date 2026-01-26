-- SQLc queries for federation sync.

-- name: CreateFederationNode :one
INSERT INTO federation_nodes (
  node_domain,
  node_name,
  base_url,
  api_version,
  geographic_scope,
  trust_level,
  federation_status,
  sync_enabled,
  sync_direction,
  contact_email,
  contact_name,
  notes
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetFederationNodeByID :one
SELECT *
FROM federation_nodes
WHERE id = $1;

-- name: GetFederationNodeByDomain :one
SELECT *
FROM federation_nodes
WHERE node_domain = $1;

-- name: ListFederationNodes :many
SELECT *
FROM federation_nodes
WHERE (sqlc.narg('federation_status') = '' OR federation_status = sqlc.narg('federation_status'))
  AND (sqlc.narg('sync_enabled')::boolean IS NULL OR sync_enabled = sqlc.narg('sync_enabled'))
  AND (sqlc.narg('is_online')::boolean IS NULL OR is_online = sqlc.narg('is_online'))
ORDER BY node_name ASC
LIMIT sqlc.arg('limit');

-- name: UpdateFederationNode :one
UPDATE federation_nodes
SET
  node_name = COALESCE(sqlc.narg('node_name'), node_name),
  base_url = COALESCE(sqlc.narg('base_url'), base_url),
  api_version = COALESCE(sqlc.narg('api_version'), api_version),
  geographic_scope = COALESCE(sqlc.narg('geographic_scope'), geographic_scope),
  trust_level = COALESCE(sqlc.narg('trust_level'), trust_level),
  federation_status = COALESCE(sqlc.narg('federation_status'), federation_status),
  sync_enabled = COALESCE(sqlc.narg('sync_enabled'), sync_enabled),
  sync_direction = COALESCE(sqlc.narg('sync_direction'), sync_direction),
  contact_email = COALESCE(sqlc.narg('contact_email'), contact_email),
  contact_name = COALESCE(sqlc.narg('contact_name'), contact_name),
  notes = COALESCE(sqlc.narg('notes'), notes),
  updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteFederationNode :exec
DELETE FROM federation_nodes
WHERE id = $1;

-- name: UpdateFederationNodeSyncStatus :exec
UPDATE federation_nodes
SET
  last_sync_at = $2,
  last_successful_sync_at = CASE WHEN $3 THEN $2 ELSE last_successful_sync_at END,
  sync_cursor = COALESCE($4, sync_cursor),
  last_error_at = CASE WHEN NOT $3 THEN now() ELSE last_error_at END,
  last_error_message = CASE WHEN NOT $3 THEN $5 ELSE last_error_message END,
  updated_at = now()
WHERE id = $1;

-- name: UpdateFederationNodeHealth :exec
UPDATE federation_nodes
SET
  is_online = $2,
  last_health_check_at = now(),
  updated_at = now()
WHERE id = $1;

-- Federation Sync Queries

-- name: GetEventByFederationURI :one
SELECT *
FROM events
WHERE federation_uri = $1
LIMIT 1;

-- name: UpsertFederatedEvent :one
INSERT INTO events (
  ulid,
  name,
  description,
  lifecycle_state,
  event_status,
  attendance_mode,
  organizer_id,
  primary_venue_id,
  series_id,
  image_url,
  public_url,
  virtual_url,
  keywords,
  in_language,
  default_language,
  is_accessible_for_free,
  accessibility_features,
  event_domain,
  origin_node_id,
  federation_uri,
  license_url,
  license_status,
  confidence,
  quality_score,
  version,
  created_at,
  updated_at,
  published_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26, $27, $28
)
ON CONFLICT (federation_uri)
WHERE federation_uri IS NOT NULL
DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  lifecycle_state = EXCLUDED.lifecycle_state,
  event_status = EXCLUDED.event_status,
  attendance_mode = EXCLUDED.attendance_mode,
  organizer_id = EXCLUDED.organizer_id,
  primary_venue_id = EXCLUDED.primary_venue_id,
  image_url = EXCLUDED.image_url,
  public_url = EXCLUDED.public_url,
  virtual_url = EXCLUDED.virtual_url,
  keywords = EXCLUDED.keywords,
  in_language = EXCLUDED.in_language,
  default_language = EXCLUDED.default_language,
  is_accessible_for_free = EXCLUDED.is_accessible_for_free,
  accessibility_features = EXCLUDED.accessibility_features,
  event_domain = EXCLUDED.event_domain,
  confidence = EXCLUDED.confidence,
  quality_score = EXCLUDED.quality_score,
  version = events.version + 1,
  updated_at = now()
RETURNING *;

-- name: CreateFederatedEventOccurrence :exec
INSERT INTO event_occurrences (
  event_id,
  start_time,
  end_time,
  timezone,
  virtual_url
) VALUES (
  $1, $2, $3, $4, $5
);
