-- SQLc queries for places domain.

-- name: ListPlaces :many
SELECT p.id,
       p.ulid,
       p.name,
       p.description,
       p.street_address,
       p.address_locality,
       p.address_region,
       p.postal_code,
       p.address_country,
       p.latitude,
       p.longitude,
       p.telephone,
       p.email,
       p.url,
       p.maximum_attendee_capacity,
       p.venue_type,
       p.federation_uri,
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
       p.street_address,
       p.address_locality,
       p.address_region,
       p.postal_code,
       p.address_country,
       p.latitude,
       p.longitude,
       p.telephone,
       p.email,
       p.url,
       p.maximum_attendee_capacity,
       p.venue_type,
       p.federation_uri,
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
RETURNING id, ulid, name, description, street_address, address_locality, address_region, postal_code, address_country, latitude, longitude, telephone, email, url, maximum_attendee_capacity, venue_type, federation_uri, created_at, updated_at;

-- name: CountAllPlaces :one
SELECT COUNT(*) FROM places WHERE deleted_at IS NULL;
