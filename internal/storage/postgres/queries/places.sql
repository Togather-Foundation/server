-- SQLc queries for places domain.

-- name: ListPlaces :many
SELECT p.id,
       p.ulid,
       p.name,
       p.description,
       p.address_locality,
       p.address_region,
       p.address_country,
       p.created_at,
       p.updated_at
  FROM places p
 WHERE ($1 = '' OR p.address_locality ILIKE '%' || $1 || '%')
   AND ($2 = '' OR p.name ILIKE '%' || $2 || '%' OR p.description ILIKE '%' || $2 || '%')
   AND (
     $3::timestamptz IS NULL OR
     p.created_at > $3::timestamptz OR
     (p.created_at = $3::timestamptz AND p.ulid > $4)
   )
 ORDER BY p.created_at ASC, p.ulid ASC
 LIMIT $5;

-- name: GetPlaceByULID :one
SELECT p.id,
       p.ulid,
       p.name,
       p.description,
       p.address_locality,
       p.address_region,
       p.address_country,
       p.created_at,
       p.updated_at
  FROM places p
 WHERE p.ulid = $1;
