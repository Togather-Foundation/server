# Plan: ICS/iCal Integration

**Spec**: 005-ics-integration | **Date**: 2026-04-13 | **Status**: In Progress (Phase 3 Delivered)
**Goal**: Make ICS (RFC 5545) a first-class ingest and output format — the SEL can
consume ICS feeds from any source (including community-calendar) and produce
subscribable ICS feeds that any calendar client or aggregator can consume.

## Vision

ICS/iCal is the most reliable machine-readable event format on the open web. Every
calendar application — Google Calendar, Apple Calendar, Outlook, Thunderbird —
speaks it natively. Jon Udell's `community-calendar` project demonstrates that ICS
feeds are the pragmatic common denominator for aggregating community events: many
venues, libraries, and arts organizations already publish ICS feeds even when they
lack JSON-LD markup.

This integration adds two capabilities:

1. **ICS Ingest** — The scraper gains an `ics` source type that fetches and parses
   ICS feeds, maps VEVENT components to SEL EventInput, and submits them through the
   existing ingest pipeline (dedup, validation, review queue).

2. **ICS Export** — The API serves `text/calendar` responses: full event feeds at
   `GET /api/v1/events.ics` (filterable, paginated via `Link rel="next"`) and per-event
   downloads at `GET /api/v1/events/{id}/ics`.

A secondary goal is upgrading the recurrence model from the current ad-hoc columns
(`repeat_frequency`, `repeat_on_days`, `repeat_on_dates`) to canonical RRULE
(RFC 5545), which eliminates a lossy translation layer and enables round-trip
fidelity with ICS sources.

## Current State

| Capability | Status | Location |
|---|---|---|
| ICS parsing | **Phase 1 Delivered** — `arran4/golang-ical` v0.3.5 | `internal/ical/parse.go` |
| ICS serialization | **None** | — |
| `text/calendar` content negotiation | **None** | `internal/api/middleware/negotiate.go:14-19` |
| Scraper source types | `scraper`, `partner`, `user`, `federation` (Go); DB also allows `api`, `manual` | `internal/domain/provenance/validation.go:24-29` |
| Scraper tiers | Tier 0 (JSON-LD), Tier 1 (Colly/CSS), Tier 2 (Rod/headless), Tier 3 (GraphQL/REST) | `internal/scraper/config.go:20-92` |
| Event series recurrence | `repeat_frequency TEXT`, `repeat_on_days TEXT[]`, `repeat_on_dates INTEGER[]` | `migrations/000001_core.up.sql:112-114`, `models.go:193-195` |
| Event occurrences | `event_occurrences` table (1:many from events) | `migrations/000001_core.up.sql` |
| Content negotiation | `application/ld+json`, `application/json`, `text/html`, `text/turtle` | `internal/api/middleware/negotiate.go:14-19` |
| Change feed | `GET /api/v1/feeds/changes` with cursor pagination | `internal/api/handlers/feeds.go:27`, `router.go:808` |
| Scraper source configs | YAML files + `scraper_sources` DB table, `server scrape sync` | `configs/sources/*.yaml`, migration `000032` |
| Latest migration | `000042_scraper_sources_extraction_method` | `internal/storage/postgres/migrations/` |
| ICS extractor | **Phase 1 Delivered** — RRULE expansion via `teambition/rrule-go` v1.8.2 | `internal/scraper/ics.go`, `internal/ical/rrule.go` |
| EventSeries handling in scraper | Detects `Event`/`EventSeries` in JSON-LD and unfolds `subEvent` into occurrences during normalization | `internal/scraper/jsonld.go:45,168,231,306`, `internal/scraper/normalize.go:98-161` |

## Architecture

### Data Flow: ICS Ingest

```
ICS Feed URL                    Existing SEL Pipeline
     │                                  │
     ▼                                  │
┌──────────────┐                        │
│  ICS Fetcher │ HTTP GET, ETag/        │
│  (net/http)  │ If-Modified-Since      │
└──────┬───────┘                        │
       │ raw []byte                     │
       ▼                                │
┌──────────────┐                        │
│  ICS Parser  │ arran4/golang-ical     │
│              │ VCALENDAR → []VEVENT   │
└──────┬───────┘                        │
       │ []ics.VEvent                   │
       ▼                                │
┌──────────────┐                        │
│  ICS→Event   │ VEVENT → EventInput    │
│  Mapper      │ RRULE → series hints   │
└──────┬───────┘                        │
       │ []events.EventInput            │
       ▼                                ▼
┌──────────────────────────────────────────┐
│           Ingest Pipeline                │
│  Validate → Dedup → Create → Review      │
│  (existing — no changes needed)          │
└──────────────────────────────────────────┘
```

