# Phase 4 Specification: Interop and Operations

**Spec**: 005-ics-integration / Phase 4 | **Date**: 2026-04-14 | **Status**: In Progress
**Parent**: `specs/005-ics-integration/plan.md`

**Goal**: Prove bidirectional ICS interoperability with real external feed shapes and publish operational documentation/runbooks for reliable production use.

## Context

Phase 1-3 deliver ingest, export, and canonical recurrence storage. Phase 4 validates those capabilities against real integration patterns and turns implementation behavior into repeatable operations guidance.

**Test boundary**: Phase 2/3 own contract tests (endpoint behavior, serialization
correctness, pagination mechanics). Phase 4 owns interop tests: realistic external
ICS feed shapes, cross-system round-trip fidelity, consumer expectation validation,
and platform-specific quirks. If a Phase 4 interop test fails, determine whether the
root cause is a Phase 2/3 contract bug (file follow-up there) or an interop gap
(fix here).

### What Exists Today

| Component | Status | Relevant Code/Docs |
|---|---|---|
| Scraper runtime and source workflow docs | Production | `docs/integration/scraper.md:35-100` |
| Platform recognition knowledge base (scraper-focused, no ICS section) | Production | `docs/integration/event-platforms.md:12-42` |
| Public events API baseline | Production | `internal/api/router.go:548-557`, `docs/api/openapi.yaml:158-251` |
| Batch ingest endpoint used by scraper | Production | `internal/api/router.go:552-554`, `internal/api/handlers/events.go` |
| Content negotiation behavior | Production | `internal/api/middleware/negotiate.go:21-64` |
| `extraction_method: ics` dispatch in scraper | Production | `internal/scraper/config.go` — bypasses tier dispatch entirely |
| ICS contract tests (Phase 2: content type, DTSTAMP, pagination, 404/410) | Production | `tests/integration/ics_test.go` |
| ICS fixture library (20 files: `parse-*` and `export-*` shapes) | Production | `tests/testdata/ics/` — `README.md` reserves `interop-*.ics` prefix for Phase 4 |
| ICS compatibility matrix (4 rows: strict parser, Apple, Google, community-calendar) | Production | `docs/integration/ics-compatibility-matrix.md` |
| Dedicated ICS integration runbook doc | Missing (Phase 4 deliverable) | — |
| Interop test suite (external feed shapes → SEL, SEL → consumer expectations) | Missing (Phase 4 deliverable) | — |

### What This Phase Delivers

1. End-to-end interop test: real external ICS feed shapes → SEL ingest.
2. End-to-end interop test: SEL ICS export → external consumer expectations, both `IncludeRRule=false` (default) and `IncludeRRule=true` modes.
3. Updated ICS platform discovery heuristics in `docs/integration/event-platforms.md`.
4. New operations guide: `docs/integration/ics-feeds.md`.

### Non-Goals

- New ingest/extract feature development (belongs to Phase 1-3).
- Parser or extractor behavior changes (create Phase 1 follow-up beads if gaps discovered).
- New recurrence data model changes (belongs to Phase 3).
- New frontend/admin UI for ICS operations.
- Automated source discovery crawler.

### Design Constraint Reminders

- Keep this phase as validation + documentation, not new architecture.
- Interop tests must exercise real API contracts (no test-only side channels).
- Operational guidance must align with existing `server scrape sync` DB-first behavior.
- Any externally visible API clarification must be reflected in `docs/api/openapi.yaml`.

### Out-of-Phase Guardrails

- If interop testing reveals ingest/parser gaps requiring core extractor behavior
  changes, create Phase 1 follow-up beads rather than broadening Phase 4 scope.
- If interop findings require serializer/endpoint contract changes, create Phase 2
  follow-up beads rather than embedding feature redesign in Phase 4.
- If rollout execution planning expands beyond documentation and validation, create
  Phase 5 follow-up beads.

---

## User Scenarios & Testing

### User Story 1 - Ingest External ICS Feed Shapes (Priority: P1)

As an operator, I can ingest representative real-world ICS feed shapes and get expected events into SEL.

**Independent Test**: `scripts/agent-run.sh go test ./tests/integration/ -run ICSInterop` against curated fixtures covering Outlook VTIMEZONE, WordPress Tribe, Google Calendar, Meetup, Tockify, and a malformed-mix feed.

**Acceptance Scenarios**:

