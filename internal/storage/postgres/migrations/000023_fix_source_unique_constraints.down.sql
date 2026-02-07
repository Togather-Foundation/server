-- Rollback source unique constraint changes
-- Restore original UNIQUE constraint on name

-- Drop the conditional unique indexes
DROP INDEX IF EXISTS sources_name_unique_when_no_url;
DROP INDEX IF EXISTS sources_base_url_unique;

-- Restore the original unique constraint on name
-- Note: This may fail if there are duplicate names in the database
ALTER TABLE sources ADD CONSTRAINT sources_name_key UNIQUE (name);
