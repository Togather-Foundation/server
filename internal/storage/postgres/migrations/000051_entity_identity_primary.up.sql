-- Migration: entity_identity_primary
-- Description: Add primary-identifier slot to entity_identifiers, broaden authority URI
-- patterns, backfill one primary per (entity, authority), and add the append-only
-- identity_decisions + signal-scoped identity_not_duplicates stores plus merge indexes.
-- See: specs/007-entity-identity-adjudication/spec-phase1.md

-- Primary-slot columns on entity_identifiers.
ALTER TABLE entity_identifiers
  ADD COLUMN observed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN is_primary       BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN superseded_by_id INTEGER REFERENCES entity_identifiers(id),
  ADD COLUMN source           TEXT;   -- nullable for legacy rows

-- Broaden http://-anchored authority patterns to https?:// (artsdata, wikidata);
-- isni/musicbrainz/osm are already https-anchored and unchanged.
UPDATE knowledge_graph_authorities
   SET base_uri_pattern = replace(base_uri_pattern, '^http://', '^https?://')
 WHERE base_uri_pattern LIKE '^http://%';

-- Backfill: elect one primary per group using the canonical order. Pre-`observed_at`,
-- `updated_at` is the recency proxy; `id` is the terminal tie-break.
UPDATE entity_identifiers SET is_primary = false;
WITH ranked AS (
  SELECT ei.id, ROW_NUMBER() OVER (
    PARTITION BY ei.entity_type, ei.entity_id, ei.authority_code
    ORDER BY CASE ei.reconciliation_method
               WHEN 'manual' THEN 5 WHEN 'imported' THEN 4
               WHEN 'auto_high' THEN 3 WHEN 'auto_low' THEN 2
               WHEN 'enrichment_sameas' THEN 1 ELSE 0 END DESC,
             a.trust_level DESC, a.priority_order ASC,
             ei.confidence DESC, ei.updated_at DESC, ei.id DESC) AS rn
  FROM entity_identifiers ei
  JOIN knowledge_graph_authorities a ON a.authority_code = ei.authority_code)
UPDATE entity_identifiers ei SET is_primary = true FROM ranked r
 WHERE ei.id = r.id AND r.rn = 1;

-- Enforce at most one primary per (entity, authority).
CREATE UNIQUE INDEX idx_entity_identifiers_one_primary
  ON entity_identifiers(entity_type, entity_id, authority_code) WHERE is_primary;

-- Append-only decision record for every identity action (link/reject; merge in Phase 2).
CREATE TABLE identity_decisions (
  id TEXT PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  entity_type TEXT NOT NULL CHECK (entity_type IN ('place','organization')),
  entity_id TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('link','reject')),   -- widen in Phase 2
  counterpart_type TEXT, counterpart_id TEXT,
  rationale TEXT NOT NULL DEFAULT '', citations JSONB NOT NULL DEFAULT '[]'::jsonb,
  confidence NUMERIC(5,4) NOT NULL DEFAULT 0, actor TEXT NOT NULL,
  reversible BOOLEAN NOT NULL DEFAULT true, undo_ref TEXT, metadata JSONB
);
CREATE INDEX idx_identity_decisions_entity ON identity_decisions(entity_type, entity_id, created_at DESC, id DESC);
CREATE INDEX idx_identity_decisions_feed   ON identity_decisions(created_at DESC, id DESC);

-- Signal-scoped not-duplicate suppression, keyed by the canonical (id_a < id_b) pair.
CREATE TABLE identity_not_duplicates (
  entity_type TEXT NOT NULL CHECK (entity_type IN ('place','organization')),
  id_a TEXT NOT NULL, id_b TEXT NOT NULL,
  evidence_fingerprint TEXT NOT NULL,
  decision_id TEXT REFERENCES identity_decisions(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_by TEXT NOT NULL,
  PRIMARY KEY (entity_type, id_a, id_b), CHECK (id_a < id_b)
);
CREATE INDEX idx_identity_not_duplicates_b ON identity_not_duplicates(entity_type, id_b);

-- Missing merge-tracking indexes.
CREATE INDEX idx_places_merged_into ON places(merged_into_id);
CREATE INDEX idx_organizations_merged_into ON organizations(merged_into_id);
