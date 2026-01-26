-- Add missing performance indexes
-- Security issue server-blq: Improve query performance

-- Index for event_occurrences joins (common query pattern)
CREATE INDEX idx_event_occurrences_event_start ON event_occurrences(event_id, start_time);

-- Index for federation queries
CREATE INDEX idx_events_federation ON events(origin_node_id, federation_uri);

-- Partial index for soft delete queries (only indexes non-null deleted_at)
CREATE INDEX idx_events_deleted ON events(deleted_at) WHERE deleted_at IS NOT NULL;
