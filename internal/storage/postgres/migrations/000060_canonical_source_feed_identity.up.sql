-- Canonical identity is the normalized feed URL; names are labels only.
-- Legacy rows remain unclaimed until explicit, dry-run-first reconciliation.
ALTER TABLE sources ADD COLUMN feed_url TEXT;
CREATE UNIQUE INDEX sources_feed_url_unique ON sources(feed_url)
  WHERE feed_url IS NOT NULL;
-- Retain sources_base_url_name_unique while the previous blue/green slot
-- still uses ON CONFLICT (base_url, name).
