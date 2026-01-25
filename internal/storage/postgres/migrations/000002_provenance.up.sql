CREATE TABLE sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  name TEXT NOT NULL UNIQUE,
  source_type TEXT NOT NULL CHECK (
    source_type IN ('scraper', 'api', 'partner', 'user', 'federation', 'manual')
  ),

  base_url TEXT,
  api_endpoint TEXT,

  trust_level INTEGER NOT NULL DEFAULT 5 CHECK (trust_level BETWEEN 1 AND 10),

  license_url TEXT NOT NULL,
  license_type TEXT NOT NULL CHECK (
    license_type IN ('CC0', 'CC-BY', 'CC-BY-SA', 'proprietary', 'unknown')
  ),

  requires_authentication BOOLEAN DEFAULT false,
  api_key_encrypted BYTEA,

  rate_limit_requests INTEGER,
  rate_limit_window_seconds INTEGER,

  contact_email TEXT,
  contact_url TEXT,

  is_active BOOLEAN DEFAULT true,
  last_successful_fetch TIMESTAMPTZ,
  last_error TIMESTAMPTZ,
  last_error_message TEXT,

  config JSONB,

  notes TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sources_active ON sources (is_active, trust_level DESC);
CREATE INDEX idx_sources_type ON sources (source_type);

CREATE TABLE event_sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  source_id UUID NOT NULL REFERENCES sources(id),

  source_url TEXT NOT NULL,
  source_event_id TEXT,

  retrieved_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  payload JSONB NOT NULL,
  payload_hash TEXT NOT NULL,

  confidence DECIMAL(3, 2) CHECK (confidence BETWEEN 0 AND 1),

  UNIQUE (event_id, source_id, source_url)
);

CREATE INDEX idx_event_sources_event ON event_sources (event_id);
CREATE INDEX idx_event_sources_source ON event_sources (source_id, retrieved_at DESC);
CREATE INDEX idx_event_sources_hash ON event_sources (payload_hash);
CREATE INDEX idx_event_sources_external_id ON event_sources (source_id, source_event_id);

CREATE TABLE field_provenance (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  field_path TEXT NOT NULL,

  value_hash TEXT NOT NULL,
  value_preview TEXT,

  source_id UUID NOT NULL REFERENCES sources(id),
  confidence DECIMAL(3, 2) NOT NULL CHECK (confidence BETWEEN 0 AND 1),

  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_to_canonical BOOLEAN NOT NULL DEFAULT true,
  superseded_at TIMESTAMPTZ,
  superseded_by_id UUID REFERENCES field_provenance(id),

  UNIQUE (event_id, field_path, source_id, observed_at)
);

CREATE INDEX idx_field_provenance_event ON field_provenance (event_id, field_path);
CREATE INDEX idx_field_provenance_active ON field_provenance (event_id, field_path)
  WHERE applied_to_canonical = true AND superseded_at IS NULL;
CREATE INDEX idx_field_provenance_source ON field_provenance (source_id, observed_at DESC);

CREATE VIEW field_conflicts AS
SELECT
  fp.event_id,
  fp.field_path,
  COUNT(DISTINCT fp.value_hash) as value_count,
  array_agg(
    jsonb_build_object(
      'source', s.name,
      'trust_level', s.trust_level,
      'confidence', fp.confidence,
      'value_preview', fp.value_preview,
      'observed_at', fp.observed_at
    ) ORDER BY s.trust_level DESC, fp.confidence DESC
  ) as conflicting_values
FROM field_provenance fp
JOIN sources s ON fp.source_id = s.id
WHERE fp.applied_to_canonical = true
  AND fp.superseded_at IS NULL
GROUP BY fp.event_id, fp.field_path
HAVING COUNT(DISTINCT fp.value_hash) > 1;
