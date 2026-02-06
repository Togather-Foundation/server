-- Add deleted_at column to users table for soft delete support
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ DEFAULT NULL;

-- Create index for efficient queries that filter out deleted users
CREATE INDEX idx_users_deleted_at ON users(deleted_at);
