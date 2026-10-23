DROP INDEX IF EXISTS idx_api_keys_federation_node;

ALTER TABLE api_keys
  DROP COLUMN IF EXISTS federation_node_id;
