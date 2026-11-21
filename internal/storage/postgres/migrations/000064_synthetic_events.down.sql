BEGIN;

-- Restore the old pending-review uniqueness. Fail atomically if synthetic
-- and ordinary pending entries still share a source and external ID.
DROP INDEX idx_review_queue_unique_pending_source;
CREATE UNIQUE INDEX idx_review_queue_unique_pending_source
    ON event_review_queue (source_id, source_external_id)
    WHERE status = 'pending';
ALTER TABLE event_review_queue DROP COLUMN synthetic;
ALTER TABLE event_series DROP COLUMN synthetic_external_key;
ALTER TABLE events DROP COLUMN synthetic;

COMMIT;
