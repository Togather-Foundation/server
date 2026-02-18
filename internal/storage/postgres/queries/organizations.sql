-- SQLc queries for organizations domain.

-- name: ListOrganizations :many
SELECT o.id,
       o.ulid,
       o.name,
       o.legal_name,
       o.description,
       o.email,
       o.telephone,
       o.url,
       o.address_locality,
       o.address_region,
       o.address_country,
       o.street_address,
       o.postal_code,
       o.organization_type,
       o.federation_uri,
       o.alternate_name,
       o.created_at,
       o.updated_at
  FROM organizations o
 WHERE o.deleted_at IS NULL
   AND ($1 = '' OR o.name ILIKE '%' || $1 || '%' OR o.legal_name ILIKE '%' || $1 || '%')
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
       o.description,
       o.email,
       o.telephone,
       o.url,
       o.address_locality,
       o.address_region,
       o.address_country,
       o.street_address,
       o.postal_code,
       o.organization_type,
       o.federation_uri,
       o.alternate_name,
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

-- name: UpdateOrganization :one
UPDATE organizations
   SET name = COALESCE(sqlc.narg('name'), name),
       description = COALESCE(sqlc.narg('description'), description),
       street_address = COALESCE(sqlc.narg('street_address'), street_address),
       address_locality = COALESCE(sqlc.narg('address_locality'), address_locality),
       address_region = COALESCE(sqlc.narg('address_region'), address_region),
       postal_code = COALESCE(sqlc.narg('postal_code'), postal_code),
       address_country = COALESCE(sqlc.narg('address_country'), address_country),
       telephone = COALESCE(sqlc.narg('telephone'), telephone),
       email = COALESCE(sqlc.narg('email'), email),
       url = COALESCE(sqlc.narg('url'), url),
       updated_at = now()
 WHERE ulid = $1
   AND deleted_at IS NULL
RETURNING id, ulid, name, legal_name, description, email, telephone, url, address_locality, address_region, address_country, street_address, postal_code, organization_type, federation_uri, alternate_name, created_at, updated_at;

-- name: CountAllOrganizations :one
SELECT COUNT(*) FROM organizations WHERE deleted_at IS NULL;
