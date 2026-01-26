-- Add federation_uri columns to places and organizations tables
-- This allows tracking the original URIs when syncing federated data

ALTER TABLE places
  ADD COLUMN federation_uri TEXT;

ALTER TABLE organizations
  ADD COLUMN federation_uri TEXT;

-- Add partial unique indexes (like events table)
CREATE UNIQUE INDEX idx_places_federation_uri_unique 
  ON places (federation_uri) 
  WHERE federation_uri IS NOT NULL;

CREATE UNIQUE INDEX idx_organizations_federation_uri_unique 
  ON organizations (federation_uri) 
  WHERE federation_uri IS NOT NULL;

-- Add regular indexes for lookups
CREATE INDEX idx_places_federated 
  ON places (federation_uri) 
  WHERE federation_uri IS NOT NULL;

CREATE INDEX idx_organizations_federated 
  ON organizations (federation_uri) 
  WHERE federation_uri IS NOT NULL;
