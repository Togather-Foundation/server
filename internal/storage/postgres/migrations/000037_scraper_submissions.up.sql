CREATE TABLE scraper_submissions (
  id               BIGSERIAL PRIMARY KEY,
  url              TEXT NOT NULL,
  url_norm         TEXT NOT NULL,
  submitted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  submitter_ip     INET NOT NULL,
  status           TEXT NOT NULL DEFAULT 'pending_validation'
                   CHECK (status IN ('pending_validation', 'pending', 'rejected', 'processed')),
  rejection_reason TEXT,
  notes            TEXT,
  validated_at     TIMESTAMPTZ
);

CREATE INDEX scraper_submissions_url_norm_idx
  ON scraper_submissions(url_norm, submitted_at DESC);
CREATE INDEX scraper_submissions_status_idx
  ON scraper_submissions(status, submitted_at DESC);
CREATE INDEX scraper_submissions_ip_idx
  ON scraper_submissions(submitter_ip, submitted_at DESC);
