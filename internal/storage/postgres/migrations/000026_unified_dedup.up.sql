-- Unified duplicate detection: Layer 0 schema changes
--
-- 1. Convert dedup_hash from GENERATED ALWAYS to regular column so the
--    application can store SHA-256 hashes computed in Go.
-- 2. Add duplicate_of_event_id to event_review_queue for tracking which
--    existing event a review-queue entry is a duplicate of.
-- 3. Add merged_into_id to places and organizations for merge tracking.

-- Fix dedup hash: drop generated expression so application can store SHA-256 hash
ALTER TABLE events ALTER COLUMN dedup_hash DROP EXPRESSION;

-- Add duplicate tracking to review queue
ALTER TABLE event_review_queue ADD COLUMN duplicate_of_event_id UUID REFERENCES events(id);

-- Add merge tracking to places and organizations
ALTER TABLE places ADD COLUMN merged_into_id UUID REFERENCES places(id);
ALTER TABLE organizations ADD COLUMN merged_into_id UUID REFERENCES organizations(id);

-- Note: event_review_queue.status has no CHECK constraint (migration 000025),
-- so adding 'merged' as a valid status requires no constraint changes —
-- the status column is free-form TEXT.
