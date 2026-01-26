-- Add hash_version column to api_keys table to support migration from SHA-256 to bcrypt
-- Version 1 = SHA-256 (legacy), Version 2 = bcrypt (secure)

ALTER TABLE api_keys 
  ADD COLUMN hash_version INTEGER NOT NULL DEFAULT 1
  CHECK (hash_version IN (1, 2));

COMMENT ON COLUMN api_keys.hash_version IS 
  'Hash algorithm version: 1=SHA-256 (legacy), 2=bcrypt (secure)';

-- Index for monitoring migration progress
CREATE INDEX idx_api_keys_hash_version ON api_keys (hash_version, is_active);
