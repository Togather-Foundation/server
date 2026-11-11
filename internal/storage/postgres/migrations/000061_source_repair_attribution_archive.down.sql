-- Rolling back this migration discards the historical archive. Export it first.
DROP TABLE IF EXISTS source_repair_attribution_archive;
DROP FUNCTION IF EXISTS reject_source_repair_archive_mutation();