### Data Flow: ICS Export

```
Calendar Client (GET)
     │
     ▼
┌──────────────────────────┐
│  Content Negotiation     │
│  Accept: text/calendar   │
│  OR .ics URL extension   │
└──────────┬───────────────┘
           │
           ▼
┌──────────────────────────┐
│  Events Handler          │
│  Same filters as JSON:   │
│  ?start_date, ?end_date, │
│  ?city, ?query, ?limit   │
└──────────┬───────────────┘
           │ []events.Event
           ▼
┌──────────────────────────┐
│  Event→ICS Serializer    │
│  arran4/golang-ical      │
│  Event → VEVENT          │
│  Series → VEVENT+RRULE   │
└──────────┬───────────────┘
           │ VCALENDAR []byte
           ▼
    HTTP Response
    Content-Type: text/calendar
    Content-Disposition: attachment
```

### Package Layout

```
internal/
  ical/                       # Phase 1 delivered — ICS parsing and mapping
    parse.go                  # VCALENDAR parsing → ParsedCalendar/ParsedEvent structs
    parse_test.go
    mapper.go                 # ParsedEvent → events.EventInput conversion
    mapper_test.go
    rrule.go                  # RRULE parsing, expansion (teambition/rrule-go)
    rrule_test.go
    doc.go                    # package doc
    serialize.go              # Phase 2 — events.Event → VCALENDAR serialization
    serialize_test.go         # Phase 2
  scraper/
    ics.go                    # Phase 1 delivered — ICS extractor (fetch + parse + map)
    ics_test.go
  api/
    middleware/
      negotiate.go            # Phase 2 — MODIFIED to add text/calendar
    handlers/
      ics.go                  # Phase 2 — ICS feed + single-event download handlers
      events.go               # Phase 2 — MODIFIED for Link alternate headers
tests/
  testdata/
    ics/                      # Phase 1 delivered — ICS fixture files (15 .ics + README.md)
      README.md               # Fixture ownership + naming rules (parse-*, export-*, interop-*)
      parse-basic-event.ics
      parse-multi-event.ics
      parse-malformed.ics
      ...                     # (see spec-phase1.md for full list)
```

## Design Constraints

1. **SEL compliance (non-negotiable)**:
   - CC0 license defaults on ingested events
   - Source provenance preserved (ICS `X-SOURCE` header, `SOURCE` property)
   - RFC 7807 error responses for malformed ICS
   - Error wrapping with `fmt.Errorf("...: %w", err)`

2. **ICS standards compliance**:
   - RFC 5545 (iCalendar) for parsing/serialization
   - RFC 7986 (New Properties for iCalendar) for `SOURCE`, `COLOR`, etc.
   - Produced ICS must validate against validators.icalendar.org

3. **Scraper integration**: ICS sources use the existing `scraper_sources` table
   and River job scheduling. `tier` remains 0-3 for the existing scraper tiers;
   ICS sources set `extraction_method = 'ics'` in a new column (not a new tier number)
   to avoid conflating extraction mode with tier semantics.

4. **Content negotiation**: `text/calendar` added alongside existing types. The `.ics`
   URL extension is an alternative to `Accept` header negotiation (same pattern as
   hypothetical `.jsonld` extension).

5. **Backward compatibility**: Existing `event_series` recurrence columns are migrated
   to RRULE in Phase 3. Migration is additive (new column first, then data migration,
   then drop old columns). No breaking API changes.

## Component Design

### ICS Parser Output

