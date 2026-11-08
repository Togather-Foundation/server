-- Restore base_url-only source identity from 000023.
--
-- DO NOT run this while a binary that uses per-feed source resolution is
-- running. `GetOrCreateSource` relies on ON CONFLICT (base_url, name), which
-- requires sources_base_url_name_unique; dropping it first makes every ingest
-- fail with:
--   no unique or exclusion constraint matching the ON CONFLICT specification
-- Correct order: roll the application back to the base_url-only build, then run
-- this migration (down).
--
-- WARNING: this rollback fails if more than one source now shares a base_url,
-- which is exactly the per-feed state this migration introduces (e.g. the five
-- TPL BiblioCommons feeds). Merge or delete the extra rows first — event_sources
-- references them, so deleting a row also drops that attribution.

DROP INDEX IF EXISTS sources_base_url_name_unique;

CREATE UNIQUE INDEX IF NOT EXISTS sources_base_url_unique
  ON sources(base_url)
  WHERE base_url IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS sources_name_unique_when_no_url
  ON sources(name)
  WHERE base_url IS NULL;
