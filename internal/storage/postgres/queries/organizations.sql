-- SQLc queries for organizations domain.

-- name: ListOrganizations :many
SELECT o.id,
       o.ulid,
       o.name,
       o.legal_name,
       o.url,
       o.created_at,
       o.updated_at
  FROM organizations o
 WHERE ($1 = '' OR o.name ILIKE '%' || $1 || '%' OR o.legal_name ILIKE '%' || $1 || '%')
   AND (
     $2::timestamptz IS NULL OR
     o.created_at > $2::timestamptz OR
     (o.created_at = $2::timestamptz AND o.ulid > $3)
   )
 ORDER BY o.created_at ASC, o.ulid ASC
 LIMIT $4;

-- name: GetOrganizationByULID :one
SELECT o.id,
       o.ulid,
       o.name,
       o.legal_name,
       o.url,
       o.deleted_at,
       o.deletion_reason,
       o.created_at,
       o.updated_at
  FROM organizations o
 WHERE o.ulid = $1;

-- name: SoftDeleteOrganization :exec
UPDATE organizations
   SET deleted_at = now(),
       deletion_reason = $2,
       updated_at = now()
 WHERE ulid = $1
   AND deleted_at IS NULL;

-- name: CreateOrganizationTombstone :exec
INSERT INTO organization_tombstones (organization_id, organization_uri, deleted_at, deletion_reason, superseded_by_uri, payload)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetOrganizationTombstoneByULID :one
SELECT t.id,
       t.organization_id,
       t.organization_uri,
       t.deleted_at,
       t.deletion_reason,
       t.superseded_by_uri,
       t.payload
  FROM organization_tombstones t
  JOIN organizations o ON o.id = t.organization_id
 WHERE o.ulid = $1
 ORDER BY t.deleted_at DESC
 LIMIT 1;

-- name: CountAllOrganizations :one
SELECT COUNT(*) FROM organizations WHERE deleted_at IS NULL;
