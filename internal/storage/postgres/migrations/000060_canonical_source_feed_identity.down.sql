-- The legacy base_url/name index remained present throughout rollout.
DROP INDEX sources_feed_url_unique;
ALTER TABLE sources DROP COLUMN feed_url;
