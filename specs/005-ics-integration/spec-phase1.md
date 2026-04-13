# Phase 1 Specification: ICS Ingest

**Spec**: 005-ics-integration / Phase 1 | **Date**: 2026-04-13 | **Status**: Draft
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
| Dedup (SHA-256 + trigram) | Production | `internal/domain/events/ingest.go:558-580` |
| Content negotiation (JSON-LD, JSON, HTML, Turtle) | Production | `internal/api/middleware/negotiate.go:14-19` |
| Source types (Go validation) | Production | `internal/domain/provenance/validation.go:24-29` |
| `scraper_sources` DB table | Production | `migrations/000032_scraper_sources.up.sql` |
| `server scrape source <name>` CLI | Production | `cmd/server/cmd/scrape.go` |
| `server scrape sync` (YAML → DB) | Production | `cmd/server/cmd/scrape.go` |
| Latest migration | `000041` | `migrations/000041_scraper_sources_default_location` |
| ICS parsing library | **None** | — |
| ICS source type | **None** | — |

### What Phase 1 Delivers

1. **`internal/ical/` package** — ICS parsing, VEVENT→EventInput mapping, RRULE
   expansion. New Go package with no SEL-domain dependencies except `events.EventInput`.
2. **ICS extractor** — `internal/scraper/ics.go` wires `ical.Parse()` and
   `ical.MapToEventInputs()` into the scraper pipeline.
3. **`source_type` column** — `scraper_sources.source_type` (`'scraper'` | `'ics'`)
   enables the scraper dispatch to route ICS sources to the ICS extractor.
4. **YAML config support** — `source_type: ics` in source YAML files, synced to DB
   via `server scrape sync`.
5. **CLI integration** — `server scrape source <ics-source>` works end-to-end.

### Non-Goals (Phase 1)

- **ICS export / `text/calendar` responses** — Phase 2
- **`webcal://` subscription feeds** — Phase 2
- **Recurrence model upgrade (RRULE column in `event_series`)** — Phase 3
- **Community-calendar interop testing** — Phase 4
- **Platform discovery heuristics documentation** — Phase 4
- **Scheduled/automated ICS polling** — Phase 1 supports manual `server scrape source`
  and the existing River job scheduler (same as other tiers). No new scheduling infra.
- **ETag/If-Modified-Since caching** — Deferred. The HTTP client will fetch the full
  feed each run. Conditional GET can be added later by storing ETags in
  `scraper_sources` (noted in plan.md Open Questions).

### Design Constraint Reminders

- **SEL compliance**: CC0 license default, source provenance preserved, RFC 7807 errors,
  `fmt.Errorf("...: %w", err)` wrapping.
- **Scraper integration**: ICS sources reuse the existing `scraper_sources` table and
  River scheduling. No new tier number — `source_type = 'ics'` is orthogonal to tiers.
- **Ingest pipeline**: No changes to `IngestService`, validation, dedup, or review
  queue. ICS events enter through the same `EventInput` interface as all other sources.

---

## User Scenarios & Testing

### User Story 1 — ICS Feed Is Fetched and Events Ingested (Priority: P1)

An operator configures an ICS source in YAML, syncs it, and runs the scraper. Events
from the ICS feed appear in the database.

**Independent Test**: Given a YAML config with `source_type: ics` pointing to an
httptest server serving a multi-event ICS file, when `ScrapeSource()` runs, events
are returned in the `ScrapeResult` with correct field mapping.

**Acceptance Scenarios**:

1. **Given** a YAML config `configs/sources/test-ics.yaml` with `source_type: ics`,
   `url: https://example.com/events.ics`, and `trust_level: 5`, **When**
   `server scrape sync` runs, **Then** the source appears in `scraper_sources` with
   `source_type = 'ics'`.

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
   fetcher reads it, **Then** it truncates the read and returns an error.

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

4. **Given** a VEVENT with DESCRIPTION containing `<script>alert('xss')</script>`
   and HTML tags, **When** the mapper produces an EventInput, **Then** the
   `Description` field has all HTML tags stripped (same sanitization as existing
   scraper normalization).

5. **Given** a completely empty VCALENDAR (no VEVENTs), **When** the parser processes
   it, **Then** it returns an empty `ParsedCalendar` with 0 events and no error.

6. **Given** an ICS file that is not valid iCalendar at all (e.g. plain HTML),
   **When** the parser processes it, **Then** it returns an error wrapping the
   parse failure from `arran4/golang-ical`.

---

### User Story 4 — ICS Source Is Configured via YAML (Priority: P2)

An operator creates a YAML config file for an ICS source, syncs it to the database,
and the scraper dispatch routes it to the ICS extractor.

