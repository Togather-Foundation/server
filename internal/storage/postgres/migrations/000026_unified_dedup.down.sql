-- Reverse unified duplicate detection: Layer 0 schema changes
--
-- WARNING: The dedup_hash column cannot be reverted to GENERATED ALWAYS
-- without dropping and recreating it, which would lose any application-stored
-- SHA-256 hashes. Since PostgreSQL doesn't support ALTER COLUMN ... SET GENERATED,
-- we leave dedup_hash as a regular column. A full rollback would require
-- manually recreating the column (see migration 000001_core.up.sql lines 189-195).

-- Drop duplicate tracking from review queue
ALTER TABLE event_review_queue DROP COLUMN IF EXISTS duplicate_of_event_id;

-- Drop merge tracking from places and organizations
ALTER TABLE places DROP COLUMN IF EXISTS merged_into_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS merged_into_id;
