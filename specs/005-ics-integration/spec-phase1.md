# Phase 1 Specification: ICS Ingest

**Spec**: 005-ics-integration / Phase 1 | **Date**: 2026-04-13 | **Status**: Delivered
**Parent**: `specs/005-ics-integration/plan.md`
**Goal**: `server scrape source <ics-source>` fetches an ICS feed, parses it, and
ingests events through the existing pipeline. End-to-end from ICS URL to events in DB.

## Context

ICS (RFC 5545) is the most widely published machine-readable event format on the
open web. Community-calendar's Toronto research identifies 91 sources with working
ICS feeds that have no SEL scraper config — including 49 Meetup groups, the torevent
aggregator (~2,900 events), and 14 WordPress Tribe sites. Adding ICS ingest as a
source type roughly doubles our Toronto source coverage.

### What Exists Today

| Component | Status | Relevant Code |
|---|---|---|
| Scraper 4-tier dispatch (0-3) | Production | `internal/scraper/scraper.go:350-361` |
| `SourceConfig` struct | Production | `internal/scraper/config.go:20-86` |
| `EventInput` struct | Production | `internal/domain/events/validation.go:44-70` |
| `IngestService.Ingest()` | Production | `internal/domain/events/ingest.go:74-76` |
| Dedup (SHA-256 + trigram) | Production | `internal/domain/events/dedup.go` |
| Content negotiation (JSON-LD, JSON, HTML, Turtle) | Production | `internal/api/middleware/negotiate.go:14-19` |
| Source types (Go validation) | Production | `internal/domain/provenance/validation.go:24-29` |
| `scraper_sources` DB table | Production | `migrations/000032_scraper_sources.up.sql` |
| `server scrape source <name>` CLI | Production | `cmd/server/cmd/scrape_cmd.go` |
| `server scrape sync` (YAML → DB) | Production | `cmd/server/cmd/scrape_sync.go` |
| Latest migration | `000041` | `migrations/000041_scraper_sources_default_location` |
| ICS parsing library | **None** | — |
| ICS source type | **None** | — |

### What Phase 1 Delivers

1. **`internal/ical/` package** — ICS parsing, VEVENT→EventInput mapping, RRULE
   expansion. New Go package with no SEL-domain dependencies except `events.EventInput`.
2. **ICS extractor** — `internal/scraper/ics.go` wires `ical.Parse()` and
   `ical.MapToEventInputs()` into the scraper pipeline.
3. **`extraction_method` column** — `scraper_sources.extraction_method` (`'scraper'` | `'ics'`)
   enables the scraper dispatch to route ICS sources to the ICS extractor.
4. **YAML config support** — `extraction_method: ics` in source YAML files, synced to DB
   via `server scrape sync`.
5. **CLI integration** — `server scrape source <ics-source>` works end-to-end.

### Non-Goals (Phase 1)

- **ICS export / `text/calendar` responses** — Phase 2
- **`webcal://` subscription support** — Deferred (not in any current phase; API-first approach uses HTTPS)
- **Recurrence model upgrade (RRULE column in `event_series`)** — Phase 3
- **Community-calendar interop testing** — Phase 4
- **Platform discovery heuristics documentation** — Phase 4
- **Scheduled/automated ICS polling** — Phase 1 supports manual `server scrape source`
  and the existing River job scheduler (same as other tiers). No new scheduling infra.
- **ETag/If-Modified-Since caching** — Deferred. The HTTP client will fetch the full
  feed each run. Conditional GET can be added later by storing ETags in
  `scraper_sources` (noted in plan.md Open Questions). **Cost note**: without ETag
  caching, a full daily sync of all ICS sources re-processes ~10,000 events × dedup
  check (~1 ms each) ≈ 10 seconds of DB queries. Acceptable at daily frequency;
  would become a concern at hourly. ETag caching (Phase 2+) reduces this to near-zero
  for unchanged feeds.

### Design Constraint Reminders

- **SEL compliance**: CC0 license default, source provenance preserved, RFC 7807 errors,
  `fmt.Errorf("...: %w", err)` wrapping.
- **Scraper integration**: ICS sources reuse the existing `scraper_sources` table and
  River scheduling. No new tier number — `extraction_method = 'ics'` is orthogonal to tiers.
- **Ingest pipeline**: No changes to `IngestService`, validation, dedup, or review
  queue. ICS events enter through the same `EventInput` interface as all other sources.
- **Phase 2 compatibility**: Preserve stable `Source.EventID` derivation for every
  mapped ICS event/occurrence (including RRULE-expanded events) so Phase 2 export can
  generate deterministic VEVENT UIDs across runs.

### Out-of-Phase Guardrails

- If implementation pressure suggests adding ICS HTTP export endpoints or
  `text/calendar` response support, stop and create a Phase 2 follow-up bead.
- If recurrence persistence schema changes are needed (`event_series` RRULE fields),
  stop and create a Phase 3 follow-up bead.
- If cross-consumer interop harness work grows beyond ingest validation, stop and
  create a Phase 4 follow-up bead.

---

## User Scenarios & Testing

### User Story 1 — ICS Feed Is Fetched and Events Ingested (Priority: P1)

An operator configures an ICS source in YAML, syncs it, and runs the scraper. Events
from the ICS feed appear in the database.

**Independent Test**: Given a YAML config with `extraction_method: ics` pointing to an
httptest server serving a multi-event ICS file, when `ScrapeSource()` runs, events
are returned in the `ScrapeResult` with correct field mapping.

**Acceptance Scenarios**:

1. **Given** a YAML config `configs/sources/test-ics.yaml` with `extraction_method: ics`,
   `url: https://example.com/events.ics`, and `trust_level: 5`, **When**
   `server scrape sync` runs, **Then** the source appears in `scraper_sources` with
   `extraction_method = 'ics'`.

2. **Given** an ICS feed with 3 VEVENTs (each with SUMMARY, DTSTART, DTEND,
   LOCATION, DESCRIPTION, URL), **When** `ScrapeSource()` processes the feed,
   **Then** it returns a `ScrapeResult` with 3 `EventInput` structs where:
   - `Name` = SUMMARY value
   - `StartDate` = DTSTART in RFC 3339 format
   - `EndDate` = DTEND in RFC 3339 format
   - `Location.Name` = LOCATION value
   - `Description` = DESCRIPTION value (HTML stripped)
   - `URL` = URL value
   - `Source.URL` = the feed URL

3. **Given** an ICS feed with a VEVENT containing GEO property (latitude;longitude),
   **When** the mapper processes it, **Then** the `EventInput.Location.Latitude` and
   `Location.Longitude` are populated from the GEO values.

4. **Given** an ICS feed with a VEVENT with CATEGORIES "Music,Jazz", **When** the
   mapper processes it, **Then** `EventInput.Keywords` contains `["Music", "Jazz"]`.

5. **Given** an ICS feed URL that returns HTTP 404, **When** `ScrapeSource()` runs,
   **Then** it returns an error wrapping the HTTP status, and no events are ingested.

6. **Given** an ICS feed larger than `MaxBodyBytes` (default 10 MB), **When** the
   fetcher reads it, **Then** it detects the overflow (reads limit+1 bytes) and
   returns an error.

