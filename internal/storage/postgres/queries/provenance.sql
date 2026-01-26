-- SQLc queries for provenance tracking.

-- name: GetEventSources :many
-- Retrieves all sources for a given event with source metadata and timestamps (FR-029)
SELECT
  es.id,
  es.event_id,
  es.source_id,
  es.source_url,
  es.source_event_id,
  es.retrieved_at,
  es.payload,
  es.payload_hash,
  es.confidence,
  s.name as source_name,
  s.source_type,
  s.trust_level,
  s.license_url,
  s.license_type
FROM event_sources es
JOIN sources s ON s.id = es.source_id
WHERE es.event_id = $1
ORDER BY s.trust_level DESC, es.confidence DESC, es.retrieved_at DESC;

-- name: GetFieldProvenance :many
-- Retrieves field-level provenance for an event, optionally filtered by field paths
-- Includes source metadata and timestamps per FR-024 and FR-029
SELECT
  fp.id,
  fp.event_id,
  fp.field_path,
  fp.value_hash,
  fp.value_preview,
  fp.source_id,
  fp.confidence,
  fp.observed_at,
  fp.applied_to_canonical,
  fp.superseded_at,
  fp.superseded_by_id,
  s.name as source_name,
  s.source_type,
  s.trust_level,
  s.license_url,
  s.license_type
FROM field_provenance fp
JOIN sources s ON s.id = fp.source_id
WHERE fp.event_id = $1
  AND fp.applied_to_canonical = true
  AND fp.superseded_at IS NULL
ORDER BY s.trust_level DESC, fp.confidence DESC, fp.observed_at DESC;

-- name: GetFieldProvenanceForPaths :many
-- Retrieves field-level provenance for specific field paths on an event
SELECT
  fp.id,
  fp.event_id,
  fp.field_path,
  fp.value_hash,
  fp.value_preview,
  fp.source_id,
  fp.confidence,
  fp.observed_at,
  fp.applied_to_canonical,
  fp.superseded_at,
  fp.superseded_by_id,
  s.name as source_name,
  s.source_type,
  s.trust_level,
  s.license_url,
  s.license_type
FROM field_provenance fp
JOIN sources s ON s.id = fp.source_id
WHERE fp.event_id = $1
  AND fp.field_path = ANY($2::text[])
  AND fp.applied_to_canonical = true
  AND fp.superseded_at IS NULL
ORDER BY s.trust_level DESC, fp.confidence DESC, fp.observed_at DESC;

-- name: GetCanonicalFieldValue :one
-- Gets the canonical (winning) field value based on conflict resolution rules
-- Priority: trust_level DESC, confidence DESC, observed_at DESC
SELECT
  fp.id,
  fp.event_id,
  fp.field_path,
  fp.value_hash,
  fp.value_preview,
  fp.source_id,
  fp.confidence,
  fp.observed_at,
  fp.applied_to_canonical,
  s.name as source_name,
  s.source_type,
  s.trust_level,
  s.license_url,
  s.license_type
FROM field_provenance fp
JOIN sources s ON s.id = fp.source_id
WHERE fp.event_id = $1
  AND fp.field_path = $2
  AND fp.applied_to_canonical = true
  AND fp.superseded_at IS NULL
ORDER BY s.trust_level DESC, fp.confidence DESC, fp.observed_at DESC
LIMIT 1;

-- name: GetAllFieldProvenanceHistory :many
-- Gets complete provenance history for a field, including superseded records
SELECT
  fp.id,
  fp.event_id,
  fp.field_path,
  fp.value_hash,
  fp.value_preview,
  fp.source_id,
  fp.confidence,
  fp.observed_at,
  fp.applied_to_canonical,
  fp.superseded_at,
  fp.superseded_by_id,
  s.name as source_name,
  s.source_type,
  s.trust_level,
  s.license_url,
  s.license_type
FROM field_provenance fp
JOIN sources s ON s.id = fp.source_id
WHERE fp.event_id = $1
  AND fp.field_path = $2
ORDER BY fp.observed_at DESC;

-- name: InsertEventSource :one
-- Records a source's contribution to an event with source and received timestamps (FR-029)
INSERT INTO event_sources (
  event_id,
  source_id,
  source_url,
  source_event_id,
  retrieved_at,
  payload,
  payload_hash,
  confidence
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, event_id, source_id, source_url, source_event_id, retrieved_at, payload, payload_hash, confidence;

-- name: InsertFieldProvenance :one
-- Records field-level provenance with source timestamp
INSERT INTO field_provenance (
  event_id,
  field_path,
  value_hash,
  value_preview,
  source_id,
  confidence,
  observed_at,
  applied_to_canonical
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, event_id, field_path, value_hash, value_preview, source_id, confidence, observed_at, applied_to_canonical, superseded_at, superseded_by_id;

-- name: SupersedeFieldProvenance :exec
-- Marks a field provenance record as superseded by a new record
UPDATE field_provenance
SET superseded_at = now(),
    superseded_by_id = $2
WHERE id = $1;

-- name: GetSourceByID :one
-- Retrieves source metadata by ID
SELECT
  id,
  name,
  source_type,
  base_url,
  trust_level,
  license_url,
  license_type,
  is_active,
  created_at,
  updated_at
FROM sources
WHERE id = $1;

-- name: GetSourcesByEventID :many
-- Gets all sources that contributed to an event (deduplicated)
SELECT DISTINCT
  s.id,
  s.name,
  s.source_type,
  s.base_url,
  s.trust_level,
  s.license_url,
  s.license_type,
  s.is_active
FROM sources s
JOIN event_sources es ON es.source_id = s.id
WHERE es.event_id = $1
ORDER BY s.trust_level DESC, s.name ASC;