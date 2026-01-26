DROP TRIGGER IF EXISTS trg_event_tombstone_to_changes ON event_tombstones;
DROP FUNCTION IF EXISTS create_event_change_on_tombstone();
