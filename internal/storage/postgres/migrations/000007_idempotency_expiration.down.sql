DROP INDEX IF EXISTS idx_idempotency_expires;
ALTER TABLE idempotency_keys DROP COLUMN IF EXISTS expires_at;
