-- Give each scraper feed its own provenance row.
--
-- Source identity becomes (base_url, name) for non-NULL base_urls. A single
-- host can serve several distinct feeds: all five TPL BiblioCommons feeds share
-- base_url = https://gateway.bibliocommons.com, so the base_url-only unique
-- index introduced in 000023 collapsed them into whichever feed created the
-- row first. Every feed's events were then attributed to that one source.
--
-- Historical attribution is intentionally NOT rewritten. event_sources stores
-- only source_id (not the feed name), so a lumped row cannot be split
-- retroactively; existing events keep the source row they were recorded with.
-- Re-running a feed attaches its new per-feed source row to the events it still
-- returns (dedup matches by event identity, then records the attribution).
--
-- Sources with a NULL/empty base_url keep their existing identity rule
-- (name must be unique). Existing rows are preserved; no renames or deletes.

DROP INDEX IF EXISTS sources_base_url_unique;

CREATE UNIQUE INDEX IF NOT EXISTS sources_base_url_name_unique
  ON sources(base_url, name)
  WHERE base_url IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS sources_name_unique_when_no_url
  ON sources(name)
  WHERE base_url IS NULL;
