-- Drop indexes first
DROP INDEX IF EXISTS idx_user_invitations_active;
DROP INDEX IF EXISTS idx_user_invitations_user;
DROP INDEX IF EXISTS idx_user_invitations_token_hash;

-- Drop table
DROP TABLE IF EXISTS user_invitations;