7. **Given** an ICS feed URL that redirects (301/302), **When** `ScrapeSource()`
   runs, **Then** it follows up to 10 redirects (same policy as Tier 3 REST)
   and processes the final response.

8. **Given** an ICS feed URL where the server returns `Content-Type: text/html`,
   **When** `ScrapeSource()` runs, **Then** it logs a warning about unexpected
   Content-Type and attempts to parse anyway (the parse will fail if not ICS).

---

### User Story 2 — Recurring ICS Events Are Expanded to Occurrences (Priority: P1)

An ICS feed contains events with RRULE recurrence rules. The mapper expands these
into individual occurrences within a configurable time horizon.

**Independent Test**: Given an ICS VEVENT with `RRULE:FREQ=WEEKLY;BYDAY=TU;COUNT=10`,
DTSTART on a Tuesday, and 2 EXDATE values, when the mapper runs with HorizonDays=90,
it produces up to 10 EventInputs (minus the 2 excluded dates) with correct start/end
times.

**Acceptance Scenarios**:

1. **Given** a VEVENT with `RRULE:FREQ=WEEKLY;BYDAY=TU` and DTSTART on 2026-04-14
   (Tuesday), **When** the mapper runs with `HorizonDays=90`, **Then** it produces
   ~13 EventInputs (one per Tuesday for 90 days), each with the correct weekday and
   time.

2. **Given** a VEVENT with `RRULE:FREQ=DAILY;COUNT=200` and `MaxOccurrences=100`,
   **When** the mapper runs, **Then** it produces exactly 100 EventInputs (capped)
   and includes a warning about the cap being hit.

3. **Given** a VEVENT with `RRULE:FREQ=WEEKLY;BYDAY=FR` and two EXDATE values
   (2026-04-24 and 2026-05-01), **When** the mapper runs, **Then** those two Fridays
   are excluded from the output.

4. **Given** a VEVENT with RRULE and two RDATE values (extra one-off dates), **When**
   the mapper runs, **Then** the RDATE dates are included as additional occurrences
   alongside the RRULE-generated ones.

5. **Given** a VEVENT with `RRULE:FREQ=MONTHLY;BYMONTHDAY=15`, DTSTART at
   2026-04-15T19:00:00, DTEND at 2026-04-15T21:00:00 (2-hour event), **When** the
   mapper expands occurrences, **Then** each occurrence preserves the 2-hour duration
   (endDate = startDate + 2h).

6. **Given** a non-recurring VEVENT (no RRULE), **When** the mapper processes it,
   **Then** exactly one EventInput is produced with the literal DTSTART/DTEND values.

7. **Given** a VEVENT with `RRULE:FREQ=DAILY` (no COUNT, no UNTIL — infinite),
   **When** the mapper runs with `HorizonDays=90` and `MaxOccurrences=100`, **Then**
   it produces occurrences only within the 90-day window, capped at 100.

---

### User Story 3 — Malformed ICS Is Handled Gracefully (Priority: P1)

Real-world ICS feeds contain malformed events. The parser must be lenient — skip bad
events rather than failing the entire feed.

**Independent Test**: Given an ICS file with 5 VEVENTs where 2 are malformed (missing
SUMMARY, invalid DTSTART), when the parser runs, it returns 3 valid ParsedEvents plus
2 warnings.

**Acceptance Scenarios**:

1. **Given** a VEVENT missing SUMMARY (required), **When** the parser processes it,
   **Then** it is skipped, a warning is recorded with the UID (if present) and
   reason "missing SUMMARY", and processing continues with the next VEVENT.

2. **Given** a VEVENT with an unparseable DTSTART value (e.g. "tomorrow"), **When**
   the parser processes it, **Then** it is skipped with a warning "unparseable
   DTSTART", and processing continues.

3. **Given** a VEVENT with DTSTART but no DTEND and no DURATION, **When** the
   parser processes it, **Then** the event is accepted with `End` set to the same
   value as `Start` (zero-duration event, following RFC 5545 default for DATE-TIME).

4. **Given** a VEVENT with DTSTART and `DURATION:PT2H` but no DTEND, **When** the
   mapper processes it, **Then** `EndDate` = `StartDate + 2 hours`.

5. **Given** a VEVENT with DESCRIPTION containing `<script>alert('xss')</script>`
   and HTML tags, **When** the mapper produces an EventInput, **Then** the
   `Description` field has all HTML tags stripped (via `sanitize.Text()`).

6. **Given** a completely empty VCALENDAR (no VEVENTs), **When** the parser processes
   it, **Then** it returns an empty `ParsedCalendar` with 0 events and no error.

7. **Given** an ICS file that is not valid iCalendar at all (e.g. plain HTML),
   **When** the parser processes it, **Then** it returns an error wrapping the
   parse failure from `arran4/golang-ical`.

8. **Given** a VEVENT with DTEND before DTSTART, **When** the parser processes it,
   **Then** it is skipped with a warning "DTEND before DTSTART (UID: ...)" and
   processing continues with the next VEVENT.

9. **Given** a VEVENT with SUMMARY longer than 500 runes, **When** the parser
   processes it, **Then** the SUMMARY is truncated to 500 runes and a warning
   "SUMMARY truncated" is appended.

10. **Given** two VEVENTs with the same UID and no RECURRENCE-ID, **When** the parser
    processes them, **Then** the first is kept and the second is skipped with a
    warning "duplicate UID".

---

### User Story 4 — ICS Source Is Configured via YAML (Priority: P2)

An operator creates a YAML config file for an ICS source, syncs it to the database,
and the scraper dispatch routes it to the ICS extractor.

**Independent Test**: Given a YAML file with `extraction_method: ics` and `tier: 0`, when
loaded and validated, it passes validation. When synced and scraped, the ICS extractor
is invoked (not the Tier 0 JSON-LD extractor).

**Acceptance Scenarios**:

1. **Given** a YAML config with `extraction_method: ics`, `tier: 0`, and valid fields,
   **When** `ValidateConfig()` runs, **Then** it passes (tier is ignored for ICS
   sources; selectors are not required).

2. **Given** a YAML config with `extraction_method: ics` and `selectors` populated,
   **When** `ValidateConfig()` runs, **Then** it emits a warning "selectors ignored
   for ICS sources" but does not fail.

3. **Given** a synced ICS source in the database, **When** `ScrapeSource()` runs,
   **Then** the dispatch in `scraper.go` routes to the ICS extractor (before the
   tier switch), not to `scrapeTier0`.

4. **Given** a YAML config with `extraction_method: ics` and `default_location` set,
   **When** the mapper produces EventInputs for VEVENTs with no LOCATION, **Then**
   the `default_location` is applied as fallback (same behavior as other source types).

5. **Given** a YAML config with `extraction_method: ics` and `timezone: America/Toronto`,
   **When** a VEVENT has floating-time DTSTART (no timezone suffix, no VTIMEZONE),
   **Then** the mapper interprets it in `America/Toronto`.

---

## Technical Design

### Package Layout

