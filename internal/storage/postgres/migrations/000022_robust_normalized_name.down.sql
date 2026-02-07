-- Rollback: Revert to simple normalized_name logic

-- Drop indexes
DROP INDEX IF EXISTS idx_places_normalized_name_trgm;
DROP INDEX IF EXISTS idx_places_normalized_name;
DROP INDEX IF EXISTS idx_organizations_normalized_name_trgm;
DROP INDEX IF EXISTS idx_organizations_normalized_name;

-- Drop columns
ALTER TABLE organizations DROP COLUMN IF EXISTS normalized_name;
ALTER TABLE places DROP COLUMN IF EXISTS normalized_name;

-- Drop normalization function
DROP FUNCTION IF EXISTS normalize_name(TEXT);

-- Restore simple normalized_name logic (from migration 000021)
ALTER TABLE organizations ADD COLUMN normalized_name TEXT GENERATED ALWAYS AS (
  lower(regexp_replace(TRIM(name), '\s+', ' ', 'g'))
) STORED;

ALTER TABLE places ADD COLUMN normalized_name TEXT GENERATED ALWAYS AS (
  lower(regexp_replace(TRIM(name), '\s+', ' ', 'g'))
) STORED;

-- Recreate simple indexes
CREATE INDEX idx_organizations_normalized_name ON organizations (normalized_name, address_locality, address_region);
CREATE INDEX idx_organizations_normalized_name_trgm ON organizations USING GIN (normalized_name gin_trgm_ops);
CREATE INDEX idx_places_normalized_name ON places (normalized_name, address_locality, address_region);
CREATE INDEX idx_places_normalized_name_trgm ON places USING GIN (normalized_name gin_trgm_ops);