```go
// internal/ical/parse.go

// ParsedCalendar holds the result of parsing a VCALENDAR.
type ParsedCalendar struct {
    ProdID   string         // PRODID value
    Name     string         // X-WR-CALNAME
    Source   string         // SOURCE property (RFC 7986)
    Events   []ParsedEvent
    Warnings []string       // Non-fatal parse issues
}

// ParsedEvent holds a single VEVENT extracted from a VCALENDAR.
type ParsedEvent struct {
    UID          string
    Summary      string
    Description  string
    Location     string
    URL          string
    Start        time.Time
    End          time.Time
    Duration     time.Duration    // DURATION property (zero if absent; used when DTEND missing)
    AllDay       bool
    RRULE        string           // Raw RRULE string, empty if non-recurring
    RecurrenceID time.Time        // RECURRENCE-ID (zero if absent; exception to RRULE series)
    ExDates      []time.Time      // EXDATE values
    RDates       []time.Time      // RDATE values
    Organizer    string           // ORGANIZER CN parameter (display name)
    OrganizerEmail string         // ORGANIZER mailto: value (email)
    Categories   []string         // CATEGORIES values (multi-value, multi-property)
    GeoLat       float64          // GEO latitude (0 if absent)
    GeoLon       float64          // GEO longitude (0 if absent)
    Created      time.Time        // CREATED
    LastMod      time.Time        // LAST-MODIFIED
    Sequence     int              // SEQUENCE (change counter)
    Status       string           // STATUS: CONFIRMED, TENTATIVE, CANCELLED
    RawProps     map[string]string // Other properties for payload preservation (incl. X-SOURCE, X-GOOGLE-*, etc.)
}
```

### ICS → EventInput Mapper

```go
// internal/ical/mapper.go

// MapToEventInputs converts parsed ICS events to SEL EventInputs.
// Non-recurring events become a single EventInput.
// Recurring events with RRULE expand occurrences within the horizon window
// and produce one EventInput per occurrence (linked by series metadata).
// ctx enables cancellation during large RRULE expansions.
func MapToEventInputs(ctx context.Context, cal *ParsedCalendar, opts MapperOptions) ([]events.EventInput, []string, error)

type MapperOptions struct {
    SourceURL       string        // Feed URL for provenance
    SourceName      string        // Source config name (→ Source.Name)
    TrustLevel      int           // Assigned trust level
    License         string        // Default "CC0-1.0"
    Timezone        string        // Fallback timezone for floating times
    HorizonDays     int           // How far to expand recurring events (default: 90)
    MaxOccurrences  int           // Safety cap on expanded occurrences (default: 100)
}
```

### Event → ICS Serializer

```go
// internal/ical/serialize.go

// SerializeEvents converts SEL events to a VCALENDAR payload.
func SerializeEvents(evts []events.Event, opts SerializeOptions) (SerializeResult, error)

// SerializeSingleEvent converts one domain event to a VCALENDAR payload.
func SerializeSingleEvent(evt events.Event, opts SerializeOptions) (SerializeResult, error)

// SerializeResult contains serialized bytes and non-fatal warnings.
// (Implementation note: original design had Content/EventCount/GeneratedAt;
// actual implementation uses Data/Warnings for idiomatic Go error handling.)
type SerializeResult struct {
    Data     []byte
    Warnings []string
}

type SerializeOptions struct {
    CalendarName        string // feed title (default: "Togather Events")
    CalendarDescription string // optional feed description
    // Phase 3 adds: IncludeRRule bool, Timezone string, ProductID string
}
```

> **Go 1.25 ServeMux constraint**: the `net/http` ServeMux rejects patterns where a
> wildcard segment (`{id}`) is immediately followed by a literal suffix (e.g.
> `{id}.ics`). The wildcard must end the path segment. For this reason the
> single-event endpoint uses `GET /api/v1/events/{id}/ics` (wildcard + `/ics` suffix
> segment) rather than `GET /api/v1/events/{id}.ics` as originally specified. The feed
> endpoint `GET /api/v1/events.ics` (no wildcard) is unaffected.

### Scraper ICS Source

```go
// internal/scraper/ics.go

// ICSExtractor fetches and parses an ICS feed URL.
type ICSExtractor struct {
    client  *http.Client
    mapper  ical.MapperOptions
}

// Extract fetches the ICS feed and returns EventInputs.
func (e *ICSExtractor) Extract(ctx context.Context, cfg SourceConfig) ([]events.EventInput, []string, error)
```

### Extraction Method Column Addition

```sql
-- Migration 000042_scraper_sources_extraction_method.up.sql
ALTER TABLE scraper_sources
  ADD COLUMN extraction_method TEXT NOT NULL DEFAULT 'scraper'
    CHECK (extraction_method IN ('scraper', 'ics'));
```

## Implementation Phases

### Phase Dependency Table