```
internal/
  ical/                         # NEW — ICS parsing, mapping, RRULE expansion
    parse.go                    # VCALENDAR → ParsedCalendar
    parse_test.go
    mapper.go                   # ParsedCalendar → []EventInput
    mapper_test.go
    rrule.go                    # RRULE string → []time.Time occurrences
    rrule_test.go
    doc.go                      # package doc
  scraper/
    ics.go                      # ICS extractor (fetch + parse + map)
    ics_test.go
    config.go                   # MODIFIED — add ExtractionMethod field
    scraper.go                  # MODIFIED — add ICS dispatch before tier switch
tests/
  testdata/
    ics/
      README.md                 # Fixture ownership + naming rules (parse-*, export-*, interop-*)
      parse-basic-event.ics
      parse-multi-event.ics
      parse-recurring-weekly.ics
      parse-recurring-monthly.ics
      parse-malformed.ics
      parse-floating-time.ics
      parse-all-day.ics
      parse-html-description.ics
      parse-reversed-dates.ics
      parse-duplicate-uids.ics
      parse-infinite-rrule.ics
      parse-outlook-vtimezone.ics
      parse-duration-event.ics
      parse-recurrence-id.ics
      parse-empty-calendar.ics
```

### Data Structures

#### ParsedCalendar and ParsedEvent (`internal/ical/parse.go`)

```go
package ical

import "time"

// ParsedCalendar holds the result of parsing a VCALENDAR.
type ParsedCalendar struct {
    ProdID   string        // PRODID value
    Name     string        // X-WR-CALNAME (informal calendar name)
    Source   string        // SOURCE property (RFC 7986)
    Events   []ParsedEvent
    Warnings []string      // Non-fatal parse issues (skipped events, etc.)
}

// ParsedEvent holds a single VEVENT extracted from a VCALENDAR.
type ParsedEvent struct {
    UID          string       // VEVENT UID (used for dedup + provenance)
    Summary      string       // SUMMARY → EventInput.Name
    Description  string       // DESCRIPTION (may contain HTML)
    Location     string       // LOCATION text
    URL          string       // URL property
    Start        time.Time    // DTSTART (resolved to UTC or explicit TZ)
    End          time.Time    // DTEND (zero if absent — caller infers)
    Duration     time.Duration // DURATION property (zero if absent; used when DTEND missing)
    AllDay       bool         // True when DTSTART is DATE (not DATE-TIME)
    RRULE        string       // Raw RRULE string, empty if non-recurring
    RecurrenceID time.Time    // RECURRENCE-ID (zero if absent; identifies exception to RRULE series)
    ExDates      []time.Time  // EXDATE values
    RDates       []time.Time  // RDATE values
    Organizer    string       // ORGANIZER CN parameter (display name)
    OrganizerEmail string     // ORGANIZER mailto: value (email address)
    Categories   []string     // CATEGORIES values (split on comma per value)
    GeoLat       float64      // GEO latitude (0 if absent)
    GeoLon       float64      // GEO longitude (0 if absent)
    HasGeo       bool         // True when GEO property was present (distinguishes "absent" from 0,0/Null Island)
    Created      time.Time    // CREATED timestamp
    LastMod      time.Time    // LAST-MODIFIED timestamp
    Sequence     int          // SEQUENCE (change counter)
    Status       string       // STATUS: CONFIRMED, TENTATIVE, CANCELLED
    RawProps     map[string]string // Other properties preserved for payload
}
```

#### Parse Function

```go
// Parse parses raw ICS bytes into a ParsedCalendar.
// Lenient mode: malformed VEVENTs are skipped with a warning appended to
// ParsedCalendar.Warnings. Only returns error if the overall VCALENDAR
// structure is unparseable.
func Parse(data []byte) (*ParsedCalendar, error)
```

Implementation uses `arran4/golang-ical`:
1. `ics.ParseCalendar(bytes.NewReader(data))`
2. Iterate `cal.Events()`, extract properties via `GetProperty()`
3. Parse DTSTART/DTEND with timezone resolution:
      - TZID param → `time.LoadLocation(tzid)` (IANA names)
      - If `LoadLocation` fails, check `WindowsTZIDAliases` map (top ~20 Windows
        timezone names like "Eastern Standard Time" → "America/New_York"). Outlook
        and Exchange emit these non-IANA TZIDs via VTIMEZONE components.
      - If alias lookup also fails, fall back to UTC and append warning
        "unknown TZID %q, using UTC"
4. Detect all-day events (VALUE=DATE parameter on DTSTART)
5. Handle DURATION property: if DTEND absent but DURATION present, compute
      End = Start + duration. Parse ISO 8601 duration (P1D, PT2H30M, etc.)
6. Extract GEO (format: `lat;lon`), CATEGORIES (comma-separated), ORGANIZER
7. Skip events missing SUMMARY or with unparseable DTSTART → append warning

#### Mapper (`internal/ical/mapper.go`)

```go
package ical

import (
    "github.com/Togather-Foundation/server/internal/domain/events"
)

// MapperOptions controls how parsed ICS events are converted to EventInputs.
type MapperOptions struct {
    SourceURL      string // Feed URL for provenance (→ Source.URL)
    SourceName     string // Source config name (→ Source.Name)
    TrustLevel     int    // Assigned trust level
    License        string // Default "CC0-1.0"
    Timezone       string // Fallback TZ for floating times (IANA name)
    DefaultLocation *events.PlaceInput // Fallback location when VEVENT LOCATION is empty
    HorizonDays    int    // RRULE expansion window (default: 90)
    MaxOccurrences int    // Safety cap on expanded occurrences (default: 100)
    Now            time.Time // Reference time for past-event filtering; zero → time.Now() once at start
}

// MapToEventInputs converts parsed ICS events to SEL EventInputs.
//
// Non-recurring events produce a single EventInput.
// Recurring events (RRULE present) are expanded via ExpandRRule into
// multiple EventInputs, one per occurrence within the horizon window.
// Each expanded occurrence has its own startDate/endDate (preserving
// original duration) and shares all other fields.
//
// Past-event filtering: events (or individual occurrences) whose effective
// end time is at or before the snapshot `now` are silently skipped. The
// effective end is endTime when endTime > startTime, otherwise startTime
// (covers zero-duration and all-day events). The check is applied to:
//   1. Non-recurring events
//   2. Each RRULE-expanded occurrence (after ExpandRRule, which also filters
//      via windowStart; the per-occurrence check is intentionally redundant
//      for zero-duration events and injectable-clock consistency)
//   3. RECURRENCE-ID exceptions — checked against exc.Start/exc.End, not
//      the original occurrence slot (an exception may reschedule to the past)
//   4. RRULE parse-failure fallback: if expansion fails, the event is treated
//      as single non-recurring and is still subject to the past filter.
//
// `now` is snapshotted once at the top of the call from opts.Now (zero → time.Now()).
//
// ctx is used for cancellation during large RRULE expansions.
//
// Returns the EventInputs and any warnings (e.g., RRULE cap hit).
func MapToEventInputs(ctx context.Context, cal *ParsedCalendar, opts MapperOptions) ([]events.EventInput, []string, error)
```

Field mapping:

