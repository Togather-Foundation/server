-- Add expiration support for idempotency keys (24-hour TTL)
-- Security issue server-brb: Prevent unbounded table growth

ALTER TABLE idempotency_keys 
  ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours');

-- Index for efficient cleanup of expired keys
CREATE INDEX idx_idempotency_expires ON idempotency_keys(expires_at);
