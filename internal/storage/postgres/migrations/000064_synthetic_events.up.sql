BEGIN;

ALTER TABLE events ADD COLUMN synthetic boolean NOT NULL DEFAULT false;

-- Retain the external_key conflict target for older binaries during cutover.
ALTER TABLE event_series ADD COLUMN synthetic_external_key text UNIQUE;

ALTER TABLE event_review_queue ADD COLUMN synthetic boolean NOT NULL DEFAULT false;
DROP INDEX idx_review_queue_unique_pending_source;
CREATE UNIQUE INDEX idx_review_queue_unique_pending_source
    ON event_review_queue (source_id, source_external_id, synthetic)
    WHERE status = 'pending';

COMMIT;