| Phase | Contract Consumed | Contract Produced |
|---|---|---|
| Phase 1 (Ingest) | Existing scraper dispatch + ingest pipeline + scraper source config model | ICS parse/map/extract path, `extraction_method` dispatch contract, stable `Source.EventID` derivation |
| Phase 2 (Export) | Phase 1 parsed/mapped event identity contract + occurrence data model | ICS serialization contract, API export endpoints, `Link rel="next"` pagination, discovery headers |
| Phase 3 (Recurrence) | Phase 2 serialization behavior + legacy `event_series` schema | Canonical recurrence persistence (`rrule/exdates/rdates`), recurrence projection contract |
| Phase 4 (Interop/Docs) | Phase 1-3 runtime behavior and contracts | Interop fixture corpus + validation contract, operations runbook |
| Phase 5 (Inventory Rollout) | Phase 4 runbook + interop fixture expectations + Toronto inventory research | Cohort rollout manifest and execution/reporting contract |

### Phase 1: ICS Ingest (Vertical Slice) — **Delivered**

**Goal**: `server scrape source <ics-source>` fetches an ICS feed, parses it, and
ingests events through the existing pipeline. End-to-end from ICS URL to events in DB.

**Entry criteria**: None (greenfield).
**Exit criteria**: ICS source scraping works via CLI; 5+ real ICS feeds tested;
unit tests for parser, mapper; integration test with httptest ICS server.
**Status**: All 6 tasks completed. 7 P2 review issues fixed. See `spec-phase1.md`
for implementation deviations.

**Tasks** (6):
1. Add `arran4/golang-ical` and `teambition/rrule-go` dependencies
2. Implement `internal/ical/parse.go` + `parse_test.go` — VCALENDAR parsing
3. Implement `internal/ical/mapper.go` + `mapper_test.go` — VEvent → EventInput
4. Implement `internal/ical/rrule.go` + `rrule_test.go` — RRULE expansion
5. Implement `internal/scraper/ics.go` + `ics_test.go` — ICS extractor wired into scraper
6. Add `extraction_method` column to `scraper_sources` (migration 000042), update `SourceConfig`,
   wire ICS tier dispatch in `scraper.go`.
   **Guardrail**: this is additive for `scraper_sources` only; do not rename or alter
   provenance `sources.source_type` (migration 000002).

**Interface contract (Phase 1 → Phase 2)**:
- `ical.SerializeOptions` struct defined but `serialize.go` not implemented
- `ParsedEvent` struct is stable — Phase 2 uses it for round-trip testing

### Phase 2: ICS Export (Vertical Slice) — **Delivered**

**Goal**: `GET /api/v1/events.ics` returns an agent/API-consumable ICS feed with
cursor pagination; `GET /api/v1/events/{id}/ics` returns a single-event download.

**Entry criteria**: Phase 1 delivered (parser/mapper stable).
**Exit criteria**: feed pagination (`after`/`limit`) works for ICS via `Link rel="next"`; per-event download
produces valid ICS; API endpoints expose `Link` alternates for ICS discovery; calendar
compatibility validated (Apple/Google import/subscription smoke tests).
**Status**: All 7 tasks completed. Multiple follow-up bugs fixed (double-escaping,
DTSTAMP, OAS3 violations, route rename). See `spec-phase2.md` for implementation
deviations.

**Tasks** (7):
1. Implement `internal/ical/serialize.go` + `serialize_test.go` — Event → VCALENDAR
2. Add `text/calendar` to content negotiation middleware (`negotiate.go`)
3. Add `GET /api/v1/events.ics` feed handler with same filters as JSON list endpoint
   plus cursor pagination semantics (`after`, `limit`, `Link rel="next"` per RFC 8288)
4. Add `GET /api/v1/events/{id}/ics` single-event download handler
   (**Note**: route changed from original `{id}.ics` — see Go 1.25 ServeMux constraint above)
5. Add `Link` discovery headers with `rel="alternate"; type="text/calendar"` on
   `/api/v1/events` and `/api/v1/events/{id}`
6. Update `docs/api/openapi.yaml` with ICS endpoints
7. Bind Phase 2 tests to `docs/integration/ics-compatibility-matrix.md`

**Interface contract (Phase 2 → Phase 3)**:
- `SerializeEvents` handles series events by emitting multiple VEVENTs
  (one per occurrence). Phase 3 upgrades this to emit RRULE on the series VEVENT.
- Feed pagination uses `Link rel="next"` (RFC 8288), not custom headers.

### Phase 3: Recurrence Model Upgrade — **Delivered**

**Goal**: Replace `repeat_frequency`/`repeat_on_days`/`repeat_on_dates` with canonical
RRULE storage and wire recurrence metadata through domain/query/serialization paths.

**Entry criteria**: Phase 1-2 delivered.
**Exit criteria**: `event_series.rrule` column populated (or confirmed unnecessary);
old columns dropped;
JSON-LD export projects recurrence from canonical RRULE while preserving `subEvent`
compatibility; ICS export emits RRULE+EXDATE+RDATE.

