DROP TABLE IF EXISTS organization_tombstones;

ALTER TABLE organizations
  DROP COLUMN IF EXISTS deleted_at,
  DROP COLUMN IF EXISTS deletion_reason;