| ICS Property | EventInput Field | Notes |
|---|---|---|
| SUMMARY | `Name` | Trimmed, HTML-stripped via `sanitize.Text()` |
| DESCRIPTION | `Description` | HTML stripped via `sanitize.Text()` (`internal/sanitize`) |
| LOCATION | `Location.Name` | Falls back to `DefaultLocation` if empty |
| URL | `URL` | Validated as http(s); webcal:// normalized to https:// |
| DTSTART | `StartDate` | RFC 3339 string. **All-day events**: produce `"2026-04-15T00:00:00-04:00"` (midnight in source TZ from `MapperOptions.Timezone`), NOT bare date. `EventInput.StartDate` validation requires full RFC 3339 (`validation.go:596-606`). |
| DTEND | `EndDate` | RFC 3339 string; if absent: use DURATION to compute, else = DTSTART |
| DURATION | `EndDate` | If DTEND absent: `EndDate = StartDate + Duration`. RFC 5545 allows DURATION in lieu of DTEND (WordPress MEC, Google Calendar use this). |
| GEO | `Location.Latitude`, `Location.Longitude` | Parsed from `lat;lon` |
| CATEGORIES | `Keywords` | Split on comma; each CATEGORIES property may have multiple values (RFC 5545 §3.8.1.2 allows multiple CATEGORIES properties per VEVENT, each comma-separated). Concatenate all, filter empty strings. |
| ORGANIZER | `Organizer.Name`, `Organizer.Email` | CN param → Name; mailto: URI → Email. If CN absent, use email local-part as Name. |
| UID | `Source.EventID` | For dedup + provenance. For RRULE-expanded occurrences, use `UID + ":" + startDate` composite (see dedup section). |
| RECURRENCE-ID | (dedup/skip) | Identifies a single-occurrence override of an RRULE series. When expanding RRULE, replace the matching occurrence with the RECURRENCE-ID event's data. |
| STATUS=CANCELLED | Skipped | Log at debug level, do not ingest cancelled events |
| (feed URL) | `Source.URL` | From `MapperOptions.SourceURL` |
| (config) | `Source.Name` | From `MapperOptions.SourceName` |
| (config) | `License` | From `MapperOptions.License` |

#### RRULE Expansion (`internal/ical/rrule.go`)

```go
package ical

import "time"

// RRuleOptions controls RRULE expansion behavior.
type RRuleOptions struct {
    HorizonDays    int       // How far forward to expand (default: 90)
    MaxOccurrences int       // Safety cap (default: 100)
    Now            time.Time // Reference time for window calculation; zero → time.Now()
}

// ExpandRRule expands an RRULE string into concrete occurrence times.
//
// Parameters:
//   - rruleStr: raw RRULE value (e.g. "FREQ=WEEKLY;BYDAY=TU;COUNT=10")
//   - dtstart: the DTSTART of the master event (anchor for the rule)
//   - exdates: EXDATE times to exclude from expansion
//   - rdates: RDATE times to add as extra occurrences
//   - opts: expansion options (horizon, max cap)
//
// Returns occurrence start times in chronological order. The first
// occurrence is dtstart itself (unless excluded by EXDATE).
//
// Uses teambition/rrule-go for RFC 5545 RRULE evaluation.
//
// The bool return is true if the MaxOccurrences cap was applied (result was
// truncated), allowing callers to emit a warning without re-checking slice length.
func ExpandRRule(rruleStr string, dtstart time.Time, exdates, rdates []time.Time, opts RRuleOptions) ([]time.Time, bool, error)
```

#### ICS Extractor (`internal/scraper/ics.go`)

```go
package scraper

import (
    "context"
    "fmt"
    "io"
    "net/http"

    "github.com/Togather-Foundation/server/internal/domain/events"
    "github.com/Togather-Foundation/server/internal/ical"
)

// ICSExtractor fetches and parses an ICS feed URL.
type ICSExtractor struct {
    client       *http.Client
    maxBodyBytes int64 // Default: 10 * 1024 * 1024 (10 MB)
}

// NewICSExtractor creates an ICS extractor with the given HTTP client.
// maxBodyBytes defaults to 10 MB if <= 0.
func NewICSExtractor(client *http.Client, maxBodyBytes int64) *ICSExtractor

// Extract fetches the ICS feed and returns EventInputs.
// Uses the scraper's existing HTTP client (timeout, redirect policy).
// icsConfig provides runtime-configurable ICS settings (HorizonDays, MaxOccurrences,
// MaxBodyBytes) — these are read from the Scraper's icsConfig field, not from SourceConfig.
func (e *ICSExtractor) Extract(ctx context.Context, cfg SourceConfig, icsConfig config.ICSConfig) ([]events.EventInput, []string, error)
```

Implementation:
1. `http.NewRequestWithContext(ctx, "GET", url, nil)` with `Accept: text/calendar`
2. Check response Content-Type: if `text/html`, log warning (likely error page)
3. Read response body up to `maxBodyBytes + 1` via `io.LimitReader`; if all bytes
   consumed, return body-too-large error (overflow detection)
4. `ical.Parse(body)` → `ParsedCalendar`
5. Build `MapperOptions` from `cfg` (SourceURL, SourceName, TrustLevel, License,
   Timezone, DefaultLocation)
6. `ical.MapToEventInputs(ctx, cal, opts)` → `[]EventInput`
7. Return events + collected warnings

**Memory note**: Large feeds (e.g., York University ~6,558 events) are parsed
entirely in memory. The `MaxBodyBytes` limit (10 MB) bounds raw input. After RRULE
expansion, the total `[]EventInput` slice could reach 10,000+ structs (~20-40 MB at
~2 KB/struct). This is manageable for daily runs. `runWithTracking` produces one
`[]EventInput` slice; submission then uses `IngestClient.SubmitBatch`, which chunks
requests to `/api/v1/events:batch` at 100 events per POST. No streaming parser is
needed for Phase 1.

#### SourceConfig Changes (`internal/scraper/config.go`)

```go
// Add to SourceConfig struct:
ExtractionMethod string `yaml:"extraction_method,omitempty" json:"extraction_method,omitempty"`
```

Default value: empty string (treated as `"scraper"` for backward compatibility).
Valid values: `"scraper"`, `"ics"`.

#### Scraper Dispatch Changes (`internal/scraper/scraper.go`)

Insert ICS dispatch before the sitemap check and tier switch in both
`ScrapeSource()` (line ~346) and `ScrapeAll()` (line ~404):

```go
// ICS source type: dispatch to ICS extractor (independent of tier).
if found.ExtractionMethod == "ics" {
    return s.scrapeICS(ctx, *found, opts)
}
```

New method (follows the `runWithTracking` + `scrapeFunc` pattern used by all tiers):

```go
func (s *Scraper) scrapeICS(ctx context.Context, cfg SourceConfig, opts ScrapeOptions) (ScrapeResult, error) {
    result := ScrapeResult{
        SourceName: cfg.Name,
        SourceURL:  cfg.URL,
        Tier:       cfg.Tier,
        DryRun:     opts.DryRun,
    }

    return s.runWithTracking(ctx, &result, func(ctx context.Context) (int, []events.EventInput, []string, error) {
        extractor := NewICSExtractor(opts.HTTPClient(fetchTimeout), s.icsConfig.MaxBodyBytes)
        inputs, warnings, err := extractor.Extract(ctx, cfg)
        if err != nil {
            return 0, nil, nil, err
        }
        return len(inputs), inputs, warnings, nil
    }), nil
}
```

