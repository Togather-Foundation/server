-- Migration: knowledge_graph_tables
-- Description: Add tables for multi-graph knowledge graph reconciliation
-- See: docs/interop/KNOWLEDGE_GRAPHS.md

-- Table 1: Registry of knowledge graph authorities
CREATE TABLE knowledge_graph_authorities (
    id SERIAL PRIMARY KEY,
    authority_code TEXT NOT NULL UNIQUE,
    authority_name TEXT NOT NULL,
    base_uri_pattern TEXT NOT NULL,  -- regex for validating URIs from this authority
    reconciliation_endpoint TEXT,     -- W3C Reconciliation API URL (nullable for non-recon authorities)
    applicable_domains TEXT[] NOT NULL DEFAULT '{}',  -- event domains this authority covers
    trust_level INTEGER NOT NULL DEFAULT 5 CHECK (trust_level BETWEEN 1 AND 10),
    priority_order INTEGER NOT NULL DEFAULT 50,  -- lower = higher priority
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 60,
    rate_limit_per_day INTEGER NOT NULL DEFAULT 10000,
    is_active BOOLEAN NOT NULL DEFAULT true,
    documentation_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Table 2: External identifiers (sameAs links) for SEL entities
CREATE TABLE entity_identifiers (
    id SERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,  -- 'event', 'place', 'organization', 'person'
    entity_id TEXT NOT NULL,    -- ULID of the SEL entity
    authority_code TEXT NOT NULL REFERENCES knowledge_graph_authorities(authority_code),
    identifier_uri TEXT NOT NULL,  -- the external URI (e.g., http://kg.artsdata.ca/resource/K11-456)
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0,  -- 0.0000 to 1.0000
    reconciliation_method TEXT NOT NULL DEFAULT 'manual',  -- 'auto_high', 'auto_low', 'manual', 'imported'
    is_canonical BOOLEAN NOT NULL DEFAULT false,  -- whether this is the primary identifier for this authority
    metadata JSONB,  -- extra data from the reconciliation (e.g., sameAs links from Artsdata response)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_type, entity_id, authority_code, identifier_uri)
);

CREATE INDEX idx_entity_identifiers_entity ON entity_identifiers(entity_type, entity_id);
CREATE INDEX idx_entity_identifiers_authority ON entity_identifiers(authority_code);
CREATE INDEX idx_entity_identifiers_uri ON entity_identifiers(identifier_uri);

-- Table 3: Cache for reconciliation API lookups
CREATE TABLE reconciliation_cache (
    id SERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL,
    authority_code TEXT NOT NULL,
    lookup_key TEXT NOT NULL,  -- normalized query string (e.g., "massey hall|toronto|on")
    result_json JSONB,  -- cached API response
    hit_count INTEGER NOT NULL DEFAULT 0,
    is_negative BOOLEAN NOT NULL DEFAULT false,  -- true if the lookup returned no results
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_type, authority_code, lookup_key)
);

CREATE INDEX idx_reconciliation_cache_expires ON reconciliation_cache(expires_at);
CREATE INDEX idx_reconciliation_cache_lookup ON reconciliation_cache(entity_type, authority_code, lookup_key);

-- Seed initial authorities
INSERT INTO knowledge_graph_authorities (authority_code, authority_name, base_uri_pattern, reconciliation_endpoint, applicable_domains, trust_level, priority_order, rate_limit_per_minute, rate_limit_per_day, documentation_url) VALUES
('artsdata', 'Artsdata Knowledge Graph', '^http://kg\.artsdata\.ca/resource/K\d+-\d+$', 'https://api.artsdata.ca/recon', ARRAY['arts', 'culture', 'music'], 9, 10, 60, 10000, 'https://docs.artsdata.ca/'),
('wikidata', 'Wikidata', '^http://www\.wikidata\.org/entity/Q\d+$', NULL, ARRAY['arts', 'culture', 'music', 'sports', 'community', 'education', 'general'], 8, 20, 30, 5000, 'https://www.wikidata.org/'),
('musicbrainz', 'MusicBrainz', '^https://musicbrainz\.org/(artist|event|place)/[0-9a-f-]+$', NULL, ARRAY['music'], 9, 15, 30, 5000, 'https://musicbrainz.org/doc/MusicBrainz_API'),
('isni', 'ISNI', '^https?://isni\.org/isni/\d{16}$', NULL, ARRAY['arts', 'culture', 'music', 'education'], 9, 30, 10, 1000, 'https://isni.org/'),
('osm', 'OpenStreetMap', '^https://www\.openstreetmap\.org/(node|way|relation)/\d+$', NULL, ARRAY['arts', 'culture', 'music', 'sports', 'community', 'education', 'general'], 7, 40, 60, 10000, 'https://wiki.openstreetmap.org/');
