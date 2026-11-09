-- Index for the feed-scoped event identity lookup.
--
-- FindByFeedEventID matches `rtrim(source_url, '/')` + `source_event_id` once per
-- ingested event. Without this index the planner falls back to a full scan of
-- idx_event_sources_external_id: source_event_id is not a leading column there,
-- so it reads every index page and applies rtrim as a filter (measured on
-- staging: 401 buffers / ~7ms for a single lookup, on a 40 MB table).
--
-- The expression must match the query verbatim (rtrim(source_url, '/')) for the
-- planner to use it. source_event_id leads because it is the selective column
-- (21k distinct values over 28k rows); the URL expression only has to confirm
-- which feed the row came from, and carrying it in the index lets the lookup
-- run index-only.
CREATE INDEX IF NOT EXISTS idx_event_sources_feed_identity
    ON event_sources (source_event_id, rtrim(source_url, '/'));