**Tasks** (7 planned → 6 implemented; Task 2 eliminated — legacy columns confirmed empty):
1. Check if legacy repeat columns have data; add `rrule TEXT`, `exdates TIMESTAMPTZ[]`,
   `rdates TIMESTAMPTZ[]` to `event_series` (additive migration)
2. ~~Backfill legacy repeat columns to RRULE equivalents~~ — **eliminated** (columns confirmed empty)
3. Update recurrence-aware repository/domain plumbing (inline LEFT JOIN in `GetByULID`,
   not a separate SQLc query file; `RecurrenceRule` type with `SeriesStart`/`SeriesEnd`)
4. Update JSON-LD serialization: generate `Schedule`-style recurrence projection
   from RRULE while preserving existing `subEvent` responses
5. Update ICS serializer to emit RRULE/EXDATE/RDATE on series events
6. Drop `repeat_frequency`, `repeat_on_days`, `repeat_on_dates` after verification
7. Preserve deterministic recurring export UID stability across Phase 2 -> 3 cutover
**Status**: All 6 tasks completed. 4 post-review fixes (RFC 5545 format, dead code,
`eventSchedule` date boundaries). See `spec-phase3.md` Delivery Reflection.

**Interface contract (Phase 3 → Phase 4)**:
- `RecurrenceRule` struct (domain) carries `RRule`, `ExDates`, `RDates`, `TZID`,
  `SeriesStart`, `SeriesEnd` — stable shape for interop fixture validation.
- `eventSchedule` JSON-LD field is `omitempty` — non-recurring events emit no
  `eventSchedule`; consumers must handle both shapes.
- `IncludeRRule = false` (default) ICS output is wire-identical to Phase 2.
  `IncludeRRule = true` emits a single VEVENT with RRULE/EXDATE/RDATE.

### Phase 4: Interop & Documentation

**Goal**: Validated interop with real external ICS feed shapes; platform discovery
heuristics for ICS feeds; operational runbook and repeatable integration validation.

**Entry criteria**: Phase 1-3 delivered.
**Exit criteria**: 7 representative external ICS fixture shapes ingest successfully into
SEL; SEL ICS export passes consumer checks in both `IncludeRRule=false` and
`IncludeRRule=true` modes; ICS discovery heuristics for 8 platforms documented; runbook
is executable.

**Tasks** (4):
1. Test: external ICS feed shapes → SEL ingest — 7 fixtures (Outlook VTIMEZONE,
   WordPress Tribe, Google Calendar, Meetup, Tockify, RRULE+EXDATE TZID-local-time
   regression, valid-mixed-with-malformed); all assertions map to compatibility matrix rows
2. Test: SEL ICS feed → external consumer — both `IncludeRRule=false` (default) and
   `IncludeRRule=true` modes; EXDATE TZID format (RFC 5545 §3.3.5); `eventSchedule`
   JSON-LD; all assertions map to compatibility matrix rows
3. Update `docs/integration/event-platforms.md` with ICS discovery heuristics for 8
   platforms (WordPress Tribe `?ical=1`, MEC `?mec-ical-feed=1`, Events Manager,
   Tockify, Google Calendar `basic.ics`, Meetup `events/ical/`, Eventbrite bridge,
   static `<link rel="alternate" type="text/calendar">`) with fallback guidance
4. Write `docs/integration/ics-feeds.md` — operational guide referencing
   `extraction_method: ics`, `server scrape sync`, diagnostics paths

**Interface contract (Phase 4 → Phase 5)**:
- Interop fixture corpus and validation scripts define accepted external ICS shapes.
- Operational runbook defines safe onboarding/diagnostics workflow for source rollout.

### Phase 5: Toronto ICS Source Inventory Rollout

**Goal**: Operationalize the Toronto ICS inventory into prioritized, trackable source
onboarding work and staged rollout metrics.

**Entry criteria**: Phase 4 delivered (interop tests + runbook available).
**Exit criteria**: inventory is converted into rollout cohorts with ownership,
success/failure tracking, and documented decisions for overlap, net-new, and
non-starter sources.

**Tasks** (6):
1. Convert inventory sections (overlap, SEL-only, net-new, non-starters) into a
   machine-readable manifest (CSV/JSON) under `specs/005-ics-integration/`.
2. Define rollout cohorts (e.g., high-value net-new first, overlap validation
   second) with explicit acceptance targets per cohort.
