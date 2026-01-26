-- Rollback: Remove event_changes triggers

DROP TRIGGER IF EXISTS trg_event_changes_update ON events;
DROP TRIGGER IF EXISTS trg_event_changes_insert ON events;

DROP FUNCTION IF EXISTS create_event_change_on_update();
DROP FUNCTION IF EXISTS create_event_change_on_insert();
