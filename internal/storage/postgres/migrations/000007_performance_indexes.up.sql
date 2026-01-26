-- Add missing performance indexes
-- Security issue server-blq: Improve query performance

-- Index for event_occurrences joins (common query pattern)
-- Optimizes queries like:
--   SELECT * FROM events e JOIN event_occurrences eo ON e.id = eo.event_id WHERE eo.start_time > NOW()
--   SELECT * FROM event_occurrences WHERE event_id = ? ORDER BY start_time
-- Used in: Event detail pages, occurrence listings, time-based filtering
CREATE INDEX idx_event_occurrences_event_start ON event_occurrences(event_id, start_time);

-- Index for federation queries
-- Optimizes queries like:
--   SELECT * FROM events WHERE origin_node_id = ? AND federation_uri = ?
--   SELECT * FROM events WHERE origin_node_id = ? FOR UPDATE
-- Used in: Federation sync, federated event lookups, change feed queries by origin
CREATE INDEX idx_events_federation ON events(origin_node_id, federation_uri);

-- Partial index for soft delete queries (only indexes non-null deleted_at)
-- Optimizes queries like:
--   SELECT * FROM events WHERE deleted_at IS NOT NULL
--   SELECT * FROM events WHERE ulid = ? AND deleted_at IS NOT NULL
-- Used in: Tombstone responses (410 Gone), cleanup jobs, federation tombstone sync
-- Note: Partial index significantly smaller than full index (only deleted events)
CREATE INDEX idx_events_deleted ON events(deleted_at) WHERE deleted_at IS NOT NULL;

