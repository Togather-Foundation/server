-- SQLc queries for entity identity primitives.
-- See: specs/007-entity-identity-adjudication/spec-phase1.md (Task 1/2)

-- name: UpsertObservation :one
-- Insert or refresh an identifier observation without disturbing the primary slot.
-- metadata is non-destructive on conflict: a NULL incoming metadata (the common case —
-- RecordObservation does not carry metadata) preserves the existing row's JSONB.
INSERT INTO entity_identifiers (entity_type, entity_id, authority_code, identifier_uri, confidence, reconciliation_method, is_canonical, metadata, observed_at, is_primary, source)
VALUES (sqlc.arg('entity_type'), sqlc.arg('entity_id'), sqlc.arg('authority_code'), sqlc.arg('identifier_uri'), sqlc.arg('confidence'), sqlc.arg('reconciliation_method'), false, sqlc.arg('metadata'), now(), false, sqlc.arg('source'))
ON CONFLICT (entity_type, entity_id, authority_code, identifier_uri)
DO UPDATE SET
    confidence = EXCLUDED.confidence,
    reconciliation_method = EXCLUDED.reconciliation_method,
    is_canonical = false,
    metadata = COALESCE(EXCLUDED.metadata, entity_identifiers.metadata),
    observed_at = now(),
    source = EXCLUDED.source,
    updated_at = now()
RETURNING *;

-- name: LockIdentityGroup :exec
-- Serialize concurrent elections on one (entity_type, entity_id, authority_code) group.
-- pg_advisory_xact_lock is released automatically at transaction end.
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg('key'), 0));

-- name: ListGroupIdentifiersForUpdate :many
-- Load one authority group's observations joined with authority trust/priority.
-- FOR UPDATE OF ei serializes the group rows against concurrent elections.
SELECT ei.id, ei.authority_code, ei.identifier_uri, ei.reconciliation_method,
       ei.confidence, ei.observed_at, ei.is_primary, ei.source,
       a.trust_level, a.priority_order
FROM entity_identifiers ei
JOIN knowledge_graph_authorities a ON a.authority_code = ei.authority_code
WHERE ei.entity_type = sqlc.arg('entity_type')
  AND ei.entity_id = sqlc.arg('entity_id')
  AND ei.authority_code = sqlc.arg('authority_code')
ORDER BY ei.id
FOR UPDATE OF ei;

-- name: ListEntityIdentifiersForUpdate :many
-- Load all of an entity's identifier observations joined with authority
-- trust/priority. FOR UPDATE OF ei serializes the entity's identifier rows against
-- concurrent elections. Used by the transactional merge path to reassign, dedupe,
-- and re-elect one primary per authority.
SELECT ei.id, ei.authority_code, ei.identifier_uri, ei.reconciliation_method,
       ei.confidence, ei.observed_at, ei.is_primary, ei.source,
       a.trust_level, a.priority_order
FROM entity_identifiers ei
JOIN knowledge_graph_authorities a ON a.authority_code = ei.authority_code
WHERE ei.entity_type = sqlc.arg('entity_type')
  AND ei.entity_id = sqlc.arg('entity_id')
ORDER BY ei.id
FOR UPDATE OF ei;

-- name: SetPrimary :exec
-- Unconditionally mark one identifier row as the group's primary and clear its supersession.
UPDATE entity_identifiers SET is_primary = true, superseded_by_id = NULL, updated_at = now()
WHERE id = sqlc.arg('id');

-- name: DemotePrimaryAndSupersede :exec
-- Demote every primary in the group except the winner, recording the superseding row.
-- superseded_by_id records the *immediate successor* at demotion time, not necessarily
-- the current primary: if that successor is later demoted, it keeps pointing at it.
UPDATE entity_identifiers SET is_primary = false, superseded_by_id = sqlc.arg('winner_id'), updated_at = now()
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id')
  AND authority_code = sqlc.arg('authority_code') AND is_primary AND id <> sqlc.arg('winner_id');

-- name: InsertIdentityNotDuplicate :exec
-- Record a not-duplicate pair; re-decision refreshes the evidence fingerprint and decision.
INSERT INTO identity_not_duplicates (entity_type, id_a, id_b, evidence_fingerprint, decision_id, created_by)
VALUES (sqlc.arg('entity_type'), sqlc.arg('id_a'), sqlc.arg('id_b'), sqlc.arg('evidence_fingerprint'), sqlc.arg('decision_id'), sqlc.arg('created_by'))
ON CONFLICT (entity_type, id_a, id_b) DO UPDATE SET
    evidence_fingerprint = EXCLUDED.evidence_fingerprint,
    decision_id = EXCLUDED.decision_id,
    created_at = now(),
    created_by = EXCLUDED.created_by;

-- name: GetIdentityNotDuplicate :one
-- Fetch a pair's not-duplicate row (for evidence-fingerprint comparison on read).
SELECT * FROM identity_not_duplicates
WHERE entity_type = sqlc.arg('entity_type') AND id_a = sqlc.arg('id_a') AND id_b = sqlc.arg('id_b');

-- name: InsertIdentityDecision :one
-- Append one identity decision. created_at is returned from the database so the
-- caller can populate the returned record.
INSERT INTO identity_decisions (
    id, entity_type, entity_id, action, counterpart_type, counterpart_id,
    rationale, citations, confidence, actor, reversible, undo_ref, metadata
)
VALUES (
    sqlc.arg('id'), sqlc.arg('entity_type'), sqlc.arg('entity_id'), sqlc.arg('action'),
    sqlc.arg('counterpart_type'), sqlc.arg('counterpart_id'), sqlc.arg('rationale'),
    sqlc.arg('citations'), sqlc.arg('confidence'), sqlc.arg('actor'),
    sqlc.arg('reversible'), sqlc.arg('undo_ref'), sqlc.arg('metadata')
)
RETURNING created_at;

-- name: ListIdentityDecisionsByEntity :many
-- List one entity's decisions, newest first (matches idx_identity_decisions_entity).
SELECT * FROM identity_decisions
WHERE entity_type = sqlc.arg('entity_type') AND entity_id = sqlc.arg('entity_id')
ORDER BY created_at DESC, id DESC;

-- name: ListIdentityDecisions :many
-- Keyset-paginated feed over all decisions, newest first (created_at DESC, id DESC).
-- Optional filters: entity_type, action, and an inclusive `since` lower bound on
-- created_at. The keyset cursor is (created_at, id).
SELECT * FROM identity_decisions
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action')::text)
  AND (sqlc.narg('since')::timestamptz IS NULL OR created_at >= sqlc.narg('since')::timestamptz)
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR created_at < sqlc.narg('cursor_created_at')::timestamptz
    OR (created_at = sqlc.narg('cursor_created_at')::timestamptz AND id < sqlc.narg('cursor_id')::text)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit');
