-- Create user_invitations table for email-based user invitation system
CREATE TABLE user_invitations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for constant-time token lookup (covering index with partial filter)
-- This prevents timing attacks by ensuring all lookups take the same time
CREATE INDEX idx_user_invitations_token_hash ON user_invitations(token_hash, expires_at, accepted_at)
WHERE accepted_at IS NULL;

-- Index for querying invitations by user
CREATE INDEX idx_user_invitations_user ON user_invitations(user_id);

-- Ensure only one active (unaccepted) invitation per user
CREATE UNIQUE INDEX idx_user_invitations_active ON user_invitations(user_id) WHERE accepted_at IS NULL;
