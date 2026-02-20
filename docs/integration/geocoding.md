# Geocoding Architecture

**Version:** 0.1.0
**Date:** 2026-02-13
**Status:** Design (Pre-Implementation)

This document describes the geocoding architecture for the Togather server, covering proximity search, forward/reverse geocoding via Nominatim, caching strategy, and background enrichment.

For the Nominatim client design and usage policies, see [nominatim.md](nominatim.md).

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Proximity Search](#proximity-search)
4. [Search-Time Geocoding](#search-time-geocoding)
5. [Background Enrichment](#background-enrichment)
6. [Caching Strategy](#caching-strategy)
7. [Reverse Geocoding](#reverse-geocoding)
8. [API Endpoints](#api-endpoints)
9. [Configuration](#configuration)
10. [Implementation Phases](#implementation-phases)

---

## Overview

The geocoding system enables three core capabilities:

1. **Proximity Search**: Find places/events within a radius of coordinates (`near_lat`, `near_lon`, `radius`)
2. **Search-Time Geocoding**: Convert place names to coordinates for proximity search (`near_place=Toronto City Hall`)
3. **Background Enrichment**: Automatically geocode events/places missing coordinates on creation

All geocoding uses [Nominatim](https://nominatim.org/), OpenStreetMap's geocoding service, with aggressive caching in PostgreSQL unlogged tables to minimize external API calls and respect OSM usage policies.

### Design Principles

- **Good Neighbor**: Respect Nominatim usage policies (1 req/sec max, User-Agent identification, caching)
- **Cache First**: PostgreSQL unlogged tables for geocoding cache (no Redis dependency)
- **Graceful Degradation**: Geocoding failures never block core operations
- **Simplicity**: Use existing infrastructure (PostgreSQL, River jobs) rather than adding services

---

## Architecture

```
                         User / MCP Agent
                              |
                   ┌──────────┴──────────┐
                   │                     │
             near_lat/lon          near_place=...
                   │                     │
                   │              ┌──────┴──────┐
                   │              │ Geocoding   │
                   │              │ Service     │
                   │              └──────┬──────┘
                   │                     │
                   │         ┌───────────┴───────────┐
                   │         │                       │
                   │    Cache Hit              Cache Miss
                   │    (PG unlogged)          │
                   │         │           ┌─────┴─────┐
                   │         │           │ Nominatim │
                   │         │           │ Client    │
                   │         │           └─────┬─────┘
                   │         │                 │
                   │         │           Store in cache
                   │         │                 │
                   │         └────────┬────────┘
                   │                  │
                   │            lat/lon resolved
                   │                  │
                   └────────┬─────────┘
                            │
                   ┌────────┴────────┐
                   │ PostGIS         │
                   │ ST_DWithin()    │
                   │ Proximity Query │
                   └────────┬────────┘
                            │
                     Matching Places
```

### Component Overview

| Component | Package | Purpose |
|-----------|---------|---------|
| **places.Filters** | `internal/domain/places` | Domain model with proximity fields |
| **PlaceRepository.List()** | `internal/storage/postgres` | PostGIS `ST_DWithin` queries |
| **NominatimClient** | `internal/geocoding/nominatim` | HTTP client for Nominatim API |
| **GeocodingCacheRepo** | `internal/storage/postgres` | PostgreSQL unlogged table cache |
| **GeocodingService** | `internal/geocoding` | Orchestrates cache + Nominatim |
| **GeocodePlaceWorker** | `internal/jobs` | River worker for background geocoding |
| **Places Handler** | `internal/api/handlers` | HTTP handler with `near_place` param |
| **Geocoding Handler** | `internal/api/handlers` | Public `/api/v1/geocode` endpoint |

---

## Proximity Search

### Query Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `near_lat` | float64 | Latitude (WGS84) | `43.6532` |
| `near_lon` | float64 | Longitude (WGS84) | `-79.3832` |
| `radius` | float64 | Search radius in km (default: 10, max: 100) | `5` |
| `near_place` | string | Place name to geocode (alternative to lat/lon) | `Toronto City Hall` |

**Rules:**
- `near_lat` and `near_lon` must both be provided, or neither
- `near_place` cannot be combined with `near_lat`/`near_lon`
- `radius` requires either `near_lat`/`near_lon` or `near_place`
- Default radius: 10 km
- Maximum radius: 100 km

### Domain Model

```go
// internal/domain/places/repository.go
type Filters struct {
    City      string
    Query     string
    Latitude  *float64  // near_lat
    Longitude *float64  // near_lon
    RadiusKm  *float64  // radius in km (default 10, max 100)
}
```

### PostGIS Query

The repository uses `ST_DWithin` on the `geography` type for accurate distance calculations:

```sql
SELECT p.*, 
       ST_Distance(
           p.geo_point::geography, 
           ST_SetSRID(ST_MakePoint($lon, $lat), 4326)::geography
       ) / 1000.0 AS distance_km
FROM places p
WHERE p.geo_point IS NOT NULL
  AND ST_DWithin(
      p.geo_point::geography,
      ST_SetSRID(ST_MakePoint($lon, $lat), 4326)::geography,
      $radius_meters  -- ST_DWithin uses meters for geography type
  )
ORDER BY distance_km ASC
LIMIT $limit
```

**Notes:**
- `ST_DWithin` on `geography` type uses meters (not degrees), so multiply km by 1000
- Uses existing GIST index on `geo_point` column (`idx_places_geo`)
- When proximity filters are active, results sort by distance (not `created_at`)
- Places without `geo_point` (NULL coordinates) are excluded from proximity results

### Validation

```go
func validateProximityFilters(f *Filters) error {
    if f.Latitude != nil || f.Longitude != nil {
        if f.Latitude == nil || f.Longitude == nil {
            return errors.New("near_lat and near_lon must both be provided")
        }
        if *f.Latitude < -90 || *f.Latitude > 90 {
            return errors.New("near_lat must be between -90 and 90")
        }
        if *f.Longitude < -180 || *f.Longitude > 180 {
            return errors.New("near_lon must be between -180 and 180")
        }
    }
    if f.RadiusKm != nil {
        if *f.RadiusKm <= 0 {
            return errors.New("radius must be positive")
        }
        if *f.RadiusKm > 100 {
            return errors.New("radius must not exceed 100 km")
        }
    }
    return nil
}
```

---

## Search-Time Geocoding

When a user provides `near_place` instead of coordinates, the system geocodes the place name in real-time:

```
GET /api/v1/places?near_place=Toronto+City+Hall&radius=5
```

### Flow

1. **Normalize** the query string (lowercase, trim whitespace, collapse spaces)
2. **Check cache** in `geocoding_cache` unlogged table
3. **If cache miss**, call Nominatim Search API
4. **Cache result** (or cache failure for 7 days)
5. **Convert** to `near_lat`/`near_lon` and proceed with proximity search
6. **Include** resolved location in response metadata

### Response Metadata

When `near_place` is used, the response includes geocoding metadata:

```json
{
  "items": [...],
  "next_cursor": "...",
  "geocoding": {
    "query": "Toronto City Hall",
    "resolved_lat": 43.6532,
    "resolved_lon": -79.3832,
    "display_name": "Toronto City Hall, 100 Queen Street West, Toronto, ON",
    "source": "cache"
  }
}
```

### Public Geocoding Endpoint

A standalone geocoding endpoint is available for frontend use:

```
GET /api/v1/geocode?q=Toronto+City+Hall
```

**Response:**

```json
{
  "latitude": 43.6532,
  "longitude": -79.3832,
  "display_name": "Toronto City Hall, 100 Queen Street West, Toronto, ON, Canada",
  "source": "nominatim",
  "cached": false
}
```

- Public endpoint (no authentication required)
- Rate limited: 60 requests/minute (public tier)
- Supports `countrycodes` parameter for restricting results (default: `ca`)

---

## Background Enrichment

When events or places are created with an address but missing latitude/longitude, a River background job geocodes them asynchronously.

### Trigger Points

- **Event creation**: If `location.latitude`/`location.longitude` missing but address fields present
- **Place creation**: If `latitude`/`longitude` missing but `street_address`/`city` present
- **Admin backfill**: Manual endpoint to enqueue geocoding for all records missing coordinates

### River Job Design

```go
// internal/jobs/geocode_place.go
type GeocodePlaceArgs struct {
    PlaceID string `json:"place_id"`
}

func (GeocodePlaceArgs) Kind() string { return "geocode_place" }

// Worker configuration:
// - Max attempts: 3
// - Backoff: Exponential (1min, 5min, 30min)
// - Queue: "geocoding" (dedicated queue for rate control)
// - Rate: Max 1 job/sec (respects Nominatim policy)
```

### Rate Control Strategy

River jobs use a **dedicated `geocoding` queue** with a single worker to ensure max 1 request/second to Nominatim:

```go
riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues: map[string]river.QueueConfig{
        "default":   {MaxWorkers: 10},
        "geocoding": {MaxWorkers: 1},  // Single worker = natural rate limit
    },
})
```

Each geocoding worker adds a `time.Sleep(1 * time.Second)` after each API call as a safety net.

### Address Building

The worker constructs the geocoding query from available address fields:

```go
func buildGeocodingQuery(p places.Place) string {
    parts := []string{}
    if p.StreetAddress != "" { parts = append(parts, p.StreetAddress) }
    if p.City != ""          { parts = append(parts, p.City) }
    if p.Region != ""        { parts = append(parts, p.Region) }
    if p.PostalCode != ""    { parts = append(parts, p.PostalCode) }
    if p.Country != ""       { parts = append(parts, p.Country) }
    return strings.Join(parts, ", ")
}
```

---

## Caching Strategy

### Why PostgreSQL Unlogged Tables

Instead of Redis, we use PostgreSQL unlogged tables for geocoding cache:

1. **Simplicity**: No additional service to manage
2. **Performance**: Unlogged tables skip WAL writes, comparable to Redis for read-heavy workloads
3. **Transactions**: Cache updates can participate in database transactions
4. **Querying**: SQL for cache analytics, debugging, and maintenance
5. **PostGIS**: Spatial indexes for reverse geocoding cache (geo-hash bucketing)
6. **Acceptable trade-off**: Cache loss on crash is fine (just a performance hit, not data loss)

### Schema

```sql
-- Forward geocoding cache (address/place name -> coordinates)
CREATE UNLOGGED TABLE geocoding_cache (
    id          BIGSERIAL PRIMARY KEY,
    query_normalized TEXT NOT NULL,
    latitude    NUMERIC(10,7) NOT NULL,
    longitude   NUMERIC(11,7) NOT NULL,
    display_name TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'nominatim',  -- 'nominatim', 'overpass_import', 'manual'
    cached_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
    hit_count   INTEGER NOT NULL DEFAULT 0,
    UNIQUE(query_normalized)
);
CREATE INDEX idx_geocoding_cache_query ON geocoding_cache(query_normalized);
CREATE INDEX idx_geocoding_cache_expires ON geocoding_cache(expires_at);

-- Reverse geocoding cache (coordinates -> address)
CREATE UNLOGGED TABLE reverse_geocoding_cache (
    id          BIGSERIAL PRIMARY KEY,
    geo_point   GEOMETRY(Point, 4326) NOT NULL,
    address     JSONB NOT NULL,
    display_name TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'nominatim',
    cached_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
    hit_count   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_reverse_geocoding_cache_geo ON reverse_geocoding_cache USING GIST(geo_point);
CREATE INDEX idx_reverse_geocoding_cache_expires ON reverse_geocoding_cache(expires_at);

-- Negative cache (failed geocoding attempts)
CREATE UNLOGGED TABLE geocoding_failures (
    id              BIGSERIAL PRIMARY KEY,
    query_normalized TEXT NOT NULL,
    error_message   TEXT,
    failed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retry_after     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
    attempt_count   INTEGER NOT NULL DEFAULT 1,
    UNIQUE(query_normalized)
);
CREATE INDEX idx_geocoding_failures_retry ON geocoding_failures(retry_after);
```

### Cache Lookup Flow

```
1. Check geocoding_cache WHERE query_normalized = $normalized
   └─ Hit? → Increment hit_count, return result
   └─ Miss? → Continue

2. Check geocoding_failures WHERE query_normalized = $normalized AND retry_after > NOW()
   └─ Recent failure? → Return ErrGeocodeFailedRecently
   └─ No failure (or expired)? → Continue

3. Call Nominatim API
   └─ Success? → INSERT INTO geocoding_cache, return result
   └─ Failure? → UPSERT INTO geocoding_failures, return error
```

### Cache Maintenance

A daily River job cleans expired entries while preserving popular queries:

```sql
-- Delete expired entries (but preserve top 10k most-hit)
WITH popular AS (
    SELECT id FROM geocoding_cache 
    ORDER BY hit_count DESC LIMIT 10000
)
DELETE FROM geocoding_cache 
WHERE expires_at < NOW() 
  AND id NOT IN (SELECT id FROM popular);

-- Clean expired failures
DELETE FROM geocoding_failures WHERE retry_after < NOW();

-- Clean expired reverse geocoding entries
WITH popular_reverse AS (
    SELECT id FROM reverse_geocoding_cache 
    ORDER BY hit_count DESC LIMIT 10000
)
DELETE FROM reverse_geocoding_cache 
WHERE expires_at < NOW() 
  AND id NOT IN (SELECT id FROM popular_reverse);
```

### Query Normalization

Consistent normalization ensures cache hits for equivalent queries:

```go
func normalizeQuery(q string) string {
    q = strings.TrimSpace(q)
    q = strings.ToLower(q)
    q = regexp.MustCompile(`\s+`).ReplaceAllString(q, " ")
    // Remove common noise words
    q = strings.ReplaceAll(q, ", canada", "")
    q = strings.ReplaceAll(q, ", ca", "")
    return q
}
```

---

## Reverse Geocoding

Converts coordinates to human-readable addresses. Used for map UI interactions.

```
GET /api/v1/reverse-geocode?lat=43.6532&lon=-79.3832
```

### Geo-Hash Bucketing

Reverse geocoding results are cached with ~100m precision (5 decimal places) to improve cache hit rates for nearby coordinates:

```go
func bucketCoordinate(lat, lon float64) (float64, float64) {
    // Round to 5 decimal places (~1.1m precision at equator)
    // Nearby clicks (within ~100m) share cache entry
    precision := 100000.0
    return math.Round(lat*precision) / precision, 
           math.Round(lon*precision) / precision
}
```

Cache lookup uses `ST_DWithin` with a 100m radius:

```sql
SELECT * FROM reverse_geocoding_cache
WHERE ST_DWithin(geo_point, ST_SetSRID(ST_MakePoint($lon, $lat), 4326)::geography, 100)
ORDER BY ST_Distance(geo_point, ST_SetSRID(ST_MakePoint($lon, $lat), 4326)::geography)
LIMIT 1
```

---

## API Endpoints

### Places API (Enhanced)

```
GET /api/v1/places?near_lat=43.6532&near_lon=-79.3832&radius=5
GET /api/v1/places?near_place=Toronto+City+Hall&radius=5
GET /api/v1/places?city=Toronto&near_place=Kensington+Market&radius=2
```

### Geocoding API (New)

```
GET /api/v1/geocode?q=Toronto+City+Hall
GET /api/v1/geocode?q=100+Queen+St+W,+Toronto&countrycodes=ca
```

### Reverse Geocoding API (New)

```
GET /api/v1/reverse-geocode?lat=43.6532&lon=-79.3832
```

### Admin Endpoints (New)

```
POST /api/v1/admin/geocoding/backfill    -- Enqueue geocoding for all records missing coordinates
GET  /api/v1/admin/geocoding/stats       -- Cache statistics (hit rates, top queries, failure count)
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NOMINATIM_API_URL` | `https://nominatim.openstreetmap.org` | Nominatim API base URL |
| `NOMINATIM_USER_EMAIL` | *(required)* | Email for User-Agent (OSM policy) |
| `NOMINATIM_RATE_LIMIT_PER_SEC` | `1` | Max requests per second |
| `NOMINATIM_TIMEOUT_SECONDS` | `5` | HTTP request timeout |
| `GEOCODING_CACHE_TTL_DAYS` | `30` | Forward/reverse cache TTL |
| `GEOCODING_FAILURE_TTL_DAYS` | `7` | Negative cache TTL |
| `GEOCODING_POPULAR_PRESERVE_COUNT` | `10000` | Top-N queries to preserve past TTL |
| `GEOCODING_DEFAULT_COUNTRY` | `ca` | Default country code for queries |
| `GEOCODING_MAX_RADIUS_KM` | `100` | Maximum proximity search radius |
| `GEOCODING_DEFAULT_RADIUS_KM` | `10` | Default proximity search radius |

---

## Observability

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `geocoding_requests_total` | Counter | Total geocoding requests (labels: `type=forward\|reverse`, `source=cache\|nominatim`) |
| `geocoding_cache_hits_total` | Counter | Cache hits (labels: `type=forward\|reverse`) |
| `geocoding_cache_misses_total` | Counter | Cache misses (labels: `type=forward\|reverse`) |
| `geocoding_nominatim_requests_total` | Counter | Nominatim API calls (labels: `status=success\|error`) |
| `geocoding_nominatim_latency_seconds` | Histogram | Nominatim API response time |
| `geocoding_failures_total` | Counter | Failed geocoding attempts |
| `geocoding_cache_size` | Gauge | Current cache entry count |
| `proximity_search_total` | Counter | Proximity search requests |

### Structured Logging

All geocoding operations log with zerolog:

```go
log.Info().
    Str("query", query).
    Float64("lat", result.Latitude).
    Float64("lon", result.Longitude).
    Str("source", "nominatim").
    Dur("latency", elapsed).
    Msg("geocoding completed")
```

---

## Implementation Phases

### Phase 1: Core Proximity + Geocoding (Branch: `feature/proximity-geocoding`)

| Bead | Title | Priority | Estimate |
|------|-------|----------|----------|
| srv-hsnt3 | Core proximity search (near_lat, near_lon, radius) | P2 | 4.5h |
| srv-hsnt4 | Nominatim client + PostgreSQL cache infrastructure | P2 | 6.5h |
| srv-hsnt5 | Search-time geocoding with `near_place` parameter | P2 | 4h |

**Dependency chain:** srv-hsnt5 depends on srv-hsnt3 and srv-hsnt4.

### Phase 2: Background Enrichment (After Phase 1 merged)

| Bead | Title | Priority | Estimate |
|------|-------|----------|----------|
| srv-hsnt6 | Background geocoding enrichment via River jobs | P3 | 5h |
| srv-hsnt7 | Reverse geocoding for map UI | P4 | 1.5h |

### Phase 3: Optimization (As needed)

| Bead | Title | Priority | Estimate |
|------|-------|----------|----------|
| srv-hsnt8 | OSM venue pre-seed via Overpass API | P4 | 2h |

---

## References

- [Nominatim API Docs](https://nominatim.org/release-docs/latest/api/Overview/)
- [Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)
- [PostGIS ST_DWithin](https://postgis.net/docs/ST_DWithin.html)
- [Overpass API](https://wiki.openstreetmap.org/wiki/Overpass_API)
- [River Job Queue](https://riverqueue.com/docs)
- Internal: [nominatim.md](nominatim.md) (client design and usage policy details)
- Internal: [scrapers.md](scrapers.md) (scraper geocoding patterns)

---

**Last Updated:** 2026-02-20
