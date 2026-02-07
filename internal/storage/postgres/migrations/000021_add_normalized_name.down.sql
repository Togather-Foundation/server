-- Rollback: Remove normalized_name columns and indexes

-- Drop places indexes and column
DROP INDEX IF EXISTS idx_places_normalized_name_trgm;
DROP INDEX IF EXISTS idx_places_normalized_name;
ALTER TABLE places DROP COLUMN IF EXISTS normalized_name;

-- Drop organizations indexes and column
DROP INDEX IF EXISTS idx_organizations_normalized_name_trgm;
DROP INDEX IF EXISTS idx_organizations_normalized_name;
ALTER TABLE organizations DROP COLUMN IF EXISTS normalized_name;
