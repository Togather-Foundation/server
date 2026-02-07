-- Fix source unique constraints to allow multiple sources with same name but different base_urls
-- Primary deduplication should be by base_url, not name

-- Drop the unique constraint on name
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_name_key;

-- Add unique constraint on base_url (excluding NULLs)
-- Use a partial unique index to handle NULL base_urls gracefully
-- Multiple sources can have NULL base_url, but non-NULL base_urls must be unique
CREATE UNIQUE INDEX IF NOT EXISTS sources_base_url_unique
  ON sources(base_url)
  WHERE base_url IS NOT NULL;

-- For sources with NULL base_url, they must have unique names
-- This handles events without URLs that use source names for identity
CREATE UNIQUE INDEX IF NOT EXISTS sources_name_unique_when_no_url
  ON sources(name)
  WHERE base_url IS NULL;
