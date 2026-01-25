DROP TABLE IF EXISTS event_tombstones;
DROP TABLE IF EXISTS event_changes;

ALTER TABLE events DROP CONSTRAINT IF EXISTS fk_events_origin_node;
ALTER TABLE places DROP CONSTRAINT IF EXISTS fk_places_origin_node;
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS fk_organizations_origin_node;

DROP TABLE IF EXISTS federation_nodes;