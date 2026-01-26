ALTER TABLE organizations
  ADD COLUMN deleted_at TIMESTAMPTZ,
  ADD COLUMN deletion_reason TEXT;

CREATE TABLE organization_tombstones (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL,
  organization_uri TEXT NOT NULL,
  deleted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deletion_reason TEXT,
  superseded_by_uri TEXT,
  payload JSONB NOT NULL
);

CREATE INDEX idx_organization_tombstones_org ON organization_tombstones (organization_id, deleted_at DESC);
CREATE INDEX idx_organization_tombstones_uri ON organization_tombstones (organization_uri);
