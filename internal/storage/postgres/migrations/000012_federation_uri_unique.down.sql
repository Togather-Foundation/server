-- Rollback: Remove UNIQUE constraint and restore non-unique index

-- Drop the unique index
DROP INDEX IF EXISTS idx_events_federation_uri_unique;

-- Restore the original non-unique index
CREATE INDEX idx_events_federated ON events (federation_uri) 
WHERE federation_uri IS NOT NULL;
