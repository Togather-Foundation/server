-- Per-source suppression of review-gating warnings.
--
-- Warning codes listed here are still generated, stored and shown to
-- reviewers; they simply do not push events into pending_review.
ALTER TABLE scraper_sources
    ADD COLUMN suppress_review_warnings TEXT[] NOT NULL DEFAULT '{}';
