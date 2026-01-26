-- Add triggers to populate event_changes on INSERT/UPDATE
-- This enables the change feed API to track all event modifications

-- Trigger function to create event_changes record on INSERT (action = 'create')
CREATE OR REPLACE FUNCTION create_event_change_on_insert()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO event_changes (
    event_id,
    action,
    changed_fields,
    snapshot,
    changed_at
  ) VALUES (
    NEW.id,
    'create',
    '{}'::jsonb,  -- Empty for creates (all fields are new)
    row_to_json(NEW)::jsonb,
    NEW.created_at
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger function to create event_changes record on UPDATE (action = 'update')
CREATE OR REPLACE FUNCTION create_event_change_on_update()
RETURNS TRIGGER AS $$
DECLARE
  changed_fields_json jsonb;
BEGIN
  -- Build array of changed field names
  changed_fields_json := '[]'::jsonb;
  
  IF OLD.name IS DISTINCT FROM NEW.name THEN
    changed_fields_json := changed_fields_json || '["name"]'::jsonb;
  END IF;
  
  IF OLD.description IS DISTINCT FROM NEW.description THEN
    changed_fields_json := changed_fields_json || '["description"]'::jsonb;
  END IF;
  
  IF OLD.lifecycle_state IS DISTINCT FROM NEW.lifecycle_state THEN
    changed_fields_json := changed_fields_json || '["lifecycle_state"]'::jsonb;
  END IF;
  
  IF OLD.event_status IS DISTINCT FROM NEW.event_status THEN
    changed_fields_json := changed_fields_json || '["event_status"]'::jsonb;
  END IF;

  INSERT INTO event_changes (
    event_id,
    action,
    changed_fields,
    snapshot,
    changed_at
  ) VALUES (
    NEW.id,
    'update',
    changed_fields_json,
    row_to_json(NEW)::jsonb,
    NEW.updated_at
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for INSERT
CREATE TRIGGER trg_event_changes_insert
AFTER INSERT ON events
FOR EACH ROW
EXECUTE FUNCTION create_event_change_on_insert();

-- Create trigger for UPDATE
CREATE TRIGGER trg_event_changes_update
AFTER UPDATE ON events
FOR EACH ROW
WHEN (OLD.* IS DISTINCT FROM NEW.*)
EXECUTE FUNCTION create_event_change_on_update();

COMMENT ON FUNCTION create_event_change_on_insert() IS 
  'Automatically creates event_changes record with action=create when new event is inserted';

COMMENT ON FUNCTION create_event_change_on_update() IS 
  'Automatically creates event_changes record with action=update when event is modified';
