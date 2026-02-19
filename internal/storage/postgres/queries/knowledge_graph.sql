-- SQLc queries for knowledge graph reconciliation.

-- name: GetActiveAuthorities :many
-- Get active knowledge graph authorities ordered by priority
SELECT * FROM knowledge_graph_authorities
WHERE is_active = true
ORDER BY priority_order ASC;

-- name: GetAuthoritiesForDomain :many
-- Get active authorities applicable to a given event domain
SELECT * FROM knowledge_graph_authorities
WHERE is_active = true AND sqlc.arg('domain')::text = ANY(applicable_domains)
ORDER BY priority_order ASC;

-- name: GetAuthorityByCode :one
SELECT * FROM knowledge_graph_authorities
WHERE authority_code = sqlc.arg('authority_code');

-- name: UpsertEntityIdentifier :one
-- Insert or update an entity identifier (sameAs link)
INSERT INTO entity_identifiers (entity_type, entity_id, authority_code, identifier_uri, confidence, reconciliation_method, is_canonical, metadata)
VALUES (sqlc.arg('entity_type'), sqlc.arg('entity_id'), sqlc.arg('authority_code'), sqlc.arg('identifier_uri'), sqlc.arg('confidence'), sqlc.arg('reconciliation_method'), sqlc.arg('is_canonical'), sqlc.arg('metadata'))
ON CONFLICT (entity_type, entity_id, authority_code, identifier_uri)
DO UPDATE SET
    confidence = EXCLUDED.confidence,
    reconciliation_method = EXCLUDED.reconciliation_method,
    is_canonical = EXCLUDED.is_canonical,
    metadata = EXCLUDED.metadata,
    updated_at = now()
RETURNING *;

-- name: GetEntityIdentifiers :many
-- Get all external identifiers for an entity
SELECT * FROM entity_identifiers
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id')
ORDER BY confidence DESC;

-- name: GetEntityIdentifiersByAuthority :many
-- Get identifiers for an entity from a specific authority
SELECT * FROM entity_identifiers
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id') AND authority_code = sqlc.arg('authority_code')
ORDER BY confidence DESC;

-- name: CountUnreconciledEntities :one
-- Count entities of a type that have no external identifiers
SELECT COUNT(DISTINCT e.ulid)::bigint
FROM (
    SELECT ulid FROM places WHERE deleted_at IS NULL
    UNION ALL
    SELECT ulid FROM organizations WHERE deleted_at IS NULL
) e
LEFT JOIN entity_identifiers ei ON ei.entity_id = e.ulid
WHERE ei.id IS NULL;

-- name: ListUnreconciledPlaces :many
-- Get places that have no external identifiers, ordered by creation date
SELECT p.ulid, p.name, p.street_address, p.address_locality, p.address_region, p.postal_code, p.address_country, p.url
FROM places p
LEFT JOIN entity_identifiers ei ON ei.entity_type = 'place' AND ei.entity_id = p.ulid
WHERE p.deleted_at IS NULL AND ei.id IS NULL
ORDER BY p.created_at ASC
LIMIT sqlc.arg('max_results');

-- name: ListUnreconciledOrganizations :many
-- Get organizations that have no external identifiers, ordered by creation date
SELECT o.ulid, o.name, o.legal_name, o.url, o.address_locality, o.address_region, o.postal_code, o.address_country
FROM organizations o
LEFT JOIN entity_identifiers ei ON ei.entity_type = 'organization' AND ei.entity_id = o.ulid
WHERE o.deleted_at IS NULL AND ei.id IS NULL
ORDER BY o.created_at ASC
LIMIT sqlc.arg('max_results');

-- name: GetReconciliationCache :one
-- Check cache for a previous reconciliation result
SELECT * FROM reconciliation_cache
WHERE entity_type = sqlc.arg('entity_type') AND authority_code = sqlc.arg('authority_code') AND lookup_key = sqlc.arg('lookup_key')
AND expires_at > now();

-- name: UpsertReconciliationCache :one
-- Insert or update a cache entry
INSERT INTO reconciliation_cache (entity_type, authority_code, lookup_key, result_json, is_negative, expires_at)
VALUES (sqlc.arg('entity_type'), sqlc.arg('authority_code'), sqlc.arg('lookup_key'), sqlc.arg('result_json'), sqlc.arg('is_negative'), sqlc.arg('expires_at'))
ON CONFLICT (entity_type, authority_code, lookup_key)
DO UPDATE SET
    result_json = EXCLUDED.result_json,
    hit_count = reconciliation_cache.hit_count + 1,
    is_negative = EXCLUDED.is_negative,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
RETURNING *;

-- name: CleanupExpiredCache :execresult
-- Delete expired cache entries
DELETE FROM reconciliation_cache WHERE expires_at <= now();

-- name: DeleteEntityIdentifier :exec
-- Delete a specific entity identifier
DELETE FROM entity_identifiers WHERE id = sqlc.arg('id');
