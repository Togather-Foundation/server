-- Add external_key to event_series for ICS ingest upsert idempotency.
-- Value convention: "<source_name>:<master_uid>" (e.g. "myorg-ical:EVT-12345").
-- UNIQUE constraint enables ON CONFLICT (external_key) DO UPDATE upsert semantics.
ALTER TABLE event_series
  ADD COLUMN external_key TEXT UNIQUE;
