-- Add UNIQUE constraint to events.federation_uri for federation sync upserts
-- This allows ON CONFLICT (federation_uri) to work correctly in UpsertFederatedEvent
-- Partial unique index ensures uniqueness only for non-NULL values

-- Drop the existing non-unique index
DROP INDEX IF EXISTS idx_events_federated;

-- Create a partial UNIQUE index on federation_uri (excludes NULL values)
CREATE UNIQUE INDEX idx_events_federation_uri_unique ON events (federation_uri) 
WHERE federation_uri IS NOT NULL;
