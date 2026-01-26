ALTER TABLE places
  ADD COLUMN deleted_at TIMESTAMPTZ,
  ADD COLUMN deletion_reason TEXT;

CREATE TABLE place_tombstones (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id UUID NOT NULL,
  place_uri TEXT NOT NULL,
  deleted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deletion_reason TEXT,
  superseded_by_uri TEXT,
  payload JSONB NOT NULL
);

CREATE INDEX idx_place_tombstones_place ON place_tombstones (place_id, deleted_at DESC);
CREATE INDEX idx_place_tombstones_uri ON place_tombstones (place_uri);