3. Create source-onboarding beads from the manifest (one bead per source or source
   bundle) with priority and dependency metadata.
4. Add outcome taxonomy (`onboarded`, `deferred`, `blocked`, `non-starter`) and
   capture criteria to avoid ad-hoc status labels.
5. Run a first staged cohort in staging and publish metrics (attempted, onboarded,
   blocked reasons, median setup time) in a rollout report.
6. Feed lessons back into `docs/integration/event-platforms.md` and
   `docs/integration/ics-feeds.md` with concrete examples from Toronto sources.

## Final Release Gate

- Use `specs/005-ics-integration/release-gate-checklist.md` as the phase-agnostic
  ship gate before feature release.
- Compatibility verification must reference
  `docs/integration/ics-compatibility-matrix.md`.

## Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Malformed ICS in the wild (missing required fields, wrong encodings) | High | Lenient parser with warnings; skip unparseable VEVENTs, don't fail the whole feed |
| RRULE expansion produces unbounded occurrences | High | `MaxOccurrences` cap (default 100); `HorizonDays` window (default 90 days) |
| Timezone handling (floating times, VTIMEZONE definitions) | Medium | `time.LoadLocation` for IANA TZIDs; `WindowsTZIDAliases` map for Outlook/Exchange non-IANA TZIDs; fall back to source config timezone → UTC; log warning on ambiguous times |
| ICS feeds change UID format between fetches → duplicate events | Medium | Dedup by content hash (name + venue + startDate) in addition to UID-based dedup |
| Large ICS feeds (1000+ events) increase memory use | Low | `MaxBodyBytes` (10 MB) bounds raw input. Scraper extraction is per-event, then submission uses the existing batch ingest client with chunking (up to 100 events per POST) to control payload size. A 6,558-event feed produces ~20-40 MB in memory — manageable. |
| `arran4/golang-ical` doesn't handle edge case X | Medium | Library is actively maintained (last release 2025); can contribute fixes upstream or wrap/patch |

## Security

### Trust Boundaries

- **Untrusted**: ICS feed content (event names, descriptions, URLs, locations).
  Same trust model as Tier 0-3 scraper output.
- **Trusted**: Source configuration (YAML/DB) — defines feed URL, trust level, license.
- **Trusted**: ICS serialization output — generated from validated DB data.

### Input Sanitization

- ICS `DESCRIPTION` may contain HTML or scripts → strip HTML tags, same as scraper
  normalization (`internal/domain/events/normalize.go`)
- ICS `URL` values → validate URL format, reject non-http(s) schemes
- ICS `ATTACH` properties → ignore (don't fetch external attachments)
- ICS feed size → enforce `MaxBodyBytes` limit (same as scraper: 10 MB default)
- ICS property injection → `arran4/golang-ical` handles escaping; we don't
  concatenate raw ICS strings

### Defense Layers

1. HTTP client: redirect-limited (10 hops, same as Tier 3 REST), timeout, body size limit (existing scraper client)
2. ICS parser: lenient mode, skip malformed events, count errors
3. Mapper: validate required fields (summary, start date), reject incomplete events
4. Ingest pipeline: existing validation, dedup, review queue (unchanged)
5. Serializer: output only validated DB data; PRODID identifies SEL as producer

## Open Questions

1. **ETag/If-Modified-Since caching**: Should the ICS fetcher store ETags per source
   in `scraper_sources` for conditional GET? (Likely yes — reduces bandwidth for
   hourly/daily polls. Can add a `last_etag TEXT` column alongside `extraction_method`.)

2. **VALARM (reminders)**: Should exported ICS include VALARM components? Most
   aggregator feeds omit them. Recommendation: omit in Phase 2, add as opt-in later.

3. **VTIMEZONE embedding**: Should exported ICS embed VTIMEZONE definitions or rely
   on UTC timestamps? Recommendation: use UTC (`Z` suffix) for maximum compatibility;
   embed VTIMEZONE only if a future consumer requests it.

4. **Community-calendar X-SOURCE header format**: Need to confirm the exact format
   community-calendar uses for provenance attribution in ICS. Documented as `X-SOURCE`
   but may have structured sub-fields.

## Toronto ICS Source Inventory

Cross-reference of community-calendar Toronto feeds (`cities/toronto/feeds.txt`,
89 ICS feed URLs) against existing SEL scraper configs (`configs/sources/*.yaml`,
107 sources). Data sourced from community-calendar's `SOURCES_CHECKLIST.md` (115
sources documented with event counts, discovery methods, and platform details).