Note: `s.icsConfig` is an `ICSConfig` field added to the `Scraper` struct, wired from
`config.ICSConfig` at construction time. See Configuration section below.

#### Migration (`000042_scraper_sources_extraction_method.up.sql`)

```sql
ALTER TABLE scraper_sources
  ADD COLUMN IF NOT EXISTS extraction_method TEXT NOT NULL DEFAULT '';

ALTER TABLE scraper_sources
  ADD CONSTRAINT scraper_sources_extraction_method_check
  CHECK (extraction_method IN ('', 'scraper', 'ics'));

COMMENT ON COLUMN scraper_sources.extraction_method IS
  'Extraction method: scraper (HTML/JSON-LD/CSS tiers 0-3) or ics (ICS feed). Empty string means default (same as scraper).';

-- Guardrail: do NOT modify provenance sources.source_type in this migration.
-- The provenance column (migration 000002) remains unchanged and serves a
-- different purpose (attribution type, not scraper dispatch mode).
```

Down migration:

```sql
ALTER TABLE scraper_sources
  DROP CONSTRAINT IF EXISTS scraper_sources_extraction_method_check;
ALTER TABLE scraper_sources DROP COLUMN extraction_method;
```

### Error Handling

#### HTTP Fetch Errors

| Error Condition | Behavior |
|---|---|
| Network error (DNS, TLS, connection refused) | Return `fmt.Errorf("ics fetch %s: %w", url, err)` |
| HTTP timeout / context cancelled | Return `fmt.Errorf("ics fetch %s: %w", url, ctx.Err())` |
| HTTP redirect | Follow up to 10 redirects (same policy as Tier 3 REST — `limitRedirects(10)` in `internal/scraper/rest.go:59`). ICS feeds legitimately redirect (Meetup 301→canonical). |
| HTTP 404 | Return `fmt.Errorf("ics fetch %s: HTTP 404 (source not found)", url)` |
| HTTP 5xx | Return `fmt.Errorf("ics fetch %s: HTTP %d (server error)", url, status)` |
| HTTP 204 No Content | Return empty result (0 events), no error |
| Other HTTP non-2xx | Return `fmt.Errorf("ics fetch %s: HTTP %d", url, status)` |
| Body exceeds MaxBodyBytes | Read `MaxBodyBytes + 1` via `io.LimitReader`; if full buffer is consumed, return `fmt.Errorf("ics fetch %s: body exceeds %d bytes", url, max)`. This detects overflow vs exact fit. |
| Response Content-Type is `text/html` | Log warning "expected text/calendar, got text/html — likely error page". Attempt parse anyway (some servers misreport Content-Type). If parse fails, the VCALENDAR unparseable error fires. |

#### Parse Errors

| Error Condition | Behavior |
|---|---|
| VCALENDAR unparseable (not valid iCal) | Return `fmt.Errorf("ics parse: %w", err)` |
| Empty VCALENDAR (0 VEVENTs) | Return empty `ParsedCalendar` with 0 events, no error |
| VEVENT missing SUMMARY | Skip + warning "missing SUMMARY (UID: %s)" |
| VEVENT with unparseable DTSTART | Skip + warning "unparseable DTSTART (UID: %s)" |
| VEVENT with DTSTART but no DTEND/DURATION | Accept: set End = Start (zero-duration, per RFC 5545 default) |
| VEVENT with DURATION but no DTEND | Accept: compute End = Start + Duration |
| VEVENT with DTEND before DTSTART | Skip + warning "DTEND before DTSTART (UID: %s)" |
| VEVENT with STATUS=CANCELLED | Skip with debug-level log (not a warning — cancelled events are expected). Log includes UID for auditability. |
| Duplicate UIDs in feed | Accept both — each UID+RECURRENCE-ID pair is unique per RFC 5545. If same UID appears without RECURRENCE-ID, keep first, skip subsequent + warning. |
| VTIMEZONE with non-IANA TZID | Check `WindowsTZIDAliases` map (e.g. "Eastern Standard Time" → "America/New_York"). If no alias found, fall back to `MapperOptions.Timezone`, then UTC. Append warning "unknown TZID %q, fell back to %s". |
| SUMMARY exceeds 500 runes | Truncate to 500 runes + warning "SUMMARY truncated" |
| DESCRIPTION exceeds 100 KB | Truncate to 100 KB + warning "DESCRIPTION truncated" |
| Non-UTF-8 encoding | `arran4/golang-ical` handles raw bytes; Go strings are UTF-8. Invalid byte sequences survive as replacement characters (U+FFFD). Log warning if detected. |

#### RRULE Expansion Errors

| Error Condition | Behavior |
|---|---|
| Unparseable RRULE string | Skip recurrence, treat as single non-recurring event + warning |
| RRULE with no COUNT and no UNTIL | Expand within `HorizonDays` window only (implicit UNTIL = now + HorizonDays). The `MaxOccurrences` cap is a secondary safety limit. **Implementation note**: use `rrule.Between(now, now+horizon)` rather than `rrule.All()` to avoid materializing unbounded series in memory. |
| RRULE COUNT > MaxOccurrences | Expand up to MaxOccurrences, stop, append warning "RRULE capped at %d occurrences" |
| All occurrences in the past (before now) | Return 0 occurrences for this event. Not an error — event series has ended. |
| All occurrences excluded by EXDATE | Return 0 occurrences. Not an error — append warning for visibility. |
| RRULE timezone mismatch with DTSTART | Use DTSTART's timezone for expansion (RRULE inherits DTSTART's TZ per RFC 5545). |

#### Mapper Errors