1. **Given** an Outlook/Exchange-style ICS with a VTIMEZONE block, **When** processed via `extraction_method: ics`, **Then** events parse with correct UTC times and no timezone-related warnings.
2. **Given** a WordPress Tribe `?ical=1` fixture, **When** ingested, **Then** all valid VEVENTs succeed and the event count matches expected minimum.
3. **Given** a Google Calendar `basic.ics` fixture, **When** ingested, **Then** all valid VEVENTs succeed.
4. **Given** an ICS fixture with RRULE + EXDATE using TZID-local-time (RFC 5545 §3.3.5: no trailing `Z` when TZID is set), **When** ingested, **Then** recurrence metadata is stored and excluded dates are correct — this is a regression guard for the Phase 3 EXDATE format fix.
5. **Given** malformed entries mixed with valid entries, **When** ingest runs, **Then** valid entries succeed and malformed entries are captured in run diagnostics without aborting the full run.
6. **Given** each ingest fixture, **When** the test asserts, **Then** assertions explicitly map to at least one row in `docs/integration/ics-compatibility-matrix.md`.

### User Story 2 - Export SEL to External Consumers (Priority: P1)

As an integrator, I can consume SEL ICS output in both default and recurrence-expanded modes.

**Independent Test**: Integration test fetches `/api/v1/events.ics` (default) and a recurrence-enabled variant, validating required ICS properties and recurrence semantics.

**Acceptance Scenarios**:

1. **Given** events in SEL, **When** `/api/v1/events.ics` is fetched (`IncludeRRule=false`, default), **Then** response is valid `text/calendar`, parseable by strict parser checks, and wire-identical to Phase 2 output (no RRULE/EXDATE/RDATE emitted).
2. **Given** a recurring event with a fully populated `event_series` row (`series_start_date`, `series_end_date`), **When** ICS is exported with `IncludeRRule=true`, **Then** a single VEVENT is emitted with correct RRULE, and EXDATE uses TZID-local-time (no trailing `Z`) when TZID is set — RFC 5545 §3.3.5 regression guard.
3. **Given** a recurring event with `IncludeRRule=true`, **When** the JSON-LD representation is fetched from `/api/v1/events`, **Then** `eventSchedule` fields (`startDate`, `endDate`, `repeatFrequency`) are present.
4. **Given** paginated feed export, **When** consumer follows continuation cursor metadata, **Then** all pages are retrievable without contract ambiguity.
5. **Given** each export assertion, **Then** it explicitly maps to at least one row in `docs/integration/ics-compatibility-matrix.md`.

### User Story 3 - ICS Source Discovery Guidance (Priority: P2)

As a source-config author, I can identify ICS opportunities quickly and consistently.

**Independent Test**: Review updated `event-platforms.md` — verify each of the 8 named platform patterns has: detection signal, ICS URL pattern, verification step, and fallback guidance.

**Acceptance Scenarios**:

1. **Given** a WordPress Tribe site, **When** consulting the guide, **Then** the `?ical=1` URL pattern, feed verification step, and T0-T3 fallback condition are explicit.
2. **Given** a Google Calendar, Meetup, or Tockify source, **When** consulting the guide, **Then** the expected feed URL format, access requirements, and known caveats are documented.
3. **Given** any of the 8 platform patterns (WordPress Tribe `?ical=1`, WordPress MEC `?mec-ical-feed=1`, WordPress Events Manager, Tockify `tockify.com/api/feeds/ics/<cal>`, Google Calendar `basic.ics`, Meetup `events/ical/`, Eventbrite bridge, static `<link rel="alternate" type="text/calendar">`), **Then** a source-config author can determine ICS viability without guesswork.
4. **Given** a site where ICS is available but unreliable or incomplete, **When** consulting the guide, **Then** the fallback to T0-T3 scraping is explicitly described with decision criteria.

### User Story 4 - Operational Runbook for ICS Feeds (Priority: P1)

As an operator, I can onboard, validate, monitor, and troubleshoot ICS sources in staging/production.

**Independent Test**: Trace each runbook command against the existing operational paths in `docs/integration/scraper.md` — verify no missing prerequisites or phantom commands.

**Acceptance Scenarios**:

1. **Given** a new ICS source YAML, **When** following runbook steps, **Then** source is synced via `server scrape sync`, scraped, and verified — using the `extraction_method: ics` config key throughout.
2. **Given** parser warnings or partial failures, **When** following runbook diagnostics, **Then** operator can locate run metadata via existing diagnostic paths (`/api/v1/admin/scraper/diagnostics`) and determine next action.
3. **Given** cleanup/retry need, **When** following runbook, **Then** remediation path is documented and safe.

---

## Technical Design

### Package Layout

