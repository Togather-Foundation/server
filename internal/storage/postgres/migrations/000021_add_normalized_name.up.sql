-- Add normalized_name columns to places and organizations for name-based reconciliation
-- Supports deduplication of entities by standardized name + location
-- Related to issue srv-85m: Entity reconciliation

-- Add normalized_name to organizations
-- Normalizes: lowercase, trim, collapse multiple spaces to single space
ALTER TABLE organizations ADD COLUMN normalized_name TEXT GENERATED ALWAYS AS (
  lower(regexp_replace(trim(name), '\s+', ' ', 'g'))
) STORED;

-- Create index for fast lookup by normalized name + location
-- This supports queries like: "find org with this name in this city"
CREATE INDEX idx_organizations_normalized_name ON organizations (normalized_name, address_locality, address_region);

-- Add GIN index for fuzzy matching (uses pg_trgm extension)
CREATE INDEX idx_organizations_normalized_name_trgm ON organizations USING GIN (normalized_name gin_trgm_ops);

-- Add normalized_name to places
ALTER TABLE places ADD COLUMN normalized_name TEXT GENERATED ALWAYS AS (
  lower(regexp_replace(trim(name), '\s+', ' ', 'g'))
) STORED;

-- Create index for fast lookup by normalized name + location
CREATE INDEX idx_places_normalized_name ON places (normalized_name, address_locality, address_region);

-- Add GIN index for fuzzy matching
CREATE INDEX idx_places_normalized_name_trgm ON places USING GIN (normalized_name gin_trgm_ops);

-- Note: We do NOT add UNIQUE constraints here because:
-- 1. Same-named venues can exist in different cities (e.g., "City Theatre" in Toronto vs Montreal)
-- 2. Application logic in UpsertPlace/UpsertOrganization will handle reconciliation
-- 3. UNIQUE constraint would prevent legitimate duplicates across different locations
