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
