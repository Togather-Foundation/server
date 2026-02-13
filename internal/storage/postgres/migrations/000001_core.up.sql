CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS postgis;

-- WORKAROUND: Some PostGIS Docker images don't populate spatial_ref_sys automatically
-- Manually insert SRID 4326 (WGS 84) which is required for geography/geometry operations
INSERT INTO spatial_ref_sys (srid, auth_name, auth_srid, proj4text, srtext)
VALUES (4326, 'EPSG', 4326, '+proj=longlat +datum=WGS84 +no_defs', 'GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563,AUTHORITY["EPSG","7030"]],AUTHORITY["EPSG","6326"]],PRIMEM["Greenwich",0,AUTHORITY["EPSG","8901"]],UNIT["degree",0.0174532925199433,AUTHORITY["EPSG","9122"]],AUTHORITY["EPSG","4326"]]')
ON CONFLICT (srid) DO NOTHING;

CREATE TABLE organizations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ulid TEXT NOT NULL UNIQUE,

  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 300),
  legal_name TEXT,
  alternate_name TEXT,
  description TEXT,

  email TEXT,
  telephone TEXT,
  url TEXT,

  street_address TEXT,
  address_locality TEXT,
  address_region TEXT,
  postal_code TEXT,
  address_country TEXT DEFAULT 'CA',

  organization_type TEXT,
  founding_date DATE,

  origin_node_id UUID,

  confidence DECIMAL(3, 2) CHECK (confidence BETWEEN 0 AND 1),

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_organizations_name ON organizations (name);
CREATE INDEX idx_organizations_name_trgm ON organizations USING GIN (name gin_trgm_ops);
CREATE INDEX idx_organizations_locality ON organizations (address_locality);

CREATE TABLE places (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ulid TEXT NOT NULL UNIQUE,

  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 300),
  description TEXT,

  street_address TEXT,
  address_locality TEXT,
  address_region TEXT,
  postal_code TEXT,
  address_country TEXT DEFAULT 'CA',

  full_address TEXT GENERATED ALWAYS AS (
    COALESCE(street_address || ', ', '') ||
    COALESCE(address_locality || ', ', '') ||
    COALESCE(address_region || ' ', '') ||
    COALESCE(postal_code || ', ', '') ||
    COALESCE(address_country, '')
  ) STORED,

  latitude NUMERIC(10, 7),
  longitude NUMERIC(11, 7),
  geo_point GEOMETRY(Point, 4326) GENERATED ALWAYS AS (
    CASE
      WHEN latitude IS NOT NULL AND longitude IS NOT NULL
      THEN ST_SetSRID(ST_MakePoint(longitude, latitude), 4326)
      ELSE NULL
    END
  ) STORED,

  telephone TEXT,
  email TEXT,
  url TEXT,

  maximum_attendee_capacity INTEGER,
  venue_type TEXT,

  accessibility_features TEXT[],

  origin_node_id UUID,

  confidence DECIMAL(3, 2) CHECK (confidence BETWEEN 0 AND 1),

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_places_locality ON places (address_locality);
CREATE INDEX idx_places_region ON places (address_region);
CREATE INDEX idx_places_geo ON places USING GIST (geo_point);
CREATE INDEX idx_places_name_trgm ON places USING GIN (name gin_trgm_ops);
CREATE INDEX idx_places_search ON places USING GIN (
  to_tsvector('english',
    COALESCE(name, '') || ' ' ||
    COALESCE(full_address, '')
  )
);

