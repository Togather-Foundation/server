DROP TABLE IF EXISTS place_tombstones;

ALTER TABLE places
  DROP COLUMN IF EXISTS deleted_at,
  DROP COLUMN IF EXISTS deletion_reason;
