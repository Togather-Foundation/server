-- SQLc queries for places domain.

-- name: ListPlacesByCreatedAt :many
SELECT sqlc.embed(p)
  FROM places p
 WHERE p.deleted_at IS NULL
   AND (sqlc.narg('city')::text IS NULL OR p.address_locality ILIKE '%' || sqlc.narg('city') || '%')
   AND (sqlc.narg('query')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('query') || '%' OR p.description ILIKE '%' || sqlc.narg('query') || '%')
   AND (
     sqlc.narg('cursor_timestamp')::timestamptz IS NULL OR
     p.created_at > sqlc.narg('cursor_timestamp')::timestamptz OR
     (p.created_at = sqlc.narg('cursor_timestamp')::timestamptz AND p.ulid > sqlc.narg('cursor_ulid'))
   )
 ORDER BY p.created_at ASC, p.ulid ASC
 LIMIT sqlc.arg('limit');

-- name: ListPlacesByCreatedAtDesc :many
SELECT sqlc.embed(p)
  FROM places p
 WHERE p.deleted_at IS NULL
   AND (sqlc.narg('city')::text IS NULL OR p.address_locality ILIKE '%' || sqlc.narg('city') || '%')
   AND (sqlc.narg('query')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('query') || '%' OR p.description ILIKE '%' || sqlc.narg('query') || '%')
   AND (
     sqlc.narg('cursor_timestamp')::timestamptz IS NULL OR
     p.created_at < sqlc.narg('cursor_timestamp')::timestamptz OR
     (p.created_at = sqlc.narg('cursor_timestamp')::timestamptz AND p.ulid > sqlc.narg('cursor_ulid'))
   )
 ORDER BY p.created_at DESC, p.ulid ASC
 LIMIT sqlc.arg('limit');

-- name: ListPlacesByName :many
SELECT sqlc.embed(p)
  FROM places p
 WHERE p.deleted_at IS NULL
   AND (sqlc.narg('city')::text IS NULL OR p.address_locality ILIKE '%' || sqlc.narg('city') || '%')
   AND (sqlc.narg('query')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('query') || '%' OR p.description ILIKE '%' || sqlc.narg('query') || '%')
   AND (
     sqlc.narg('cursor_name')::text IS NULL OR
     p.name > sqlc.narg('cursor_name') OR
     (p.name = sqlc.narg('cursor_name') AND p.ulid > sqlc.narg('cursor_ulid'))
   )
 ORDER BY p.name ASC, p.ulid ASC
 LIMIT sqlc.arg('limit');

-- name: ListPlacesByNameDesc :many
SELECT sqlc.embed(p)
  FROM places p
 WHERE p.deleted_at IS NULL
   AND (sqlc.narg('city')::text IS NULL OR p.address_locality ILIKE '%' || sqlc.narg('city') || '%')
   AND (sqlc.narg('query')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('query') || '%' OR p.description ILIKE '%' || sqlc.narg('query') || '%')
   AND (
     sqlc.narg('cursor_name')::text IS NULL OR
     p.name < sqlc.narg('cursor_name') OR
     (p.name = sqlc.narg('cursor_name') AND p.ulid > sqlc.narg('cursor_ulid'))
   )
 ORDER BY p.name DESC, p.ulid ASC
 LIMIT sqlc.arg('limit');

-- name: GetPlaceByULID :one
SELECT sqlc.embed(p)
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

-- name: UpdatePlace :one
UPDATE places
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
RETURNING sqlc.embed(places);

-- name: CountAllPlaces :one
SELECT COUNT(*) FROM places WHERE deleted_at IS NULL;