**Independent Test**: Given a YAML file with `source_type: ics` and `tier: 0`, when
loaded and validated, it passes validation. When synced and scraped, the ICS extractor
is invoked (not the Tier 0 JSON-LD extractor).

**Acceptance Scenarios**:

1. **Given** a YAML config with `source_type: ics`, `tier: 0`, and valid fields,
   **When** `ValidateConfig()` runs, **Then** it passes (tier is ignored for ICS
   sources; selectors are not required).

2. **Given** a YAML config with `source_type: ics` and `selectors` populated,
   **When** `ValidateConfig()` runs, **Then** it emits a warning "selectors ignored
   for ICS sources" but does not fail.

3. **Given** a synced ICS source in the database, **When** `ScrapeSource()` runs,
   **Then** the dispatch in `scraper.go` routes to the ICS extractor (before the
   tier switch), not to `scrapeTier0`.

4. **Given** a YAML config with `source_type: ics` and `default_location` set,
   **When** the mapper produces EventInputs for VEVENTs with no LOCATION, **Then**
   the `default_location` is applied as fallback (same behavior as other source types).

5. **Given** a YAML config with `source_type: ics` and `timezone: America/Toronto`,
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
    testdata/                   # ICS fixture files
      basic-event.ics           # Single non-recurring VEVENT
      multi-event.ics           # 5 VEVENTs, mixed fields
      recurring-weekly.ics      # VEVENT with RRULE + EXDATE
      recurring-monthly.ics     # VEVENT with RRULE + RDATE
      malformed.ics             # Mix of valid + invalid VEVENTs
      floating-time.ics         # VEVENTs with floating (no TZ) times
      all-day.ics               # DATE-only DTSTART (all-day events)
      html-description.ics      # DESCRIPTION with HTML content
  scraper/
    ics.go                      # ICS extractor (fetch + parse + map)
    ics_test.go
    config.go                   # MODIFIED — add SourceType field
    scraper.go                  # MODIFIED — add ICS dispatch before tier switch
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
    UID         string       // VEVENT UID (used for dedup + provenance)
    Summary     string       // SUMMARY → EventInput.Name
    Description string       // DESCRIPTION (may contain HTML)
    Location    string       // LOCATION text
    URL         string       // URL property
    Start       time.Time    // DTSTART (resolved to UTC or explicit TZ)
    End         time.Time    // DTEND (zero if absent — caller infers)
    AllDay      bool         // True when DTSTART is DATE (not DATE-TIME)
    RRULE       string       // Raw RRULE string, empty if non-recurring
    ExDates     []time.Time  // EXDATE values
    RDates      []time.Time  // RDATE values
    Organizer   string       // ORGANIZER value (often mailto:)
    Categories  []string     // CATEGORIES values
    GeoLat      float64      // GEO latitude (0 if absent)
    GeoLon      float64      // GEO longitude (0 if absent)
    Created     time.Time    // CREATED timestamp
    LastMod     time.Time    // LAST-MODIFIED timestamp
    Sequence    int          // SEQUENCE (change counter)
    Status      string       // STATUS: CONFIRMED, TENTATIVE, CANCELLED
    RawProps    map[string]string // Other properties preserved for payload
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
3. Parse DTSTART/DTEND with timezone resolution (TZID param → `time.LoadLocation`)
4. Detect all-day events (VALUE=DATE parameter on DTSTART)
5. Extract GEO (format: `lat;lon`), CATEGORIES (comma-separated), ORGANIZER
6. Skip events missing SUMMARY or with unparseable DTSTART → append warning

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
    HorizonDays    int    // RRULE expansion window (default: 90)
    MaxOccurrences int    // Safety cap on expanded occurrences (default: 100)
}

// MapToEventInputs converts parsed ICS events to SEL EventInputs.
//
// Non-recurring events produce a single EventInput.
// Recurring events (RRULE present) are expanded via ExpandRRule into
// multiple EventInputs, one per occurrence within the horizon window.
// Each expanded occurrence has its own startDate/endDate (preserving
// original duration) and shares all other fields.
//
// Returns the EventInputs and any warnings (e.g., RRULE cap hit).
func MapToEventInputs(cal *ParsedCalendar, opts MapperOptions) ([]events.EventInput, []string, error)
```

Field mapping:

| ICS Property | EventInput Field | Notes |
|---|---|---|
| SUMMARY | `Name` | Trimmed |
| DESCRIPTION | `Description` | HTML stripped via `sanitize.Text()` (`internal/sanitize`) |
| LOCATION | `Location.Name` | Falls back to `DefaultLocation` if empty |
| URL | `URL` | Validated as http(s) |
| DTSTART | `StartDate` | RFC 3339 string |
| DTEND | `EndDate` | RFC 3339 string; if absent, = DTSTART |
| GEO | `Location.Latitude`, `Location.Longitude` | Parsed from `lat;lon` |
| CATEGORIES | `Keywords` | Split on comma |
| ORGANIZER | `Organizer.Name` | Extracted from CN param or mailto |
| UID | `Source.EventID` | For dedup + provenance |
| STATUS=CANCELLED | Skipped | Do not ingest cancelled events |
| (feed URL) | `Source.URL` | From `MapperOptions.SourceURL` |
| (config) | `Source.Name` | From `MapperOptions.SourceName` |
| (config) | `License` | From `MapperOptions.License` |

#### RRULE Expansion (`internal/ical/rrule.go`)

```go
package ical

