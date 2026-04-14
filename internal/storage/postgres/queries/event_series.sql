-- SQLc queries for event_series domain.
-- These are the first queries for this previously-dormant table.
-- Introduced in Phase 3 (srv-i1f0t) to load canonical recurrence data alongside events.

-- name: GetEventSeriesByID :one
-- Fetch a single event_series row by its UUID.
-- Used when loading recurrence metadata for an event via series_id FK.
SELECT id,
       name,
       description,
       series_start_date,
       series_end_date,
       rrule,
       exdates,
       rdates,
       schedule_timezone,
       default_venue_id,
       default_start_time,
       default_end_time,
       organizer_id,
       created_at,
       updated_at
  FROM event_series
 WHERE id = $1::uuid;
