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
       o.created_at,
       o.updated_at
  FROM organizations o
 WHERE o.ulid = $1;
