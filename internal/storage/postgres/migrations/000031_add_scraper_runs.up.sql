CREATE TABLE scraper_runs (
  id            BIGSERIAL PRIMARY KEY,
  source_name   TEXT NOT NULL,
  source_url    TEXT NOT NULL,
  tier          INT NOT NULL DEFAULT 0,
  started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at  TIMESTAMPTZ,
  status        TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed')),
  events_found  INT NOT NULL DEFAULT 0,
  events_new    INT NOT NULL DEFAULT 0,
  events_dup    INT NOT NULL DEFAULT 0,
  events_failed INT NOT NULL DEFAULT 0,
  error_message TEXT,
  metadata      JSONB
);

CREATE INDEX idx_scraper_runs_source ON scraper_runs(source_name);
CREATE INDEX idx_scraper_runs_started ON scraper_runs(started_at DESC);
