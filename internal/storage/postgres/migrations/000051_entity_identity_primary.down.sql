-- Down for: entity_identity_primary

DROP INDEX IF EXISTS idx_organizations_merged_into;
DROP INDEX IF EXISTS idx_places_merged_into;
DROP INDEX IF EXISTS idx_identity_not_duplicates_b;
DROP TABLE IF EXISTS identity_not_duplicates;
DROP INDEX IF EXISTS idx_identity_decisions_feed;
DROP INDEX IF EXISTS idx_identity_decisions_entity;
DROP TABLE IF EXISTS identity_decisions;
DROP INDEX IF EXISTS idx_entity_identifiers_one_primary;
ALTER TABLE entity_identifiers
  DROP COLUMN IF EXISTS source,
  DROP COLUMN IF EXISTS superseded_by_id,
  DROP COLUMN IF EXISTS is_primary,
  DROP COLUMN IF EXISTS observed_at;
-- Authority pattern broadening is data; not reverted on down.