| Error Condition | Behavior |
|---|---|
| GEO with invalid format (not `lat;lon`) | Skip GEO data (leave Location coordinates zero), append warning |
| GEO with out-of-range values (lat > 90, lon > 180) | Skip GEO data, append warning |
| URL with non-http(s) scheme (ftp://, etc.) | Skip URL field, append warning |
| URL with webcal:// scheme | Normalize to https:// (strip `webcal://`, replace with `https://`). webcal:// is a defacto standard calendar subscription scheme that maps to HTTPS. |
| CATEGORIES with empty values after split | Filter out empty strings before assigning to Keywords |
| ORGANIZER without CN param | Use email local-part (before @) as Name fallback. If no mailto either, set both Name and Email to empty string. |
| All VEVENTs skipped (none valid) | Return empty result (not error), log warning |

#### Integration Errors

| Error Condition | Behavior |
|---|---|
| Ingest rejects all events (validation) | `runWithTracking` records EventsFailed = N, IngestErrors populated. Not a scraper error — individual ingest rejections are logged per-event. |
| All events deduped | Success result with EventsDuplicate = N, EventsCreated = 0. Not an error. |
| Context cancelled mid-processing | `runWithTracking` returns partial result with error. Partially ingested events remain in DB (ingest is per-event, not transactional). |
| extraction_method YAML/DB mismatch | DB value takes precedence (DB-first config loading). YAML `extraction_method` is synced to DB via `server scrape sync`. If DB says `ics` but YAML says `scraper`, DB wins at runtime. `server scrape sync` overwrites DB with YAML. |

### Security Model

**Trust boundaries**: Same as existing Tier 0-3 scraping. ICS feed content is
untrusted input from external servers.

**Input sanitization**:
- DESCRIPTION: strip HTML tags via `sanitize.Text()` (`internal/sanitize` package)
- SUMMARY: strip HTML via `sanitize.Text()`, truncate to 500 runes
- DESCRIPTION size: truncate to 100 KB before sanitization
- URL/Source URLs: validate http(s) scheme, reject others
- ATTACH properties: ignored entirely (no external fetches)
- Body size: enforce `MaxBodyBytes` via `io.LimitReader` (read limit+1 to detect overflow)
- Property injection: `arran4/golang-ical` handles ICS escaping; we never
  concatenate raw ICS strings

**Defense layers**:
1. HTTP client: redirect-limited (10 hops, same as Tier 3 REST), timeout, body size limit
2. Content-Type check: warn on unexpected Content-Type (still attempt parse)
3. ICS parser: lenient mode, skip malformed events, per-event field length limits
4. Mapper: validate required fields (summary, start date), strip HTML, validate URLs
5. RRULE expansion: HorizonDays window + MaxOccurrences cap (double protection)
6. Ingest pipeline: existing validation, dedup, review queue (unchanged)

**SSRF note**: VEVENT URL values are stored as metadata — the ingest pipeline does
not fetch these URLs. SSRF protection on the ICS feed URL itself is inherited from
the scraper's operator-configured source URLs (YAML/DB). The ICS fetcher uses
`opts.HTTPClient()` which creates a plain `http.Client` without SSRF transport
blocking (unlike `validate_submissions.go` and `rod.go`). This is acceptable for
trusted config URLs but should be revisited if user-submitted ICS URLs are ever
supported.

**Audit trail for parser/mapper warnings**: ICS parse/mapper warnings must be
persisted in `scraper_runs.metadata` for every run (including successful runs),
not just emitted to logs. `runWithTracking` already stores per-event ingest
failures in metadata; extend this to store a bounded warning payload such as:

```json
{
  "ics_warnings": [
    "missing SUMMARY (UID: abc123)",
    "unknown TZID \"Eastern Standard Time\", fell back to America/Toronto"
  ],
  "ics_warning_count": 12,
  "ics_warning_truncated": true
}
```

Cap stored warning strings (e.g., first 200) to keep metadata bounded, while
always preserving full counts for auditability. This supports human/agent review
workflows in admin tooling when feeds contain noisy or malformed data.

### Phase 1 Audit Boundary (Vertical Slice)

**In scope for Phase 1**:
- Persist run-level ICS diagnostics in `scraper_runs.metadata` (warning count,
  bounded warning samples, ingest error summary) for operator triage.
- Ensure diagnostics are available through existing scraper run/admin endpoints,
  without requiring new UI surfaces.
- Keep using existing event/admin review queue flows for post-ingest correction
  of confusing or low-quality data.

**Explicitly out of scope for Phase 1** (later phases):
- New dedicated audit tables for per-event parser/mapper forensic history.
- Cross-run warning analytics (trend dashboards, search, filtering, alerting).
- New remediation workflow UX beyond current admin review queue.

**Acceptance intent**: Phase 1 is complete when operators can answer “what went
wrong in this ICS run?” from persisted run metadata and existing admin tooling,
without introducing a new audit subsystem.

**ICS producer quirks** (tolerated by the lenient parser):
- Google Calendar: `X-GOOGLE-*` extension properties — ignored by default
- Meetup: sometimes omits DTEND for open-ended events → End = Start
- WordPress Tribe: CREATED/LAST-MODIFIED in UTC, DTSTART/DTEND with VTIMEZONE refs
- Tockify: `X-TOCKIFY-*` extension properties — ignored by default
- `arran4/golang-ical` tolerates unrecognized `X-` properties per RFC 5545 §3.8.8.2

**Line folding**: RFC 5545 §3.1 requires content lines longer than 75 octets to be
folded (continuation lines start with a space or tab). `arran4/golang-ical` handles
line folding/unfolding transparently during parsing. Implementers should NOT add
manual line unfolding code.

---

## Implementation Tasks

### Task 1: Add `arran4/golang-ical` and `teambition/rrule-go` dependencies

**Bead**: `srv-0xb3l`
**What**: `go get github.com/arran4/golang-ical` and
`go get github.com/teambition/rrule-go`. Run `go mod tidy`. Verify compilation.
**Test**: `go build ./...` succeeds. Spot-check library API in a throwaway test.
**Acceptance**: `make build` passes with new dependencies in `go.mod` / `go.sum`.

### Task 2: Implement `internal/ical/parse.go` — VCALENDAR parsing

**Bead**: `srv-4pft0`
**What**: Implement `ParsedCalendar`, `ParsedEvent` structs, and `Parse()` function.
Handle VEVENT extraction, timezone resolution (TZID parameter → `time.LoadLocation`,
with Windows TZID alias map fallback for Outlook/Exchange feeds), DURATION property
parsing (compute End from Start + Duration when DTEND absent), RECURRENCE-ID
extraction, all-day detection (VALUE=DATE), GEO parsing, CATEGORIES splitting
(multiple values per property, multiple properties per VEVENT), RRULE/EXDATE/RDATE
preservation, ORGANIZER CN + mailto extraction, lenient skip-on-error with warnings.
Also handle: DTEND-before-DTSTART skip, SUMMARY length truncation (500 runes),
duplicate UID detection.
**Test**: `parse_test.go` with 15 fixtures under `tests/testdata/ics/`:
- `parse-basic-event.ics` — single VEVENT, all fields populated
- `parse-multi-event.ics` — 5 VEVENTs, mixed field presence
- `parse-recurring-weekly.ics` — VEVENT with RRULE + EXDATE
- `parse-recurring-monthly.ics` — VEVENT with RRULE FREQ=MONTHLY + RDATE
- `parse-malformed.ics` — mix of valid and invalid VEVENTs (missing SUMMARY, bad DTSTART)
- `parse-floating-time.ics` — DTSTART without timezone suffix or VTIMEZONE
- `parse-all-day.ics` — VALUE=DATE DTSTART (whole-day events)
- `parse-html-description.ics` — DESCRIPTION with HTML content
- `parse-empty-calendar.ics` — VCALENDAR with zero VEVENTs
- `parse-reversed-dates.ics` — VEVENT with DTEND before DTSTART
- `parse-duplicate-uids.ics` — two VEVENTs with same UID, no RECURRENCE-ID
- `parse-infinite-rrule.ics` — RRULE with no COUNT and no UNTIL
- `parse-outlook-vtimezone.ics` — VTIMEZONE with Windows TZID ("Eastern Standard Time")
- `parse-duration-event.ics` — VEVENT with DURATION instead of DTEND
- `parse-recurrence-id.ics` — RRULE series with RECURRENCE-ID exception override
**Acceptance**: `go test ./internal/ical/...` passes with all fixtures.

### Task 3: Implement `internal/ical/mapper.go` — VEvent → EventInput

**Bead**: `srv-ohinb`
**What**: Implement `MapperOptions` struct and `MapToEventInputs()` function.
Map VEVENT fields to EventInput fields per the field mapping table. Handle:
timezone fallback for floating times, all-day event→RFC 3339 midnight conversion
(must produce full RFC 3339 datetime, not bare date — `validation.go:596-606`
rejects bare dates), DURATION→EndDate computation when DTEND absent, HTML stripping
from DESCRIPTION, GEO→Location, CATEGORIES→Keywords (multi-value, multi-property),
UID→Source.EventID (with `UID:startDate` composite for RRULE-expanded occurrences),
ORGANIZER CN→Name + mailto→Email, RECURRENCE-ID exception merging with RRULE
expansion, STATUS=CANCELLED skip (debug-level log), DefaultLocation fallback, RRULE
expansion via `ExpandRRule()`, webcal:// URL normalization to https://. Also handle:
GEO validation (lat/lon range), URL scheme validation (http(s) only), CATEGORIES
empty-string filtering.
**Test**: `mapper_test.go` covering:
- Basic field mapping (SUMMARY→Name, DTSTART→StartDate, etc.)
- Floating time with timezone fallback
- All-day event produces RFC 3339 midnight in source timezone (e.g. `"2026-04-15T00:00:00-04:00"`)
- HTML stripping from description (via `sanitize.Text()`)
- GEO parsing to Location coordinates
- GEO invalid format (not lat;lon) → skipped with warning
- GEO out-of-range (lat > 90) → skipped with warning
- CATEGORIES to Keywords (multi-value per property, multi-property per event)
- CATEGORIES with trailing comma → empty strings filtered
- CANCELLED events skipped (debug log, no warning)
- DefaultLocation applied when LOCATION is empty
- URL with non-http(s) scheme → skipped with warning
- webcal:// URL → normalized to https://
- ORGANIZER CN→Name, mailto→Email
- DURATION → EndDate when DTEND absent
- RECURRENCE-ID exception replaces matching RRULE occurrence
- Recurring event produces multiple EventInputs with UID:startDate composite Source.EventID
**Acceptance**: `go test ./internal/ical/...` passes.

### Task 4: Implement `internal/ical/rrule.go` — RRULE expansion

**Bead**: `srv-ujz4o`
**What**: Implement `RRuleOptions` struct and `ExpandRRule()` function using
`teambition/rrule-go`. Parse RRULE string, set DTSTART as anchor, expand within
HorizonDays window, apply EXDATE exclusions and RDATE additions, enforce
MaxOccurrences cap.
**Test**: `rrule_test.go` covering:
- FREQ=DAILY with COUNT
- FREQ=WEEKLY with BYDAY
- FREQ=MONTHLY with BYMONTHDAY
- EXDATE exclusion
- RDATE inclusion
- MaxOccurrences cap
- HorizonDays window
- Invalid RRULE string returns error
- No COUNT and no UNTIL → bounded by HorizonDays
- All occurrences in the past → returns empty slice
- Empty RRULE returns just DTSTART
**Acceptance**: `go test ./internal/ical/...` passes.

### Task 5: Implement `internal/scraper/ics.go` — ICS extractor

**Bead**: `srv-c6hz3`
**What**: Implement `ICSExtractor` struct with `Extract()` method. Wire HTTP fetch
using `limitRedirects(10)` (same as Tier 3 REST), body size limit with overflow
detection (read limit+1 bytes), Content-Type warning, `ical.Parse()`,
`ical.MapToEventInputs()`. Add `scrapeICS()` method to `Scraper` struct using the
`runWithTracking` + `scrapeFunc` pattern. Wire dispatch in `ScrapeSource()` and
`ScrapeAll()` — check `cfg.ExtractionMethod == "ics"` before sitemap check and tier switch.
Also:
- Add `icsConfig config.ICSConfig` field to the `Scraper` struct
- Update `NewScraper*` constructor functions to accept and wire `ICSConfig`
- Add `ICS ICSConfig` field to `Config` struct in `internal/config/config.go`
- Wire env vars `ICS_HORIZON_DAYS`, `ICS_MAX_OCCURRENCES`, `ICS_MAX_BODY_BYTES` via
  `getEnvInt` in `config.Load()` (`ICS_MAX_BODY_BYTES` parsed as int then cast to int64)
**Test**: `ics_test.go`:
- Integration test: httptest server serves ICS fixture → `Extract()` returns correct
  EventInputs
- HTTP error handling (404, timeout, body too large)
- HTTP redirect (301 → follows to final URL)
- Content-Type text/html → logs warning, parse fails with clear error
- Empty feed returns empty result (no error)
**Acceptance**: `go test ./internal/scraper/...` passes.

### Task 6: Add `extraction_method` column migration + wire ICS dispatch

**Bead**: `srv-1j6ng`
**What**:
1. Create `migrations/000042_scraper_sources_extraction_method.{up,down}.sql`
   (Migration number 000042 is provisional — verify availability at implementation
   time with `ls internal/storage/postgres/migrations/000042*`. If taken, use next.)
2. Add `ExtractionMethod` field to `SourceConfig` struct with YAML/JSON tags
3. Update `ValidateConfig()` — accept `extraction_method: ics`; skip selector validation
   for ICS sources; warn if selectors provided on ICS source
4. Update `server scrape sync` — read/write `extraction_method` from YAML → DB
5. Update SQLc queries if needed (add `extraction_method` to insert/update/select)

**Breaking-change scope and safety**:
- Treat this as an API/schema-visible additive change for scraper source payloads
  (admin endpoints that serialize `scraper_sources` may expose a new field).
- Do not rename, repurpose, or revalidate provenance `sources.source_type`; that
  column remains owned by provenance flows and existing SEL semantics.
- Add/keep tests that fail if provenance source-type behavior changes while adding
  scraper extraction-method support.
6. Run `make sqlc` to regenerate

**Files affected** (the `extraction_method` field addition cascades through the full
YAML→DB→Go→dispatch pipeline):
- `internal/storage/postgres/migrations/000042_*` — new migration (up + down)
- `internal/scraper/config.go` — add `ExtractionMethod string` to `SourceConfig`
- `internal/domain/scraper/source.go` — add `ExtractionMethod string` to `Source` and
  `UpsertParams` structs
- `internal/scraper/db_source.go` — map `ExtractionMethod` in `SourceConfigFromDomain()`
- `internal/storage/postgres/queries/scraper_sources.sql` — add `extraction_method` to
  INSERT/UPDATE/SELECT queries
- `internal/scraper/db_upsert.go` — wire `extraction_method` when building `UpsertParams`
  from YAML config during `server scrape sync`
- `cmd/server/cmd/scrape_sync.go` — sync command path that calls
  `SourceConfigToUpsertParams`
- SQLc regeneration (`make sqlc`) updates `querier.go` and `*.sql.go`

**Note on naming**: The new `scraper_sources.extraction_method` column avoids
semantic collision with `sources.source_type` (provenance table, migration 000002). The
provenance column tracks *how data entered the SEL* (scraper/api/partner/user/etc.);
the new column tracks *which extraction method the scraper uses* (scraper/ics).
The `COMMENT ON COLUMN` in the migration should explicitly document this distinction.

**OpenAPI/config lint note**: Evaluate whether `ICS_HORIZON_DAYS`,
`ICS_MAX_OCCURRENCES`, `ICS_MAX_BODY_BYTES` need entries in
`internal/config/openapi_lint_test.go`. Likely not — these control scraper internals
and don't affect public API responses. If any admin API endpoint serializes
`scraper_sources` rows (e.g., `/api/v1/admin/scraper/diagnostics`), update
`docs/api/openapi.yaml` to include the `extraction_method` field in the response schema.
**Test**:
- Migration up/down test (existing migration test pattern)
- Config validation: `extraction_method: ics` passes without selectors
- Config validation: `extraction_method: ics` with selectors emits warning
- Sync: YAML with `extraction_method: ics` → DB row with `extraction_method = 'ics'`
**Acceptance**: `make sqlc && make build && go test ./internal/scraper/...` passes.
Migration applies cleanly on local DB.

---

## Configuration

New config fields in `internal/config/config.go`:

```go
// ICSConfig holds defaults for ICS feed ingestion.
type ICSConfig struct {
    HorizonDays    int   `json:"horizon_days"`    // RRULE expansion window. Default: 90. Env: ICS_HORIZON_DAYS
    MaxOccurrences int   `json:"max_occurrences"` // RRULE expansion cap. Default: 100. Env: ICS_MAX_OCCURRENCES
    MaxBodyBytes   int64 `json:"max_body_bytes"`  // Feed size limit. Default: 10485760 (10 MB). Env: ICS_MAX_BODY_BYTES
}
```

Source-level config (YAML):

```yaml
name: "civic-tech-toronto"
extraction_method: ics
url: "https://www.meetup.com/Civic-Tech-Toronto/events/ical/"
schedule: "daily"
trust_level: 5
license: "CC0-1.0"
enabled: true
timezone: "America/Toronto"
```

No new env vars are required for Phase 1 — defaults are sensible. The config fields
exist for operator tuning if needed.

## Success Criteria

1. **End-to-end test**: Create a test ICS source config, run `server scrape source
   <name>`, verify events appear in the database with correct field mapping.
2. **Real feeds**: Successfully ingest 5+ real ICS feeds from the Toronto inventory
   (at least one each of: WordPress Tribe, Meetup, Tockify, Google Calendar).
3. **Recurring events**: A weekly RRULE event produces the expected number of
   occurrences within the 90-day horizon.
4. **Malformed tolerance**: A feed with 50% malformed VEVENTs still ingests the
   valid ones with appropriate warnings.
5. **CI green**: `make ci` passes (build, test, lint, sqlc check).
6. **No regressions**: Existing scraper tests (Tier 0-3) continue to pass unchanged.

## Open Questions

1. **Meetup ICS authentication**: Do Meetup `events/ical/` feeds require
   authentication for private groups? (Likely no for public groups, which are all
   the ones in community-calendar's list. Verify during real-feed testing.)

2. **VTIMEZONE handling** (resolved): Strategy committed — `time.LoadLocation()` for
   IANA TZIDs, `WindowsTZIDAliases` map for top ~20 Windows timezone names
   (Outlook/Exchange), fallback to `MapperOptions.Timezone` then UTC with warning.
   `arran4/golang-ical` parses VTIMEZONE components but does not auto-resolve
   arbitrary TZIDs. The `outlook-vtimezone.ics` testdata fixture covers this path.
   Real-feed testing with WordPress Tribe feeds (which typically include VTIMEZONE)
   will validate the implementation.

3. **Tockify feed pagination**: The torevent Tockify feed has ~2,900 events. Does
   Tockify paginate ICS feeds or return the entire calendar? If paginated, we may
   need to handle `REFRESH-INTERVAL` or date-range parameters. Test with real feed.

## Implementation Deviations

The following deviations from the original spec were made during implementation.
All are intentional design improvements discovered during development.

### 1. `ExpandRRule` Returns `([]time.Time, bool, error)`

The spec defined `ExpandRRule` as returning `([]time.Time, error)`. The implementation
adds a `bool` return indicating whether the `MaxOccurrences` cap was applied
(`capped=true` means the result was truncated). This lets callers emit a warning
without re-checking slice length vs cap, and avoids ambiguity when the rule naturally
produces exactly `MaxOccurrences` results.

### 2. `HasGeo` Field Added to `ParsedEvent`

`ParsedEvent` gains a `HasGeo bool` field not in the original spec. This distinguishes
"GEO property was absent" from "GEO was explicitly 0,0" (the Null Island problem).
The mapper checks `HasGeo` rather than `GeoLat != 0 || GeoLon != 0`.

### 3. `RawProps` Populated via `collectRawProps()`

The spec listed `RawProps map[string]string` but did not detail population logic.
Implementation adds `collectRawProps()` which collects non-standard VEVENT properties
(X-*, etc.) by excluding a `handledProperties` set of already-mapped RFC 5545
properties. Returns `nil` (not empty map) when no extras exist.

### 4. Migration 042 Uses `DEFAULT ''` and Three-Value CHECK

The spec called for `DEFAULT 'scraper'` and `CHECK (extraction_method IN ('scraper', 'ics'))`.
Implementation uses `DEFAULT ''` because existing rows were not created through
explicit scraper extraction — labeling them `'scraper'` would be retrospectively
inaccurate. The CHECK constraint is `CHECK (extraction_method IN ('', 'scraper', 'ics'))`.
The down migration includes `DROP CONSTRAINT` for clean rollback.

### 5. `SetICSConfig()` Setter Pattern Instead of Constructor Changes

Rather than modifying all 4 `NewScraper*` constructor signatures (which would
change every call site), the implementation adds a `SetICSConfig(config.ICSConfig)`
method on `Scraper`, following the existing `SetRodExtractor()` pattern. This is
backwards-compatible with zero call-site changes to existing constructors.

### 6. `Extract()` Takes `config.ICSConfig` as Additional Parameter

`ICSExtractor.Extract()` accepts `config.ICSConfig` alongside `SourceConfig` to
receive runtime-configurable ICS settings (HorizonDays, MaxOccurrences, MaxBodyBytes).
The spec showed `Extract(ctx, cfg SourceConfig)` only.

### 7. `updateRunCompleted` Refactored for ICS Warnings

`updateRunCompleted` in `scraper.go` was refactored to build a combined metadata
JSON map that includes both `event_failures` (existing) and `ics_warnings` /
`ics_warning_count` / `ics_warning_truncated` (new). Warnings are bounded at
`maxStoredWarnings = 200` entries to keep metadata size manageable.

### 8. Staticcheck Fix in `parseISO8601Duration`

An if/else chain in `parseISO8601Duration` was rewritten as a switch statement to
satisfy staticcheck linting. Additionally, bare "P" (no duration components) was
not rejected by the original implementation — fixed to return error.

### 9. Test Fixtures in `tests/testdata/ics/` (Not `internal/ical/testdata/`)

The spec listed `testdata/` under `internal/ical/` as "legacy package-local fixtures
(phase transition)". All 15 fixtures were placed directly in `tests/testdata/ics/`
with a `README.md` documenting fixture ownership and naming conventions.

## Rollback Notes (Phase 1)

- If ICS ingest rollout causes regressions, disable affected sources or set
  `extraction_method='scraper'` for impacted rows while preserving source records.
- If the new migration is the latest applied migration and must be rolled back,
  run `make migrate-down` once, then verify scrape commands still operate in
  scraper-only mode.
- Keep `tests/testdata/ics/` fixtures and parse/mapper tests intact during rollback;
  they are required to validate re-apply safety.