import "time"

// RRuleOptions controls RRULE expansion behavior.
type RRuleOptions struct {
    HorizonDays    int    // How far forward to expand (default: 90)
    MaxOccurrences int    // Safety cap (default: 100)
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
func ExpandRRule(rruleStr string, dtstart time.Time, exdates, rdates []time.Time, opts RRuleOptions) ([]time.Time, error)
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

// NewICSExtractor creates an ICS extractor using the provided HTTP client.
func NewICSExtractor(client *http.Client, maxBodyBytes int64) *ICSExtractor

// Extract fetches the ICS feed and returns EventInputs.
// Uses the scraper's existing HTTP client (timeout, redirect policy).
func (e *ICSExtractor) Extract(ctx context.Context, cfg SourceConfig) ([]events.EventInput, []string, error)
```

Implementation:
1. `http.NewRequestWithContext(ctx, "GET", url, nil)` with `Accept: text/calendar`
2. Read response body up to `maxBodyBytes` via `io.LimitReader`
3. `ical.Parse(body)` → `ParsedCalendar`
4. Build `MapperOptions` from `cfg` (SourceURL, SourceName, TrustLevel, License,
   Timezone, DefaultLocation)
5. `ical.MapToEventInputs(cal, opts)` → `[]EventInput`
6. Return events + collected warnings

#### SourceConfig Changes (`internal/scraper/config.go`)

```go
// Add to SourceConfig struct:
SourceType string `yaml:"source_type,omitempty" json:"source_type,omitempty"`
```

Default value: empty string (treated as `"scraper"` for backward compatibility).
Valid values: `"scraper"`, `"ics"`.

#### Scraper Dispatch Changes (`internal/scraper/scraper.go`)

Insert ICS dispatch before the sitemap check and tier switch in both
`ScrapeSource()` (line ~346) and `ScrapeAll()` (line ~404):

```go
// ICS source type: dispatch to ICS extractor (independent of tier).
if found.SourceType == "ics" {
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
        extractor := NewICSExtractor(opts.HTTPClient(fetchTimeout))
        inputs, warnings, err := extractor.Extract(ctx, cfg)
        if err != nil {
            return 0, nil, nil, err
        }
        return len(inputs), inputs, warnings, nil
    }), nil
}
```

#### Migration (`000042_scraper_sources_source_type.up.sql`)

```sql
ALTER TABLE scraper_sources
  ADD COLUMN source_type TEXT NOT NULL DEFAULT 'scraper'
    CHECK (source_type IN ('scraper', 'ics'));

COMMENT ON COLUMN scraper_sources.source_type IS
  'Source type: scraper (HTML/JSON-LD/CSS tiers 0-3) or ics (ICS feed)';
