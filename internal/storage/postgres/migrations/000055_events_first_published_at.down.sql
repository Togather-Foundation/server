DROP TRIGGER IF EXISTS trg_events_first_published_at ON events;
DROP FUNCTION IF EXISTS set_event_first_published_at();
ALTER TABLE events DROP COLUMN IF EXISTS first_published_at;
DROP FUNCTION IF EXISTS event_is_public_state(text);
