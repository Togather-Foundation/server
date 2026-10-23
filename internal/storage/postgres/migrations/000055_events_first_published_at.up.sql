-- Track the first time an event entered a public lifecycle state.
--
-- events.published_at is NOT this signal: only federation sync writes it (from
-- the federated occurrence startDate), while local publish/unpublish never does.
-- first_published_at is maintained by a BEFORE INSERT OR UPDATE trigger so every
-- path (admin transitions, ingest, federation upsert, imports, raw SQL) records
-- it without a Go chokepoint. NULL means "never public" and is the fail-closed
-- value used by public tombstones and change-feed delete rows.
ALTER TABLE events ADD COLUMN first_published_at TIMESTAMPTZ;

-- Mirrors events.PublicStates (internal/domain/events/visibility.go). Kept in the
-- database so the trigger cannot drift from the SQL visibility predicates.
CREATE OR REPLACE FUNCTION event_is_public_state(state text)
RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
  SELECT state = ANY (ARRAY['published','postponed','rescheduled','sold_out','cancelled','completed']::text[])
$$;

-- One-time idempotent backfill from event_changes public snapshots.
--
-- CRITICAL: the events AFTER UPDATE change-log trigger fires whenever
-- OLD.* IS DISTINCT FROM NEW.*, and a BEFORE trigger's change counts. Disable it
-- so the backfill does not inject a synthetic 'update' row per event into the
-- public feed. Rows created before 000013 with no recorded history stay NULL
-- after the backfill: they fail closed if already deleted, while a row still in a
-- public state is stamped from created_at when it next leaves a public state.
-- Deployment note: golang-migrate runs this file in one implicit transaction, so
-- the DISABLE/ENABLE pair and the backfill UPDATE hold ACCESS EXCLUSIVE on events
-- for the whole backfill window. Run it in a low-traffic window; for a very large
-- event_changes table, batch the UPDATE in a follow-up migration instead.
ALTER TABLE events DISABLE TRIGGER trg_event_changes_update;
UPDATE events e
   SET first_published_at = fp.first_public_at
  FROM (
    SELECT ec.event_id, MIN(ec.changed_at) AS first_public_at
      FROM event_changes ec
     WHERE event_is_public_state(ec.snapshot->>'lifecycle_state')
     GROUP BY ec.event_id
  ) fp
 WHERE e.id = fp.event_id AND e.first_published_at IS NULL;
ALTER TABLE events ENABLE TRIGGER trg_event_changes_update;

-- Maintain first_published_at on every write. created_at/updated_at defaults run
-- before BEFORE triggers, so NEW.created_at is populated on INSERT; on a
-- transition use now() rather than NEW.updated_at (which callers may not set).
CREATE OR REPLACE FUNCTION set_event_first_published_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.first_published_at IS NULL AND event_is_public_state(NEW.lifecycle_state) THEN
      NEW.first_published_at := NEW.created_at;
    END IF;
    RETURN NEW;
  END IF;
  -- An explicit value (import/backfill) always wins.
  IF NEW.first_published_at IS NOT NULL THEN RETURN NEW; END IF;
  IF event_is_public_state(NEW.lifecycle_state) AND NOT event_is_public_state(OLD.lifecycle_state) THEN
    NEW.first_published_at := now();
  ELSIF event_is_public_state(OLD.lifecycle_state) AND NOT event_is_public_state(NEW.lifecycle_state) THEN
    NEW.first_published_at := COALESCE(OLD.created_at, now());
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_events_first_published_at
BEFORE INSERT OR UPDATE ON events FOR EACH ROW
EXECUTE FUNCTION set_event_first_published_at();

COMMENT ON COLUMN events.first_published_at IS 'First time the event entered a public lifecycle state (NULL = never public). Maintained by trg_events_first_published_at; gates public tombstones and change-feed delete rows.';
