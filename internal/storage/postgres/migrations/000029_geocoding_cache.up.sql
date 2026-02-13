-- Geocoding cache tables for Nominatim API responses
-- UNLOGGED tables for performance (cache can be rebuilt if lost during crash)

-- Forward geocoding cache (query -> coordinates)
CREATE UNLOGGED TABLE IF NOT EXISTS geocoding_cache (
    id BIGSERIAL PRIMARY KEY,
    query_normalized TEXT NOT NULL,
    country_codes TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    display_name TEXT NOT NULL,
    place_type TEXT NOT NULL DEFAULT '',
    osm_id BIGINT,
    raw_response JSONB,
    source TEXT NOT NULL DEFAULT 'nominatim',
    hit_count INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
    UNIQUE(query_normalized, country_codes)
);

CREATE INDEX idx_geocoding_cache_query ON geocoding_cache (query_normalized, country_codes);
CREATE INDEX idx_geocoding_cache_expires ON geocoding_cache (expires_at) WHERE expires_at IS NOT NULL;

-- Reverse geocoding cache (coordinates -> address)
CREATE UNLOGGED TABLE IF NOT EXISTS reverse_geocoding_cache (
    id BIGSERIAL PRIMARY KEY,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    geo_point GEOMETRY(Point, 4326) NOT NULL,
    display_name TEXT NOT NULL,
    address_road TEXT,
    address_suburb TEXT,
    address_city TEXT,
    address_state TEXT,
    address_postcode TEXT,
    address_country TEXT,
    osm_id BIGINT,
    raw_response JSONB,
    hit_count INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days'
);

CREATE INDEX idx_reverse_geocoding_cache_geo ON reverse_geocoding_cache USING GIST (geo_point);
CREATE INDEX idx_reverse_geocoding_cache_expires ON reverse_geocoding_cache (expires_at) WHERE expires_at IS NOT NULL;

-- Failure tracking (avoid hammering API with queries that consistently fail)
CREATE UNLOGGED TABLE IF NOT EXISTS geocoding_failures (
    id BIGSERIAL PRIMARY KEY,
    query_normalized TEXT NOT NULL,
    country_codes TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 1,
    retry_after TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
    UNIQUE(query_normalized, country_codes)
);

CREATE INDEX idx_geocoding_failures_query ON geocoding_failures (query_normalized, country_codes);
CREATE INDEX idx_geocoding_failures_expires ON geocoding_failures (expires_at) WHERE expires_at IS NOT NULL;
