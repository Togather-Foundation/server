-- Rollback Developer Self-Service API Keys feature
-- Phase 1.1: Foundation tables

-- Remove developer_id from api_keys
DROP INDEX IF EXISTS idx_api_keys_developer;
ALTER TABLE api_keys DROP COLUMN IF EXISTS developer_id;

-- Drop api_key_usage table
DROP TABLE IF EXISTS api_key_usage;

-- Drop developer_invitations table
DROP TABLE IF EXISTS developer_invitations;

-- Drop developers table
DROP TABLE IF EXISTS developers;
