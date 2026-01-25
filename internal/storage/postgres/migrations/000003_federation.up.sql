CREATE TABLE federation_nodes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  node_domain TEXT NOT NULL UNIQUE,
  node_name TEXT NOT NULL,

  base_url TEXT NOT NULL,
  api_version TEXT NOT NULL DEFAULT 'v1',

  geographic_scope TEXT,
  service_area_geojson JSONB,

  trust_level INTEGER NOT NULL DEFAULT 5 CHECK (trust_level BETWEEN 1 AND 10),
  federation_status TEXT NOT NULL DEFAULT 'pending' CHECK (
    federation_status IN ('pending', 'active', 'paused', 'blocked')
  ),

  sync_enabled BOOLEAN DEFAULT true,
  sync_direction TEXT DEFAULT 'bidirectional' CHECK (
    sync_direction IN ('bidirectional', 'pull_only', 'push_only', 'disabled')
  ),
  last_sync_at TIMESTAMPTZ,
  last_successful_sync_at TIMESTAMPTZ,
  sync_cursor TEXT,

  requires_authentication BOOLEAN DEFAULT true,
  api_key_encrypted BYTEA,

  contact_email TEXT,
  contact_name TEXT,

  config JSONB,

  is_online BOOLEAN DEFAULT true,
  last_health_check_at TIMESTAMPTZ,
  last_error_at TIMESTAMPTZ,
  last_error_message TEXT,

  notes TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_federation_nodes_status ON federation_nodes (federation_status, is_online);
CREATE INDEX idx_federation_nodes_sync ON federation_nodes (sync_enabled, last_sync_at);

ALTER TABLE events
  ADD CONSTRAINT fk_events_origin_node
  FOREIGN KEY (origin_node_id) REFERENCES federation_nodes(id);

ALTER TABLE places
  ADD CONSTRAINT fk_places_origin_node
  FOREIGN KEY (origin_node_id) REFERENCES federation_nodes(id);

ALTER TABLE organizations
  ADD CONSTRAINT fk_organizations_origin_node
  FOREIGN KEY (origin_node_id) REFERENCES federation_nodes(id);

CREATE TABLE event_changes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  event_id UUID NOT NULL,

  action TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete')),
  changed_fields JSONB,
  snapshot JSONB,

  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  sequence_number BIGSERIAL UNIQUE,

  source_id UUID REFERENCES sources(id),
  user_id UUID
);

CREATE INDEX idx_event_changes_event ON event_changes (event_id, changed_at DESC);
CREATE INDEX idx_event_changes_sequence ON event_changes (sequence_number);
CREATE INDEX idx_event_changes_action ON event_changes (action, changed_at DESC);

CREATE TABLE event_tombstones (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID NOT NULL,
  event_uri TEXT NOT NULL,
  deleted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deletion_reason TEXT,
  superseded_by_uri TEXT,
  payload JSONB NOT NULL
);

CREATE INDEX idx_event_tombstones_event ON event_tombstones (event_id, deleted_at DESC);
CREATE INDEX idx_event_tombstones_uri ON event_tombstones (event_uri);