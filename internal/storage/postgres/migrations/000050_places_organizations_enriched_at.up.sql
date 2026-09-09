-- Migration: places_organizations_enriched_at
-- Description: Add an enriched_at marker to places and organizations.
--
-- EnrichmentWorker sets enriched_at = now() on a successful Artsdata dereference
-- (200 + parsed). The freshness skip in EnrichmentWorker.Work then skips
-- re-dereferencing entities whose enriched_at is within
-- ARTSDATA_ENRICH_REFRESH_DAYS. NULL means "never enriched" and always backfills.

ALTER TABLE places ADD COLUMN enriched_at TIMESTAMPTZ;

ALTER TABLE organizations ADD COLUMN enriched_at TIMESTAMPTZ;
