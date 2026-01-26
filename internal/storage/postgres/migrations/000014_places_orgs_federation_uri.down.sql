-- Remove federation_uri columns from places and organizations tables

DROP INDEX IF EXISTS idx_organizations_federated;
DROP INDEX IF EXISTS idx_places_federated;
DROP INDEX IF EXISTS idx_organizations_federation_uri_unique;
DROP INDEX IF EXISTS idx_places_federation_uri_unique;

ALTER TABLE organizations DROP COLUMN IF EXISTS federation_uri;
ALTER TABLE places DROP COLUMN IF EXISTS federation_uri;