```text
tests/
  integration/
    ics_interop_test.go               # NEW — all Phase 4 interop scenarios (ingest + export)
                                      #   consistent with ics_test.go (one file per topic)
  testdata/
    ics/
      README.md                       # MODIFIED — add interop-*.ics naming entries
      interop-outlook-vtimezone.ics   # NEW — Outlook/Exchange VTIMEZONE definition
      interop-tribe-ical.ics          # NEW — WordPress Tribe ?ical=1 output shape
      interop-google-basic.ics        # NEW — Google Calendar basic.ics shape
      interop-meetup.ics              # NEW — Meetup ICS export shape
      interop-tockify.ics             # NEW — Tockify feed shape
      interop-recurrence-exdate.ics   # NEW — RRULE + EXDATE with TZID-local-time (RFC 5545 §3.3.5)
      interop-mixed-malformed.ics     # NEW — valid VEVENTs mixed with malformed entries
docs/
  integration/
    event-platforms.md                # MODIFIED — new ICS discovery heuristics section (~8 platforms)
    ics-compatibility-matrix.md       # MODIFIED — add test references to each row
    ics-feeds.md                      # NEW — operational runbook
specs/
  005-ics-integration/
    spec-phase4.md                    # THIS FILE
```

**Fixture naming convention**: flat under `tests/testdata/ics/` with `interop-` prefix — consistent
with the existing `README.md` reservation and the existing `parse-*` / `export-*` files. Do NOT
create an `interop/` subdirectory.

### Data Structures

These are test-internal types used only in Go table-driven tests; JSON serialization
tags are not needed.

```go
// InteropIngestCase defines one external-ICS → SEL ingest interop scenario.
type InteropIngestCase struct {
    Name              string
    FixturePath       string    // relative to tests/testdata/ics/
    ExpectedMinEvents int
    ExpectWarnings    bool
    ExpectRecurrence  bool
}

// ExportExpectation defines assertions against SEL ICS output.
// IncludeRRule controls which serialization mode is exercised:
//   false (default) → wire-identical to Phase 2 output (no RRULE/EXDATE/RDATE emitted)
//   true            → single VEVENT with RRULE/EXDATE/RDATE where set on the event
// Both modes must be tested for recurring events.
type ExportExpectation struct {
    IncludeRRule       bool   // SerializeOptions.IncludeRRule
    RequireContentType string // must be "text/calendar"
    RequireRRULE       bool   // only when IncludeRRule=true
    RequireEXDATE      bool   // only when IncludeRRule=true and ExDates set
    EXDATENoTrailingZ  bool   // when TZID set: EXDATE must use local time (no Z suffix)
    RequireEventSchedule bool // JSON-LD eventSchedule field present in /events endpoint
}
```

**`RecurrenceRule` struct** (from `internal/domain/events/repository.go`) — used for
fixture setup when testing export interop with recurring events:

```go
type RecurrenceRule struct {
    RRule       string       // e.g. "FREQ=WEEKLY;BYDAY=MO,WE,FR"
    ExDates     []time.Time
    RDates      []time.Time
    TZID        string       // IANA timezone ID; empty → UTC
    SeriesStart *time.Time   // maps to event_series.series_start_date
    SeriesEnd   *time.Time   // maps to event_series.series_end_date
}
```

**`eventSchedule` requirement**: export interop tests that assert the JSON-LD
`eventSchedule` field (via `/api/v1/events`) must insert an `event_series` row with
non-nil `series_start_date`/`series_end_date`. Without these, the API handler omits
`eventSchedule` due to `omitempty`.

### Interfaces

- Reuse existing scrape CLI/API paths from prior phases.
- Reuse existing events read APIs for ingest verification.
- Reuse ICS endpoints introduced in Phase 2 for export validation.

### Test Contract

Interop test contract focuses on observable outcomes:
- ingest success counts,
- warning/error diagnostics presence,
- recurrence fidelity in exported ICS,
- stable media type and pagination semantics.

### Error Handling

- Fixture/setup failures should fail tests immediately with actionable messages.
- Expected malformed feed behavior should assert warnings/partial success, not full-run hard failure.
- Docs validation failures should be treated as test failures when checklists/commands drift.

### Security Model

- Treat all external ICS fixture content as untrusted.
- Do not include secrets or live tokens in fixtures/docs.
- Runbook must include safe guidance for staging/prod keys and diagnostics endpoint usage.
- Interop docs must avoid encouraging unsafe ingest of test domains in production.

---

## Implementation Tasks

### Task 1: Add external ICS feed shape → SEL ingest interop tests

