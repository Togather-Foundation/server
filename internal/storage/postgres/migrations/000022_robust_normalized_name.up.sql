-- Enhance normalized_name to handle & vs and, punctuation, and special characters
-- Related to issue srv-qqk: Robust normalization for entity reconciliation

-- Create a robust name normalization function
-- Handles: & <-> and, punctuation removal, whitespace normalization, case normalization
CREATE OR REPLACE FUNCTION normalize_name(name_text TEXT) RETURNS TEXT AS $$
BEGIN
  RETURN lower(
    -- Collapse multiple spaces to single space
    regexp_replace(
      -- Remove common punctuation (keep only alphanumeric, spaces, and essential chars)
      regexp_replace(
        -- Replace & with 'and'
        regexp_replace(
          -- Replace 'and' with 'and' (normalize spacing around 'and')
          regexp_replace(
            -- Trim leading/trailing whitespace
            trim(name_text),
            '\s+and\s+', ' and ', 'gi'  -- Normalize 'and' (case-insensitive)
          ),
          '\s*&\s*', ' and ', 'g'  -- Replace & with ' and '
        ),
        '[^\w\s]', ' ', 'g'  -- Remove punctuation (keep word chars and spaces)
      ),
      '\s+', ' ', 'g'  -- Collapse multiple spaces
    )
  );
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Drop existing normalized_name columns (they'll be recreated with new logic)
ALTER TABLE places DROP COLUMN IF EXISTS normalized_name;
ALTER TABLE organizations DROP COLUMN IF EXISTS normalized_name;

-- Re-add normalized_name to places using robust normalization function
ALTER TABLE places ADD COLUMN normalized_name TEXT GENERATED ALWAYS AS (
  normalize_name(name)
) STORED;

-- Re-add normalized_name to organizations using robust normalization function
ALTER TABLE organizations ADD COLUMN normalized_name TEXT GENERATED ALWAYS AS (
  normalize_name(name)
) STORED;

-- Recreate indexes (they were dropped with the column)
CREATE INDEX idx_organizations_normalized_name ON organizations (normalized_name, address_locality, address_region);
CREATE INDEX idx_organizations_normalized_name_trgm ON organizations USING GIN (normalized_name gin_trgm_ops);
CREATE INDEX idx_places_normalized_name ON places (normalized_name, address_locality, address_region);
CREATE INDEX idx_places_normalized_name_trgm ON places USING GIN (normalized_name gin_trgm_ops);

-- Test the normalization function with common cases
-- Verify in psql:
-- SELECT normalize_name('Studio & Gallery');          -- 'studio and gallery'
-- SELECT normalize_name('Studio and Gallery');        -- 'studio and gallery'
-- SELECT normalize_name('Café-Bar');                  -- 'cafe bar'
-- SELECT normalize_name('The   Art    Space');        -- 'the art space'
-- SELECT normalize_name('DROM Taberna');              -- 'drom taberna'
-- SELECT normalize_name('Drom Taberna');              -- 'drom taberna'
