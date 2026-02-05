-- Drop the existing non-optimal covering index
DROP INDEX IF EXISTS idx_user_invitations_token_hash;

-- Create a UNIQUE index on token_hash for security (prevents duplicate tokens)
-- This is more explicit than relying on the column constraint alone
CREATE UNIQUE INDEX idx_user_invitations_token_hash_unique ON user_invitations(token_hash);

-- Create a separate partial index for expiration cleanup queries
-- This helps with background jobs that clean up expired invitations
CREATE INDEX idx_user_invitations_expires ON user_invitations(expires_at)
WHERE accepted_at IS NULL;

-- Create a covering index for the main lookup query
-- This avoids heap access by including all needed columns
-- Query pattern: WHERE token_hash = $1 AND expires_at > now() AND accepted_at IS NULL
CREATE INDEX idx_user_invitations_token_lookup ON user_invitations(token_hash)
INCLUDE (expires_at, accepted_at, user_id, email, id, created_by, created_at)
WHERE accepted_at IS NULL;
