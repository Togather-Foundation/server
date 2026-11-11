-- Colliding historical attributions cannot share the canonical live upsert key.
-- Preserve the complete original attribution and source metadata before retiring
-- the redundant live row. Plain UUIDs intentionally survive subsequent cleanup.
CREATE TABLE source_repair_attribution_archive (
  original_attribution_id UUID PRIMARY KEY,
  canonical_attribution_id UUID NOT NULL,
  original_attribution JSONB NOT NULL,
  original_source JSONB NOT NULL,
  archived_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE FUNCTION reject_source_repair_archive_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'source repair attribution archive is immutable';
END;
$$;

CREATE TRIGGER source_repair_archive_immutable
BEFORE UPDATE OR DELETE ON source_repair_attribution_archive
FOR EACH ROW EXECUTE FUNCTION reject_source_repair_archive_mutation();
