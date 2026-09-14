-- Add an explicit all-day marker to event occurrences so date-only events are
-- distinguishable from genuine midnight events. The marker is stored alongside
-- the existing local-midnight instant, which keeps the existing node-local
-- rendering contract intact while letting consumers emit date-only values.
--
-- NOTE: no backfill is attempted. Pre-existing all-day occurrences that were
-- stored as midnight instants remain timed after this migration — midnight is
-- ambiguous, so a reliable heuristic to distinguish "all-day" from "genuine
-- midnight" is impossible. Only occurrences created or corrected after this
-- migration carry an accurate is_all_day marker.
ALTER TABLE event_occurrences ADD COLUMN is_all_day BOOLEAN NOT NULL DEFAULT FALSE;

-- Preserve the all-day marker across the review/re-ingest path: the review
-- queue snapshots occurrence timing in event_start_time/event_end_time, so it
-- must carry an equivalent flag to avoid dropping the marker on review.
ALTER TABLE event_review_queue ADD COLUMN event_all_day BOOLEAN NOT NULL DEFAULT FALSE;