CREATE TABLE event_series (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

  name TEXT NOT NULL,
  description TEXT,

  series_start_date DATE NOT NULL,
  series_end_date DATE,
  repeat_frequency TEXT,
  repeat_on_days TEXT[],
  repeat_on_dates INTEGER[],
  schedule_timezone TEXT NOT NULL DEFAULT 'America/Toronto',

  default_venue_id UUID REFERENCES places(id),
  default_start_time TIME,
  default_end_time TIME,

  organizer_id UUID REFERENCES organizations(id),

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_series_organizer ON event_series (organizer_id);
CREATE INDEX idx_series_venue ON event_series (default_venue_id);
CREATE INDEX idx_series_date_range ON event_series (series_start_date, series_end_date);

CREATE TABLE events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ulid TEXT NOT NULL UNIQUE,

  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 500),
  description TEXT CHECK (length(description) <= 10000),

  lifecycle_state TEXT NOT NULL DEFAULT 'draft' CHECK (
    lifecycle_state IN (
      'draft',
      'published',
      'postponed',
      'rescheduled',
      'sold_out',
      'cancelled',
      'completed',
      'deleted'
    )
  ),
  event_status TEXT DEFAULT 'https://schema.org/EventScheduled' CHECK (
    event_status IN (
      'https://schema.org/EventScheduled',
      'https://schema.org/EventCancelled',
      'https://schema.org/EventPostponed',
      'https://schema.org/EventRescheduled',
      'https://schema.org/EventMovedOnline'
    )
  ),
  attendance_mode TEXT DEFAULT 'https://schema.org/OfflineEventAttendanceMode' CHECK (
    attendance_mode IN (
      'https://schema.org/OfflineEventAttendanceMode',
      'https://schema.org/OnlineEventAttendanceMode',
      'https://schema.org/MixedEventAttendanceMode'
    )
  ),

  organizer_id UUID REFERENCES organizations(id),
  primary_venue_id UUID REFERENCES places(id),
  series_id UUID REFERENCES event_series(id),

  image_url TEXT,
  public_url TEXT,
  virtual_url TEXT,

  keywords TEXT[],
  in_language TEXT[] DEFAULT ARRAY['en'],
  default_language TEXT DEFAULT 'en',
  is_accessible_for_free BOOLEAN DEFAULT NULL,
  accessibility_features TEXT[],
  event_domain TEXT DEFAULT 'arts' CHECK (
    event_domain IN (
      'arts',
      'music',
      'culture',
      'sports',
      'community',
      'education',
      'general'
    )
  ),

  origin_node_id UUID,
  federation_uri TEXT,

  dedup_hash TEXT GENERATED ALWAYS AS (
    md5(
      lower(trim(name)) ||
      COALESCE(primary_venue_id::text, COALESCE(virtual_url, 'null')) ||
      COALESCE(series_id::text, 'single')
    )
  ) STORED,

  license_url TEXT NOT NULL DEFAULT 'https://creativecommons.org/publicdomain/zero/1.0/',
  license_status TEXT NOT NULL DEFAULT 'cc0' CHECK (
    license_status IN ('cc0', 'cc-by', 'proprietary', 'unknown')
  ),
  takedown_requested BOOLEAN NOT NULL DEFAULT false,
  takedown_requested_at TIMESTAMPTZ,
  takedown_request_notes TEXT,

  confidence DECIMAL(3, 2) CHECK (confidence BETWEEN 0 AND 1),
  quality_score INTEGER CHECK (quality_score BETWEEN 0 AND 100),

  version INTEGER NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,

  merged_into_id UUID REFERENCES events(id),
  deletion_reason TEXT,

  CONSTRAINT event_location_required CHECK (
    primary_venue_id IS NOT NULL OR virtual_url IS NOT NULL
  )
);

CREATE INDEX idx_events_lifecycle ON events (lifecycle_state, updated_at);
CREATE INDEX idx_events_dedup ON events (dedup_hash) WHERE lifecycle_state NOT IN ('deleted');
CREATE INDEX idx_events_organizer ON events (organizer_id);
CREATE INDEX idx_events_venue ON events (primary_venue_id);
CREATE INDEX idx_events_series ON events (series_id);
CREATE INDEX idx_events_origin ON events (origin_node_id, updated_at);
CREATE INDEX idx_events_federated ON events (federation_uri) WHERE federation_uri IS NOT NULL;
CREATE INDEX idx_events_published ON events (published_at) WHERE published_at IS NOT NULL;
CREATE INDEX idx_events_search_vector ON events USING GIN (
  to_tsvector('english',
    COALESCE(name, '') || ' ' ||
    COALESCE(description, '')
  )
);
CREATE INDEX idx_events_name_trgm ON events USING GIN (name gin_trgm_ops);

CREATE TABLE event_occurrences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,

  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  timezone TEXT NOT NULL DEFAULT 'America/Toronto',
  door_time TIMESTAMPTZ,

  local_date DATE GENERATED ALWAYS AS (
    (start_time AT TIME ZONE timezone)::date
  ) STORED,
  local_start_time TIME GENERATED ALWAYS AS (
    (start_time AT TIME ZONE timezone)::time
  ) STORED,
  local_day_of_week INTEGER GENERATED ALWAYS AS (
    EXTRACT(ISODOW FROM (start_time AT TIME ZONE timezone))
  ) STORED,

  venue_id UUID REFERENCES places(id),
  virtual_url TEXT,
  status_override TEXT,
  cancellation_reason TEXT,

  occurrence_index INTEGER,

  ticket_url TEXT,
  price_min DECIMAL(10, 2),
  price_max DECIMAL(10, 2),
  price_currency TEXT DEFAULT 'CAD',
  availability TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT valid_end_time CHECK (end_time IS NULL OR end_time >= start_time),
  CONSTRAINT valid_price_range CHECK (
    (price_min IS NULL AND price_max IS NULL) OR
    (price_min IS NOT NULL AND price_max >= price_min)
  ),
  CONSTRAINT occurrence_location_required CHECK (
    venue_id IS NOT NULL OR virtual_url IS NOT NULL
  )
);

CREATE INDEX idx_occurrences_event ON event_occurrences (event_id, start_time);
CREATE INDEX idx_occurrences_time_range ON event_occurrences (start_time, end_time);
CREATE INDEX idx_occurrences_venue ON event_occurrences (venue_id, start_time);
CREATE INDEX idx_occurrences_local_date ON event_occurrences (local_date, local_start_time);
CREATE INDEX idx_occurrences_day_of_week ON event_occurrences (local_day_of_week, local_start_time);
CREATE INDEX idx_occurrences_calendar ON event_occurrences (
  local_day_of_week,
  local_start_time,
  venue_id
) WHERE local_start_time >= '17:00' AND local_start_time <= '23:00';