### Overlap: 11 Sources (SEL scrapes HTML/JSON-LD; ICS also available)

These venues already have SEL scraper configs. ICS feeds could serve as fallback,
coverage comparison, or preferred ingest method if ICS proves more reliable.

| Source | SEL Config | SEL Method | ICS Type |
|--------|-----------|------------|----------|
| Bloor West Village BIA | `bloor-west-village-bia` | Tier 0 JSON-LD | WordPress Tribe `?ical=1` |
| Buddies in Bad Times | `buddies-in-bad-times` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Factory Theatre | `factory-theatre` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Gardiner Museum | `gardiner-museum` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Glad Day Bookshop | `glad-day-bookshop` | HTML/JSON-LD | Eventbrite via eb-to-ical |
| High Park Nature Centre | `high-park-nature-centre` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Jazz Bistro | `jazz-bistro` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Textile Museum | `textile-museum` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Toronto Botanical Garden | `toronto-botanical-garden` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Toronto Knitters Guild | `toronto-knitters-guild` | HTML/JSON-LD | WordPress Tribe `?ical=1` |
| Union Station | `toronto-union` | Tier 0 JSON-LD | WordPress Tribe `?ical=1` |

### SEL-Only: ~96 Sources (no ICS feed available)

Arts institutions, theatres, music venues, BIAs, and galleries that do not publish
ICS feeds. Our HTML/JSON-LD/CSS scrapers remain the only ingest path. Includes AGO,
ROM, Harbourfront Centre, TSO, National Ballet, Canadian Stage, Tarragon Theatre,
Hot Docs, Soulpepper, and 87 others. Community-calendar's `SOURCES_CHECKLIST.md`
confirms many of these as "Non-Starters" for ICS (Cloudflare-protected, no feed,
404, etc.).

### Net-New via ICS: 91 Sources

Sources in community-calendar that have working ICS feeds but no SEL scraper config.
These become ingestible once ICS support lands.

#### By Feed Type

| Feed Type | Count | URL Pattern | Examples |
|-----------|-------|-------------|---------|
| Meetup ICS | 49 | `meetup.com/<group>/events/ical/` | Civic Tech, Python, JavaScript, TechTO, hiking, yoga, dance, book clubs |
| WordPress Tribe | 14 | `<domain>/events/?ical=1` | Bata Shoe Museum, NOW Toronto, Grossman's Tavern, Ontario Nature, CultureLink |
| UofT WordPress Tribe | 4 | `<dept>.utoronto.ca/events/?ical=1` | Engineering, Philosophy, Social Work, Indigenous Studies |
| Tockify | 4 | `tockify.com/api/feeds/ics/<cal>` | torevent (~2,900 events), Distillery District, St. Lawrence NA, Councillor Myers |
| Google Calendar | 3 | `calendar.google.com/.../basic.ics` | CITA Local Events, Seminars, Special Events |
| Eventbrite bridge | 1 | `eb-to-ical.daylightpirates.org/...` | Another Story Bookshop |
| WordPress MEC | 1 | `<domain>/?mec-ical-feed=1` | York University (~6,558 events) |
| Static ICS | 1 | Direct `.ics` file URL | Show Up Toronto |
| WordPress Events Manager | 1 | `<domain>/events/?ical=1` (alt plugin) | Repair Cafe Toronto |
| **Total** | **78 unique + 13 addl Meetup** | | |

#### High-Value Sources

| Source | Feed Type | Est. Events | Why High-Value |
|--------|-----------|-------------|----------------|
| torevent (Toronto Events) | Tockify ICS | ~2,900 | Largest single aggregator — music, comedy, film, nightlife |
| York University | WordPress MEC | ~6,558 | Huge feed, needs date filtering |
| CultureLink | WordPress Events Mgr | ~494 | Newcomer/community events — underserved audience |
| 49 Meetup groups | Meetup ICS | ~450 total | Tech, social, outdoor, arts — grassroots community |
| NOW Toronto | WordPress Tribe | ~30 | Major arts/culture aggregator |
| CITA (3 feeds) | Google Calendar | ~1,600 | Science/education — unique niche |
| Repair Cafe Toronto | WordPress ICS | ~82 | Maker/sustainability community |

#### Meetup Groups by Category