**What**: Build `tests/integration/ics_interop_test.go` and the 7 ICS fixtures listed in
Package Layout. Fixtures must cover: Outlook VTIMEZONE block, WordPress Tribe `?ical=1`
shape, Google Calendar `basic.ics` shape, Meetup export shape, Tockify feed shape,
RRULE+EXDATE with TZID-local-time (RFC 5545 §3.3.5 regression guard), and
valid-mixed-with-malformed. Do NOT duplicate scenarios already in `ics_test.go` (content
type, DTSTAMP, 404/410, pagination — those are Phase 2 contract tests).

Each test assertion must explicitly reference at least one row in
`docs/integration/ics-compatibility-matrix.md`. Update the matrix with test references.

**Test**: `scripts/agent-run.sh go test ./tests/integration/ -run ICSInterop`

**Acceptance**: All 7 fixture shapes ingest cleanly (or assert expected partial-success
for malformed mix). Malformed entries do not abort the run. Recurrence metadata stored
correctly for RRULE+EXDATE fixture. `tests/testdata/ics/README.md` updated with
`interop-*.ics` entries. Each assertion maps to a compatibility matrix row.

### Task 2: Add SEL → external consumer export interop tests

**What**: Add export-focused scenarios to `tests/integration/ics_interop_test.go`.
Test both serialization modes explicitly:
- `IncludeRRule=false` (default): wire-identical to Phase 2; no RRULE/EXDATE/RDATE emitted.
- `IncludeRRule=true`: single VEVENT with RRULE; EXDATE uses TZID-local-time when TZID is
  set (RFC 5545 §3.3.5 — no trailing `Z`).

For `eventSchedule` JSON-LD assertions: insert an `event_series` row with non-nil
`series_start_date`/`series_end_date` — the API handler requires these to emit
`eventSchedule` (omitempty).

Each assertion must map to at least one row in `docs/integration/ics-compatibility-matrix.md`.

**Test**: targeted integration test command + `scripts/agent-run.sh make test-ci`.

**Acceptance**: Both `IncludeRRule` modes tested. EXDATE TZID format assertion present and
passing. `eventSchedule` fields verified in JSON-LD response for recurring event with
populated `event_series`. Compatibility matrix rows all referenced by at least one test.

### Task 3: Add ICS discovery heuristics section to `docs/integration/event-platforms.md`

**What**: Create a new ICS-discovery section in `event-platforms.md` (currently 898 lines
of scraper-focused content with zero ICS content). Cover 8 platform fingerprints with
detection signal, ICS URL pattern, verification step, and fallback criteria:

| Platform | ICS URL pattern |
|---|---|
| WordPress Tribe | `?ical=1` appended to calendar page URL |
| WordPress MEC (Modern Events Calendar) | `?mec-ical-feed=1` |
| WordPress Events Manager | `/?ical=1` or `/events/feed/?ical=1` |
| Tockify | `tockify.com/api/feeds/ics/<calendar-name>` |
| Google Calendar | `calendar.google.com/calendar/ical/<id>/public/basic.ics` |
| Meetup | `meetup.com/<group>/events/ical/` |
| Eventbrite | No native ICS feed; bridge via third-party or scrape T1/T2 |
| Static link | `<link rel="alternate" type="text/calendar" href="...">` in HTML |

Include fallback guidance: when ICS is available but incomplete/unreliable, prefer T1/T2
selectors. Reference `extraction_method: ics` as the config key that activates ICS mode.

**Test**: manual checklist review; `make lint` and `make lint-openapi` pass.

**Acceptance**: All 8 platform patterns documented with URL pattern, detection signal,
verification step, fallback condition. A source-config author can determine ICS viability
without guesswork. `extraction_method: ics` referenced with `server scrape sync` workflow.

### Task 4: Create `docs/integration/ics-feeds.md` runbook

**What**: Write operational guide covering:
1. Source config creation with `extraction_method: ics`
2. Syncing to DB: `server scrape sync` (required — scraper reads from DB, not YAML files)
3. Triggering a scrape run and reading results
4. Diagnostics: `/api/v1/admin/scraper/diagnostics` path
5. Common warning types and remediation (VTIMEZONE missing, UID collisions, EXDATE format)
6. Recurrence sanity checks (verify `event_series` row, `series_start_date`/`series_end_date`)
7. Troubleshooting checklist (source disabled, feed URL changed, auth required)

Reference `docs/integration/scraper.md` for general scraper ops; this runbook is
ICS-specific and complements the general guide.

**Test**: trace each command against existing operational paths — verify no missing
prerequisites or phantom commands; `make lint` passes.

**Acceptance**: Runbook is self-contained and executable. All commands reference real
CLI/API paths. `server scrape sync` and `extraction_method: ics` are central. Staging
admin token usage (from `.agent-keys/staging`) is referenced for diagnostics endpoints.

