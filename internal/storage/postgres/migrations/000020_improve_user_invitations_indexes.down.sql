-- Drop the optimized indexes
DROP INDEX IF EXISTS idx_user_invitations_token_lookup;
DROP INDEX IF EXISTS idx_user_invitations_expires;
DROP INDEX IF EXISTS idx_user_invitations_token_hash_unique;

-- Restore the original covering index
CREATE INDEX idx_user_invitations_token_hash ON user_invitations(token_hash, expires_at, accepted_at)
WHERE accepted_at IS NULL;
