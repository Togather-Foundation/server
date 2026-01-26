-- Compound index for change feed queries with filtering
-- Covers the common pattern: WHERE sequence_number > X AND action = Y ORDER BY sequence_number
CREATE INDEX idx_event_changes_feed ON event_changes(sequence_number, changed_at) 
WHERE action IS NOT NULL;

-- Additional index for filtered queries by action
CREATE INDEX idx_event_changes_feed_action ON event_changes(action, sequence_number, changed_at)
WHERE action IS NOT NULL;
