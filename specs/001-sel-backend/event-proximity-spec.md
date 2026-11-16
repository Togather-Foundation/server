# Specification: Event proximity filtering

**Date:** 2026-11-16 · **Status:** In Progress · **Ticket:** `t_dbb163bc`
**Parent:** [SEL backend specification](spec.md)

## Goal and context

Return events within a caller-supplied geographic radius through REST and MCP,
ordered by their earliest matching occurrence. Places already support proximity
(`internal/domain/places/service.go`, `internal/storage/postgres/places_repository.go`);
events have no proximity fields (`internal/domain/events/repository.go:258`).
The events list already joins the effective occurrence venue
(`internal/storage/postgres/events_repository.go:92`).

Each node serves its configured city/location scope (staging serves the GTA).
Radius filtering narrows that node's inventory; it does not discover events from
other nodes or replace the existing city/region boundary checks.

## Contract

`GET /api/v1/events?near_lat=43.6532&near_lon=-79.3832&radius=5`

The MCP `events` input gains optional numeric `near_lat`, `near_lon`, and `radius`
properties, with identical semantics. Existing input/output fields remain unchanged:

```json
{"name":"events","arguments":{"near_lat":43.6532,"near_lon":-79.3832,"radius":5,"limit":20}}
```

The response remains the existing event list envelope (`items`, `next_cursor`,
`warnings` for REST; existing MCP content envelope). No distance field is added.
Both coordinates are required if any proximity parameter is supplied; a radius
without coordinates is invalid. Latitude is finite and within [-90,90]; longitude
is finite and within [-180,180]. Radius is finite, positive and at most the configured
maximum. The default radius is 10 km; maximum 100 km. Coordinates of zero are valid.
Malformed REST input returns HTTP 400 with the existing RFC 7807 envelope; MCP uses
its existing invalid-filter error result. Unknown parameters retain their warnings.
The combined `near=lat,lon`, `near_place`, and guessed `lat`/`lng` parameters are
not introduced in this slice; use the established places coordinate parameter names.

## Data and query design

Extend the existing `events.Filters` with:

```go
Latitude  *float64
Longitude *float64
RadiusKm  *float64
```

Use the effective venue `COALESCE(event_occurrences.venue_id, events.primary_venue_id)`.
An explicit occurrence venue without coordinates does not fall back to the primary
venue's coordinates. Exclude occurrences without effective venue coordinates under
proximity filtering. Here, virtual-only means no effective geocoded venue; an
occurrence with a virtual URL and an effective geocoded venue is treated as hybrid
and may match, following the existing venue fallback semantics. Without proximity input, preserve existing results.

Apply radius and date conditions to the same occurrence before choosing the earliest
matching occurrence per event. Return each event once across pagination, ordered by
`start_time ASC, ulid ASC`. Apply the cursor after selecting the earliest matching
occurrence so later occurrences do not make an event reappear on subsequent pages.
Keep the existing cursor format. Preserve other filters and public visibility.
Use parameterized PostGIS geography `ST_DWithin` in metres (kilometres × 1000).
Ensure a matching geography-expression GiST index exists; migrations are reversible.
No new persisted event fields or third-party services are required.

## Configuration

Add a typed `EventSearchConfig` to `internal/config/config.go`, injected into the
event service via `WithEventSearchConfig`. Add `Service.ParseFilters(values, loc)`
which delegates to a config-aware parser; REST and MCP must call that method.
Keep the package-level `ParseFilters` as a default-config wrapper for existing
callers. Wire the config in both `internal/api/router.go` and
`cmd/mcp-server/main.go`. `EVENTS_DEFAULT_RADIUS_KM`
defaults to 10; `EVENTS_MAX_RADIUS_KM` defaults to 100. Reject invalid configuration
(malformed values, nonfinite/nonpositive values or default greater than maximum) during configuration
validation in `config.Load`, returning an error before the server starts.
Invalid radius messages should follow the existing `FilterError` style and
include the effective configured maximum. Document both defaults in OpenAPI, environment docs and examples.
Existing callers using zero-value service config retain defaults.

## User scenarios and verification

1. Given near and far venues, when querying a 5 km radius, then only events with a
   matching geocoded occurrence are returned; boundary distances use PostGIS.
2. Given multiple venues and dates, when both proximity and date filters are set,
   then the returned occurrence satisfies both filters and occurrence venue overrides
   primary venue. Missing coordinates do not create false positive matches.
3. Given recurring/multi-occurrence events and a page size of one, when following
   cursors, then every matching event appears exactly once, in chronological order.
4. Given invalid or incomplete proximity parameters, when querying REST or MCP,
   then errors are returned rather than unfiltered results. Zero coordinates work.
5. Given configured radius defaults/maxima, when parsing via REST or MCP, then
   both surfaces enforce the configured values. Existing no-proximity queries work.

## Security and constraints

Coordinates and radius are untrusted inputs: validate before database access, use
bound SQL arguments, and cap the radius. Keep existing public lifecycle filtering,
JSON-LD negotiation, CC0 defaults and provenance unchanged. No per-user location
storage is introduced. The index persists in PostgreSQL and uses existing backups.

## Implementation and acceptance

Single implementation ticket `t_dbb163bc` covers domain/config, storage, HTTP/MCP,
docs and skills. Follow red/green TDD. Targeted domain/config/handler/MCP tests and
real-PostGIS storage tests must pass, followed by `make ci-fast`, full pre-push CI,
and the required race gate before merging. Deploy HEAD to staging and run remote
checks plus read-only near/far/invalid-input probes before closure. Record query
plan evidence for the geography-expression index using representative fixtures.

## Deferred scope

Nearest-first sorting/distance-aware cursors, distance response fields, named-place
geocoding, maps, and coordinate backfill are follow-ups. Measure live coordinate
coverage during staging verification rather than reusing historical samples.

## Design review

Independent explorer review on 2026-11-16 identified parser/config wiring as
ambiguous. Resolved by using a configured service parsing method and explicitly
wiring both entrypoints before implementation. No remaining blocking findings.
