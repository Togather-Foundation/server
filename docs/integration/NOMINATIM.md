# Nominatim Integration Guide

**Version:** 0.1.0
**Date:** 2026-02-13
**Status:** Design (Pre-Implementation)

This document covers the Nominatim client design, usage policies, API reference, and rate limiting strategy for the Togather server.

For the overall geocoding architecture (proximity search, caching, enrichment), see [GEOCODING.md](GEOCODING.md).

---

## Table of Contents

1. [Overview](#overview)
2. [Usage Policy Compliance](#usage-policy-compliance)
3. [API Reference](#api-reference)
4. [Client Design](#client-design)
5. [Rate Limiting Strategy](#rate-limiting-strategy)
6. [Error Handling](#error-handling)
7. [Testing Strategy](#testing-strategy)
8. [OSM Bulk Data via Overpass API](#osm-bulk-data-via-overpass-api)
9. [Future: Self-Hosted Nominatim](#future-self-hosted-nominatim)

---

## Overview

[Nominatim](https://nominatim.org/) is OpenStreetMap's geocoding service. It provides:

- **Forward Geocoding (Search)**: Address/place name -> coordinates
- **Reverse Geocoding**: Coordinates -> address/place details
- **Address Lookup**: OSM ID -> address details

Togather uses the public Nominatim API at `https://nominatim.openstreetmap.org` with plans to self-host if usage grows beyond policy limits.

---

## Usage Policy Compliance

**Source**: https://operations.osmfoundation.org/policies/nominatim/

### Requirements (Mandatory)

| Requirement | Our Implementation |
|-------------|-------------------|
| **User-Agent header** with valid email | `Togather/1.0 (nominatim@togather.foundation)` |
| **Max 1 request/second** | River job queue with single worker + 1s sleep |
| **Cache results** | PostgreSQL unlogged tables (30-day TTL) |
| **No large-scale systematic geocoding** | Background enrichment via rate-limited River jobs |
| **Provide attribution** | OSM attribution in API responses and UI |

### Prohibited Actions

- Bulk geocoding without caching
- Exceeding 1 request/second sustained
- Automated requests without User-Agent identification
- Using Nominatim for tile serving or navigation routing

### Attribution Requirements

All responses that include Nominatim data must include:

```
Data (c) OpenStreetMap contributors, ODbL 1.0. https://osm.org/copyright
```

This is included in:
- API response headers: `X-Geocoding-Attribution`
- JSON responses: `geocoding.attribution` field
- Frontend UI: Footer attribution text

### When to Self-Host

Self-host Nominatim if:
- Sustained load exceeds 1 request/second
- OSM Foundation contacts us about usage
- We need guaranteed uptime/latency SLAs
- We want to customize import data (e.g., Ontario-only)

---

## API Reference

### Search (Forward Geocoding)

**Endpoint:** `GET /search`

```
GET https://nominatim.openstreetmap.org/search
    ?q=Toronto+City+Hall
    &format=jsonv2
    &addressdetails=1
    &limit=1
    &countrycodes=ca
```

**Parameters we use:**

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `q` | search text | Free-form query string |
| `format` | `jsonv2` | JSON output with stable fields |
| `addressdetails` | `1` | Include address breakdown |
| `limit` | `1` | We only need the best match |
| `countrycodes` | `ca` | Restrict to Canada (configurable) |
| `email` | `nominatim@togather.foundation` | Usage policy compliance |

**Structured query alternative** (more precise when address is already parsed):

```
GET /search?street=100+Queen+St+W&city=Toronto&state=Ontario&country=Canada
    &format=jsonv2&addressdetails=1&limit=1
```

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `street` | street address | House number + street name |
| `city` | city name | City/municipality |
| `state` | province/state | Province or state |
| `country` | country name | Country |
| `postalcode` | postal code | Postal/zip code |

**Response (jsonv2):**

```json
[
  {
    "place_id": 298375103,
    "licence": "Data (c) OpenStreetMap contributors, ODbL 1.0. https://osm.org/copyright",
    "osm_type": "relation",
    "osm_id": 3575816,
    "lat": "43.6534817",
    "lon": "-79.3839347",
    "category": "amenity",
    "type": "townhall",
    "place_rank": 30,
    "importance": 0.5201556591925,
    "addresstype": "amenity",
    "name": "Toronto City Hall",
    "display_name": "Toronto City Hall, 100, Queen Street West, Garden District, Old Toronto, Toronto, Golden Horseshoe, Ontario, M5H 2N2, Canada",
    "address": {
      "amenity": "Toronto City Hall",
      "house_number": "100",
      "road": "Queen Street West",
      "suburb": "Garden District",
      "city": "Toronto",
      "state": "Ontario",
      "postcode": "M5H 2N2",
      "country": "Canada",
      "country_code": "ca"
    },
    "boundingbox": ["43.6527", "43.6541", "-79.3852", "-79.3828"]
  }
]
```

### Reverse Geocoding

**Endpoint:** `GET /reverse`

```
GET https://nominatim.openstreetmap.org/reverse
    ?lat=43.6532
    &lon=-79.3832
    &format=jsonv2
    &addressdetails=1
    &zoom=18
```

**Parameters we use:**

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `lat` | latitude | WGS84 latitude |
| `lon` | longitude | WGS84 longitude |
| `format` | `jsonv2` | JSON output |
| `addressdetails` | `1` | Include address breakdown |
| `zoom` | `18` | Building-level detail |
| `email` | `nominatim@togather.foundation` | Usage policy compliance |

**Response (jsonv2):**

```json
{
  "place_id": 298375103,
  "licence": "Data (c) OpenStreetMap contributors, ODbL 1.0. https://osm.org/copyright",
  "osm_type": "relation",
  "osm_id": 3575816,
  "lat": "43.6534817",
  "lon": "-79.3839347",
  "category": "amenity",
  "type": "townhall",
  "place_rank": 30,
  "importance": 0.5201556591925,
  "addresstype": "amenity",
  "name": "Toronto City Hall",
  "display_name": "Toronto City Hall, 100, Queen Street West, ...",
  "address": {
    "amenity": "Toronto City Hall",
    "house_number": "100",
    "road": "Queen Street West",
    "suburb": "Garden District",
    "city": "Toronto",
    "state": "Ontario",
    "postcode": "M5H 2N2",
    "country": "Canada",
    "country_code": "ca"
  }
}
```

---

## Client Design

### Package Structure

```
internal/geocoding/
    geocoding.go            -- GeocodingService (orchestrates cache + client)
    types.go                -- Shared types (Result, Address, etc.)
    nominatim/
        client.go           -- NominatimClient (HTTP client)
        client_test.go      -- Tests with mocked HTTP
        types.go            -- Nominatim API response types
```

### Interface

```go
// internal/geocoding/geocoding.go

// Geocoder is the interface for geocoding operations.
// Implementations: NominatimClient, MockGeocoder (tests)
type Geocoder interface {
    // Search converts a text query or address to coordinates.
    Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error)

    // Reverse converts coordinates to an address.
    Reverse(ctx context.Context, lat, lon float64) (*ReverseResult, error)
}

type SearchOptions struct {
    CountryCodes []string  // ISO 3166-1 alpha-2 codes (default: ["ca"])
    Limit        int       // Max results (default: 1)
    Structured   *StructuredQuery  // nil for free-form search
}

type StructuredQuery struct {
    Street     string
    City       string
    State      string
    Country    string
    PostalCode string
}

type SearchResult struct {
    Latitude    float64
    Longitude   float64
    DisplayName string
    Address     Address
    OSMType     string
    OSMID       int64
    Importance  float64
    Attribution string
}

type ReverseResult struct {
    Latitude    float64
    Longitude   float64
    DisplayName string
    Address     Address
    Attribution string
}

type Address struct {
    HouseNumber string `json:"house_number,omitempty"`
    Road        string `json:"road,omitempty"`
    Suburb      string `json:"suburb,omitempty"`
    City        string `json:"city,omitempty"`
    State       string `json:"state,omitempty"`
    PostalCode  string `json:"postcode,omitempty"`
    Country     string `json:"country,omitempty"`
    CountryCode string `json:"country_code,omitempty"`
}
```

### HTTP Client Configuration

```go
// internal/geocoding/nominatim/client.go

type Client struct {
    httpClient *http.Client
    baseURL    string
    userAgent  string
    email      string
}

func NewClient(cfg Config) *Client {
    return &Client{
        httpClient: &http.Client{
            Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        10,
                MaxIdleConnsPerHost: 2,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        baseURL:   cfg.APIURL,       // https://nominatim.openstreetmap.org
        userAgent: fmt.Sprintf("Togather/%s (%s)", version, cfg.UserEmail),
        email:     cfg.UserEmail,
    }
}
```

### Request Headers

Every request to Nominatim includes:

```
User-Agent: Togather/1.0 (nominatim@togather.foundation)
Accept: application/json
Accept-Language: en
```

---

## Rate Limiting Strategy

### Layer 1: River Job Queue (Background Enrichment)

Background geocoding jobs run in a dedicated `geocoding` queue with a single worker:

```go
// Ensures max 1 concurrent geocoding operation
Queues: map[string]river.QueueConfig{
    "geocoding": {MaxWorkers: 1},
}
```

Each worker sleeps 1 second after each API call:

```go
func (w *GeocodePlaceWorker) Work(ctx context.Context, job *river.Job[GeocodePlaceArgs]) error {
    defer time.Sleep(1 * time.Second) // Rate limit: 1 req/sec
    // ... geocoding logic
}
```

### Layer 2: In-Process Rate Limiter (Search-Time Geocoding)

Search-time geocoding uses a Go rate limiter for real-time requests:

```go
import "golang.org/x/time/rate"

type RateLimitedGeocoder struct {
    geocoder Geocoder
    limiter  *rate.Limiter
}

func NewRateLimitedGeocoder(g Geocoder, rps float64) *RateLimitedGeocoder {
    return &RateLimitedGeocoder{
        geocoder: g,
        limiter:  rate.NewLimiter(rate.Limit(rps), 1), // 1 req/sec, burst of 1
    }
}

func (r *RateLimitedGeocoder) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
    if err := r.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limit wait: %w", err)
    }
    return r.geocoder.Search(ctx, query, opts)
}
```

### Layer 3: Cache (Eliminates Most API Calls)

The PostgreSQL cache eliminates the vast majority of API calls:
- Forward cache: 30-day TTL, top 10k preserved indefinitely
- Negative cache: 7-day TTL (prevents retry spam)
- Expected cache hit rate: 80-95% in steady state

---

## Error Handling

### Nominatim Error Responses

| HTTP Status | Meaning | Our Action |
|-------------|---------|------------|
| 200, empty results | No match found | Cache as negative result (7 days) |
| 200, results | Success | Cache result (30 days) |
| 400 | Bad request | Log error, don't retry (bad query) |
| 403 | Blocked | Log critical, alert admin, back off |
| 429 | Rate limited | Exponential backoff (2s, 4s, 8s) |
| 500 | Server error | Retry with backoff (max 2 retries) |
| 503 | Service unavailable | Retry with backoff (max 2 retries) |
| Timeout | Request timeout | Retry once, then cache as failure |

### Retry Policy

```go
type RetryConfig struct {
    MaxRetries    int           // 2
    InitialDelay  time.Duration // 2 seconds
    MaxDelay      time.Duration // 30 seconds
    BackoffFactor float64       // 2.0
}
```

### Graceful Degradation

Geocoding failures NEVER block core operations:

- **Search-time geocoding fails**: Return 422 with `"geocoding_failed"` error, suggest using `near_lat`/`near_lon` instead
- **Background enrichment fails**: Job retries up to 3 times, then marks as failed. Place/event remains in database without coordinates
- **Cache unavailable**: Skip cache, call Nominatim directly (with rate limiting)

---

## Testing Strategy

### Unit Tests (Always Run)

Mock the Nominatim HTTP client:

```go
// internal/geocoding/nominatim/client_test.go

func TestSearch_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify User-Agent header
        assert.Contains(t, r.Header.Get("User-Agent"), "Togather")
        // Verify email parameter
        assert.Equal(t, "nominatim@togather.foundation", r.URL.Query().Get("email"))
        
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode([]NominatimSearchResult{
            {
                Lat:         "43.6534817",
                Lon:         "-79.3839347",
                DisplayName: "Toronto City Hall, ...",
            },
        })
    }))
    defer server.Close()

    client := NewClient(Config{
        APIURL:    server.URL,
        UserEmail: "nominatim@togather.foundation",
    })
    
    result, err := client.Search(context.Background(), "Toronto City Hall", SearchOptions{})
    require.NoError(t, err)
    assert.InDelta(t, 43.653, result.Latitude, 0.001)
}
```

### Integration Tests (Opt-In, Never in CI)

Run against live Nominatim API only when explicitly enabled:

```go
func TestNominatimLive_Search(t *testing.T) {
    if os.Getenv("GEOCODING_LIVE_TESTS") != "true" {
        t.Skip("Skipping live Nominatim test (set GEOCODING_LIVE_TESTS=true)")
    }
    // ... test against real API
}
```

### Cache Tests

Test cache behavior with a real PostgreSQL database (integration test):

```go
func TestGeocodingCache_HitAndMiss(t *testing.T) {
    // Uses test database with unlogged tables
    repo := NewGeocodingCacheRepo(testPool)
    
    // Miss
    _, ok := repo.GetCachedGeocode(ctx, "toronto city hall")
    assert.False(t, ok)
    
    // Store
    repo.CacheGeocode(ctx, "toronto city hall", result, 30*24*time.Hour)
    
    // Hit
    cached, ok := repo.GetCachedGeocode(ctx, "toronto city hall")
    assert.True(t, ok)
    assert.Equal(t, result.Latitude, cached.Latitude)
}
```

---

## OSM Bulk Data via Overpass API

For pre-seeding the geocoding cache with known Toronto venues, we use the [Overpass API](https://wiki.openstreetmap.org/wiki/Overpass_API) instead of downloading full PBF extracts.

### Why Overpass (Not Geofabrik Extracts)

| Criteria | Overpass API | Geofabrik Extracts |
|----------|-------------|-------------------|
| **Complexity** | HTTP query, JSON response | PBF download + osm2pgsql |
| **Data size** | ~5MB (targeted venues) | ~600MB (all of Ontario) |
| **Setup time** | None (API call) | osm2pgsql install + import |
| **Precision** | Exact query for venue types | Must filter after import |
| **Best for** | Targeted venue extraction | Running own Nominatim |

### Overpass Query: Toronto Cultural Venues

```overpass
[out:json][timeout:120];
// Greater Toronto Area bounding box
// South: 43.5810, West: -79.6390, North: 43.8554, East: -79.1168
(
  // Performing arts and culture
  nwr["amenity"~"theatre|arts_centre|community_centre|cinema|library"]
      (43.5810,-79.6390,43.8554,-79.1168);
  // Tourism and attractions
  nwr["tourism"~"museum|attraction|gallery|theme_park|viewpoint"]
      (43.5810,-79.6390,43.8554,-79.1168);
  // Recreation and sports
  nwr["leisure"~"park|stadium|sports_centre|fitness_centre|swimming_pool"]
      (43.5810,-79.6390,43.8554,-79.1168);
  // Music venues
  nwr["amenity"="nightclub"](43.5810,-79.6390,43.8554,-79.1168);
  nwr["amenity"="pub"]["live_music"="yes"](43.5810,-79.6390,43.8554,-79.1168);
  // Conference and event spaces
  nwr["amenity"~"conference_centre|exhibition_centre"]
      (43.5810,-79.6390,43.8554,-79.1168);
  // Religious and historic buildings (often host events)
  nwr["amenity"="place_of_worship"]["heritage"](43.5810,-79.6390,43.8554,-79.1168);
  nwr["historic"~"building|monument|memorial"](43.5810,-79.6390,43.8554,-79.1168);
);
out center;
```

**Notes:**
- `nwr` = nodes, ways, relations
- `out center` = for ways/relations, output the centroid coordinates
- Timeout set to 120s for large queries
- Expected result: ~5,000-15,000 venue entries for GTA

### Overpass Query: Extended GTA Regions

For broader coverage, repeat with bounding boxes for:
- **Mississauga**: `(43.5190,-79.7430,43.6500,-79.5400)`
- **Brampton**: `(43.6500,-79.8200,43.7700,-79.6500)`
- **Markham**: `(43.8000,-79.4000,43.9200,-79.2000)`
- **Vaughan**: `(43.7500,-79.5800,43.9000,-79.4000)`

### CLI Command Design

```
server geocode-preseed --region=toronto    # GTA bounding box
server geocode-preseed --region=ontario    # Multiple Ontario cities
server geocode-preseed --bbox=43.58,-79.64,43.86,-79.12  # Custom bbox
server geocode-preseed --dry-run           # Show query without executing
```

### Overpass API Usage Policy

From https://wiki.openstreetmap.org/wiki/Overpass_API#Usage_policy:

- Max 2 concurrent requests
- Max 10,000 requests/day
- For bulk downloads, use Geofabrik extracts
- Our use case (one-time import of ~10k venues) is within policy

---

## Future: Self-Hosted Nominatim

### When to Self-Host

| Trigger | Threshold |
|---------|-----------|
| Sustained API load | > 1 req/sec for > 1 hour/day |
| OSM Foundation contact | Any communication about usage |
| Latency requirements | Need < 100ms geocoding |
| Uptime requirements | Need 99.9%+ geocoding availability |

### Self-Hosted Setup (Docker)

```yaml
# deploy/docker/docker-compose.nominatim.yml
services:
  nominatim:
    image: mediagis/nominatim:4.4
    container_name: togather-nominatim
    environment:
      PBF_URL: https://download.geofabrik.de/north-america/canada/ontario-latest.osm.pbf
      REPLICATION_URL: https://download.geofabrik.de/north-america/canada/ontario-updates/
      NOMINATIM_PASSWORD: ${NOMINATIM_PASSWORD:-nominatim}
      IMPORT_STYLE: full
    volumes:
      - nominatim-data:/var/lib/postgresql/14/main
    ports:
      - "8088:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/status"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 600s  # Import takes ~10 minutes for Ontario

volumes:
  nominatim-data:
    name: togather-nominatim-data
```

**Resource requirements (Ontario only):**
- Disk: ~20GB (PBF import + indexes)
- RAM: 4GB minimum, 8GB recommended
- CPU: 2 cores minimum
- Initial import: ~10-30 minutes
- Daily updates: ~5 minutes

### Migration Path

When switching to self-hosted:
1. Deploy Nominatim container with Ontario data
2. Update `NOMINATIM_API_URL` to `http://togather-nominatim:8080`
3. Remove rate limiting (self-hosted can handle unlimited requests)
4. Keep cache tables (still useful for performance)
5. Remove User-Agent email requirement (own instance)

---

## References

- [Nominatim API Documentation](https://nominatim.org/release-docs/latest/api/Overview/)
- [Nominatim Search API](https://nominatim.org/release-docs/latest/api/Search/)
- [Nominatim Reverse API](https://nominatim.org/release-docs/latest/api/Reverse/)
- [Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)
- [Overpass API](https://wiki.openstreetmap.org/wiki/Overpass_API)
- [Overpass QL Reference](https://wiki.openstreetmap.org/wiki/Overpass_API/Overpass_QL)
- [Geofabrik Downloads](https://download.geofabrik.de/north-america/canada/)
- [OSM Planet Data](https://planet.openstreetmap.org/)
- Internal: [GEOCODING.md](GEOCODING.md) (overall geocoding architecture)