| Category | Groups | Examples |
|----------|--------|---------|
| Tech/Dev | 8 | Civic Tech, Python, JavaScript, TechTO, AI/ML, Postgres, DevOps, Microsoft Reactor |
| Hiking/Outdoors | 6 | Bruce Trail, Hiking Network, GTA Hiking, Wilderness Union, Boots for Hiking, Bike |
| Social/Activities | 5 | 20s-30s Social, Soul City, Try New Things, Experience TO, Singles |
| Book Clubs | 4 | A Book Club Downtown, Sci Fi, Post-Apocalyptic, Silent Book Club |
| Language Exchange | 3 | TorontoBabel, Japanese-English, TILE Language Party |
| Dance/Music | 2 | Salsa/Bachata/Kizomba, Go Latin Dance |
| Board Games | 2 | Board Games & Social, Heavy Boardgamers |
| Kids/Family | 3 | Toronto Dads, Little Sunbeams, Mini+Me Meetups |
| Yoga/Wellness | 3 | High Park Yoga, Mindful Movement, Toronto Wellness |
| Water Sports | 3 | SUP/Kayak/Canoe, Paddlers, Canoe Trippers |
| Comedy/Improv | 2 | Improv for New Friends, Acting & Karaoke |
| Arts/Crafts | 2 | Arts & Culture, Midtown Arts & Crafts |
| History | 2 | History of Parkdale, Medieval/Renaissance SCA |
| Photography | 1 | Toronto Photography Group |
| Running | 1 | Founders Running Club |
| Film | 1 | Toronto Movies & Social |
| Volunteering | 1 | SAI Dham Canada Volunteer |
| 3D Printing | 1 | Toronto 3D Printing |
| Business | 1 | Women in Business |

### Platform Fingerprints for ICS Discovery

Patterns observed in community-calendar's Toronto research, useful for identifying
ICS feeds on new venues (to be documented in `docs/integration/event-platforms.md`
during Phase 4):

| Platform | ICS Endpoint Pattern | Detection Signal |
|----------|---------------------|------------------|
| WordPress Tribe Events | `<domain>/events/?ical=1` | `tribe-events` in page source |
| WordPress MEC | `<domain>/?mec-ical-feed=1` | `mec-single-event` class in HTML |
| WordPress Events Manager | `<domain>/events/?ical=1` | `em-events` in page source |
| Tockify | `tockify.com/api/feeds/ics/<cal-name>` | `tockify.com` embed in page |
| Google Calendar | `calendar.google.com/calendar/ical/.../basic.ics` | `calendar.google.com` embed |
| Meetup | `meetup.com/<group>/events/ical/` | Meetup group URL exists |
| Eventbrite (via bridge) | `eb-to-ical.daylightpirates.org/...?organizer=<id>` | Eventbrite organizer page |
| Static ICS | Direct `.ics` file URL | `<link rel="alternate" type="text/calendar">` |

### Non-Starters (from community-calendar research)

Community-calendar's `SOURCES_CHECKLIST.md` documents 40+ Toronto sources that were
investigated and found to have no usable ICS/RSS feeds. Notable overlaps with SEL's
existing scraper targets (where we use HTML/JSON-LD instead): AGO (Cloudflare 403),
ROM (404), TIFF (no feed), Mirvish (no feed), Harbourfront (empty RSS), Canadian
Stage (no Tribe ICS), TSO (no feed), Aga Khan (no Tribe ICS), Hot Docs (no Tribe).
These confirm that our HTML/JSON-LD scrapers cover venues that ICS cannot reach.

### Community-Calendar Infrastructure Notes

- **Scraper ecosystem**: Python-based scrapers in `scrapers/` with reusable base
  classes for Elfsight, CitySpark, CKAN, Bibliocommons, Bookmanager platforms
- **Toronto-specific scrapers**: `blogto.py` (BlogTO API), `toronto_meetings.py`
  (City Council via CKAN), `toronto_festivals.py` (City festivals via CKAN),
  `toronto_public_library.py` (TPL kids/family via Bibliocommons), `bookmanager.py`
  (indie bookstore events), `volunteer_toronto.py`, `uoft_events.py`
- **Eventbrite bridge**: `eb-to-ical.daylightpirates.org` converts organizer pages
  to ICS — single point of failure for 9 bookstore/publisher feeds
- **Geo-filtering**: `cities/toronto/geo_allowlist.txt` limits to Toronto + 5 inner
  boroughs (North York, Scarborough, Etobicoke, East York, York)
- **Venue deny-list**: `bookstore_venue_denylist.txt` drops chain stores (Indigo/Chapters)
  from publisher feeds
