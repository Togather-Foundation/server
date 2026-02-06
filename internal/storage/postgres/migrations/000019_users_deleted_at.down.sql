-- Remove deleted_at column from users table
--
-- ⚠️  WARNING: DATA LOSS MIGRATION ⚠️
-- This migration permanently deletes soft-delete history.
-- All users with deleted_at != NULL will lose their deletion timestamp.
--
-- DO NOT run this migration in production without:
-- 1. Backing up the users table first
-- 2. Verifying no soft-deleted users need to be preserved
-- 3. Understanding that deletion history cannot be recovered
--
-- If you need to preserve soft-deleted users, run this first:
--   DELETE FROM users WHERE deleted_at IS NOT NULL;
--
DROP INDEX IF EXISTS idx_users_deleted_at;
ALTER TABLE users DROP COLUMN deleted_at;
