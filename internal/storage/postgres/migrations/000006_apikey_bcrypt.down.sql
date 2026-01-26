-- Rollback bcrypt migration support

DROP INDEX IF EXISTS idx_api_keys_hash_version;

ALTER TABLE api_keys 
  DROP COLUMN IF EXISTS hash_version;
