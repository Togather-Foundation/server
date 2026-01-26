-- Trigger function to create event_changes record when tombstone is created
CREATE OR REPLACE FUNCTION create_event_change_on_tombstone()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO event_changes (
    event_id,
    action,
    changed_fields,
    snapshot,
    changed_at
  ) VALUES (
    NEW.event_id,
    'delete',
    jsonb_build_object(
      'deleted_at', NEW.deleted_at,
      'deletion_reason', NEW.deletion_reason
    ),
    NEW.payload,
    NEW.deleted_at
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on event_tombstones table
CREATE TRIGGER trg_event_tombstone_to_changes
  AFTER INSERT ON event_tombstones
  FOR EACH ROW
  EXECUTE FUNCTION create_event_change_on_tombstone();

COMMENT ON TRIGGER trg_event_tombstone_to_changes ON event_tombstones IS 
  'Automatically creates event_changes record with action=delete when tombstone is created for federation sync';