---

## Configuration

No new required environment variables for Phase 4.

Documentation must reference existing config knobs from prior phases (e.g., ICS parser limits) rather than introducing undocumented flags.

---

## Success Criteria

1. Ingest interop tests pass against 7 representative external ICS fixture shapes.
2. Export interop tests pass for both `IncludeRRule=false` (default) and `IncludeRRule=true` modes, including RFC 5545 §3.3.5 EXDATE TZID format and `eventSchedule` JSON-LD assertions.
3. Every interop test assertion maps to at least one row in `docs/integration/ics-compatibility-matrix.md`; the matrix is updated with test references.
4. `docs/integration/event-platforms.md` includes clear ICS heuristics for all 8 platform patterns with fallback rules.
5. `docs/integration/ics-feeds.md` exists, references `extraction_method: ics` and `server scrape sync`, and is executable as an operations runbook.

---

## Open Questions

1. Should Phase 4 include a versioned fixture refresh process for external feed format drift?
2. Should parser-compatibility checks include a second ICS parsing library for stronger interop confidence?

## Delivery Supplement: ICS Recurring Event Series Dates (srv-bvpkq)

**Bead**: `srv-bvpkq` | **Date**: 2026-04-14 | **Status**: Implemented

**Goal**: Make ICS ingest automatically populate `event_series.series_start_date` and
`event_series.series_end_date` so that `eventSchedule` JSON-LD appears in API responses
for recurring events ingested via ICS feeds.

### What Changed

| File | Change |
|---|---|
| `internal/ical/parse.go` | Added `TZID` field to `ParsedEvent`; `extractTZID()` helper resolves IANA timezone from DTSTART TZID parameter (with Windows alias support) |
| `internal/ical/mapper.go` | `deriveSeriesEnd()` helper parses `UNTIL` from RRULE value string; `MapToEventInputs()` populates `RecurrenceInput` on each expanded occurrence of a recurring event |
| `internal/domain/events/validation.go` | `RecurrenceInput` struct and `Recurrence *RecurrenceInput` field on `EventInput` (tagged `json:"-"`) |
| `internal/domain/events/repository.go` | `UpsertEventSeriesParams`, `UpsertEventSeriesResult` types; `UpsertEventSeries()` method on `Repository` interface |
| `internal/domain/events/create_event_core.go` | Step 10.5: if `validated.Recurrence != nil`, call `dbRepo.UpsertEventSeries()` before `Create()`, then set `params.SeriesID` |
| `internal/storage/postgres/events_repository.go` | `UpsertEventSeries()` implementation with `ON CONFLICT (external_key) DO UPDATE`; `Create()` includes `series_id` in INSERT |
| `internal/storage/postgres/migrations/000045_*` | `ALTER TABLE event_series ADD COLUMN external_key TEXT UNIQUE` |
| `tests/` | Unit: `deriveSeriesEnd`, `RecurrenceInput` population, `TZID` extraction; Integration: `UpsertEventSeries` |

### Architectural Decisions

1. **Series row created inside `createEventCore` transaction** — atomicity for free, no separate lifecycle
2. **`EventInput.Recurrence` is `*RecurrenceInput` with `json:"-"` tag** — zero blast radius on non-ICS paths
3. **`series_end_date` derived from UNTIL in RRULE string** — no expansion needed; `NULL` for COUNT-only or infinite series
4. **`external_key TEXT UNIQUE` on `event_series`** — upsert key is `"source_name:master_uid"`, stable across re-ingests
5. **`series_start_date` truncated to local date using TZID** — not UTC date

### What This Enables

- ICS-recurring events now create `event_series` rows with `series_start_date`, `series_end_date`, `rrule`, `exdates`, `rdates`, and `schedule_timezone` populated
- The `eventSchedule` JSON-LD field in API responses (`/api/v1/events`) is now populated for ICS-sourced recurring events
- The `external_key` column supports idempotent re-ingest: same source + UID → upsert, not duplicate

### Not Yet Done (Phase 5 follow-up)

- Full end-to-end integration test verifying `eventSchedule` in JSON-LD API response after ICS ingest
- Update `spec-phase4.md` Delivery Notes section
- Phase 4 branch landing (`srv-tdhsr`)

## Rollback Notes (Phase 4)

- If interop tests are unstable due to external drift, pin fixtures and mark volatile
  checks separately while retaining deterministic core contract tests.
- If documentation updates conflict with runtime behavior, revert docs changes to last
  verified state and reopen implementation follow-up beads for contract corrections.
