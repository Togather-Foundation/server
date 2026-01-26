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
       p.deleted_at,
       p.deletion_reason,
       p.created_at,
       p.updated_at
  FROM places p
 WHERE p.ulid = $1;

-- name: SoftDeletePlace :exec
UPDATE places
   SET deleted_at = now(),
       deletion_reason = $2,
       updated_at = now()
 WHERE ulid = $1
   AND deleted_at IS NULL;

-- name: CreatePlaceTombstone :exec
INSERT INTO place_tombstones (place_id, place_uri, deleted_at, deletion_reason, superseded_by_uri, payload)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetPlaceTombstoneByULID :one
SELECT t.id,
       t.place_id,
       t.place_uri,
       t.deleted_at,
       t.deletion_reason,
       t.superseded_by_uri,
       t.payload
  FROM place_tombstones t
  JOIN places p ON p.id = t.place_id
 WHERE p.ulid = $1
 ORDER BY t.deleted_at DESC
 LIMIT 1;