```

Down migration:

```sql
ALTER TABLE scraper_sources DROP COLUMN source_type;
```

### Error Handling

| Error Condition | Behavior |
|---|---|
| HTTP fetch fails (network, timeout) | Return `fmt.Errorf("ics fetch %s: %w", url, err)` |
| HTTP non-200 status | Return `fmt.Errorf("ics fetch %s: HTTP %d", url, status)` |
| Body exceeds MaxBodyBytes | Return `fmt.Errorf("ics fetch %s: body exceeds %d bytes", url, max)` |
| VCALENDAR unparseable | Return `fmt.Errorf("ics parse: %w", err)` |
| Individual VEVENT malformed | Skip + append warning, continue |
| RRULE unparseable | Skip recurrence, treat as single event + warning |
| All VEVENTs skipped | Return empty result (not error), log warning |
| VEVENT with STATUS=CANCELLED | Skip silently (not a warning) |

### Security Model

**Trust boundaries**: Same as existing Tier 0-3 scraping. ICS feed content is
untrusted input from external servers.

**Input sanitization**:
- DESCRIPTION: strip HTML tags via `sanitize.Text()` (`internal/sanitize` package)
- URL/Source URLs: validate http(s) scheme
- ATTACH properties: ignored entirely (no external fetches)
- Body size: enforce `MaxBodyBytes` via `io.LimitReader`
- Property injection: `arran4/golang-ical` handles ICS escaping; we never
  concatenate raw ICS strings

**Defense layers** (same as existing scraper):
1. HTTP client: no-redirect, timeout, body size limit
2. ICS parser: lenient mode, skip malformed events
3. Mapper: validate required fields (summary, start date), strip HTML
4. Ingest pipeline: existing validation, dedup, review queue (unchanged)

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
Handle VEVENT extraction, timezone resolution (TZID parameter → `time.LoadLocation`),
all-day detection (VALUE=DATE), GEO parsing, CATEGORIES splitting, RRULE/EXDATE/RDATE
preservation, lenient skip-on-error with warnings.
**Test**: `parse_test.go` with 8 fixture files in `testdata/`:
- `basic-event.ics` — single VEVENT, all fields populated
- `multi-event.ics` — 5 VEVENTs, mixed field presence
- `recurring-weekly.ics` — VEVENT with RRULE + EXDATE
- `malformed.ics` — mix of valid and invalid VEVENTs (missing SUMMARY, bad DTSTART)
- `floating-time.ics` — DTSTART without timezone suffix or VTIMEZONE
- `all-day.ics` — VALUE=DATE DTSTART (whole-day events)
- `html-description.ics` — DESCRIPTION with HTML content
- `empty-calendar.ics` — VCALENDAR with zero VEVENTs
**Acceptance**: `go test ./internal/ical/...` passes with all fixtures.

### Task 3: Implement `internal/ical/mapper.go` — VEvent → EventInput

**Bead**: `srv-ohinb`
**What**: Implement `MapperOptions` struct and `MapToEventInputs()` function.
Map VEVENT fields to EventInput fields per the field mapping table. Handle:
timezone fallback for floating times, all-day event date conversion, HTML stripping
from DESCRIPTION, GEO→Location, CATEGORIES→Keywords, UID→Source.EventID, STATUS=CANCELLED
skip, DefaultLocation fallback, RRULE expansion via `ExpandRRule()`.
**Test**: `mapper_test.go` covering:
- Basic field mapping (SUMMARY→Name, DTSTART→StartDate, etc.)
- Floating time with timezone fallback
- All-day event produces date-only startDate
- HTML stripping from description
- GEO parsing to Location coordinates
- CATEGORIES to Keywords
- CANCELLED events skipped
- DefaultLocation applied when LOCATION is empty
- Recurring event produces multiple EventInputs
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
- Empty RRULE returns just DTSTART
**Acceptance**: `go test ./internal/ical/...` passes.

### Task 5: Implement `internal/scraper/ics.go` — ICS extractor

**Bead**: `srv-c6hz3`
**What**: Implement `ICSExtractor` struct with `Extract()` method. Wire HTTP fetch
(existing scraper HTTP client), body size limit, `ical.Parse()`, `ical.MapToEventInputs()`.
Add `scrapeICS()` method to `Scraper` struct. Wire dispatch in `ScrapeSource()` and
`ScrapeAll()` — check `cfg.SourceType == "ics"` before sitemap check and tier switch.
**Test**: `ics_test.go`:
- Integration test: httptest server serves ICS fixture → `Extract()` returns correct
  EventInputs
- HTTP error handling (404, timeout, body too large)
- Empty feed returns empty result (no error)
**Acceptance**: `go test ./internal/scraper/...` passes.

### Task 6: Add `source_type` column migration + wire ICS dispatch

**Bead**: `srv-1j6ng`
**What**:
1. Create `migrations/000042_scraper_sources_source_type.{up,down}.sql`
2. Add `SourceType` field to `SourceConfig` struct with YAML/JSON tags
3. Update `ValidateConfig()` — accept `source_type: ics`; skip selector validation
   for ICS sources; warn if selectors provided on ICS source
4. Update `server scrape sync` — read/write `source_type` from YAML → DB
5. Update SQLc queries if needed (add `source_type` to insert/update/select)
6. Run `make sqlc` to regenerate
**Test**:
- Migration up/down test (existing migration test pattern)
- Config validation: `source_type: ics` passes without selectors
- Config validation: `source_type: ics` with selectors emits warning
- Sync: YAML with `source_type: ics` → DB row with `source_type = 'ics'`
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
source_type: ics
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

2. **VTIMEZONE handling**: `arran4/golang-ical` may or may not resolve VTIMEZONE
   definitions automatically. If it doesn't, we need to extract TZID from the
   VTIMEZONE component and build a `*time.Location` manually. Test with real feeds
   (WordPress Tribe feeds typically include VTIMEZONE).

3. **Tockify feed pagination**: The torevent Tockify feed has ~2,900 events. Does
   Tockify paginate ICS feeds or return the entire calendar? If paginated, we may
   need to handle `REFRESH-INTERVAL` or date-range parameters. Test with real feed.
