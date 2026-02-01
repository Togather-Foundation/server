-- Add trigram indexes for description/legal_name ILIKE filters

CREATE INDEX IF NOT EXISTS idx_events_description_trgm ON events USING GIN (description gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_places_description_trgm ON places USING GIN (description gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_organizations_legal_name_trgm ON organizations USING GIN (legal_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_organizations_description_trgm ON organizations USING GIN (description gin_trgm_ops);
