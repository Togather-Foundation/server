# Phase 3 Specification: Recurrence Model Upgrade

**Spec**: 005-ics-integration / Phase 3 | **Date**: 2026-04-13 | **Status**: Draft
**Parent**: `specs/005-ics-integration/plan.md`
**Goal**: Replace legacy repeat fields with canonical RRULE storage so recurring events round-trip cleanly across ingest, JSON-LD, and ICS export.

## Context

Phase 1 introduces ICS ingest and RRULE expansion into occurrence rows. Phase 2 introduces ICS export endpoints. Phase 3 upgrades persistence so recurrence intent is stored canonically (not inferred from legacy repeat columns or derived occurrences only).

### What Exists Today

| Component | Status | Relevant Code |
|---|---|---|
| `event_series` table exists with legacy repeat columns | Production | `internal/storage/postgres/migrations/000001_core.up.sql:104-115` |
| Legacy recurrence columns (`repeat_frequency`, `repeat_on_days`, `repeat_on_dates`) | Production | `internal/storage/postgres/migrations/000001_core.up.sql:112-114` |
| `events.series_id` FK exists | Production | `internal/storage/postgres/migrations/000001_core.up.sql:169` |
| SQLc model still includes legacy repeat fields | Production | `internal/storage/postgres/models.go:187-203` |
| Main event queries are occurrence-centric, not series-centric | Production | `internal/storage/postgres/queries/events.sql:3-25` |
| JSON-LD event output uses `subEvent` occurrences | Production | `internal/jsonld/schema/types.go:28` |
| Event detail repository path does not load series recurrence metadata | Production | `internal/storage/postgres/events_repository.go:278-295` |
| ICS serializer package | Pending from Phase 2 | `internal/ical/serialize.go` (planned) |

### What Phase 3 Delivers

1. Canonical recurrence columns on `event_series`: `rrule`, `exdates`, `rdates`.
2. Migration/backfill strategy from legacy repeat columns to RRULE-based representation.
3. Repository/domain wiring so recurrence metadata can be read with event/series payloads.
4. JSON-LD recurrence projection from RRULE (Schedule-based output), while preserving occurrence compatibility.
5. ICS export upgrade to emit RRULE/EXDATE/RDATE for recurring series.
6. Legacy repeat columns removed after verification.

### Non-Goals (Phase 3)

- Reworking dedup architecture for non-recurring events.
- New recurrence UI/editor in admin frontend.
- Per-consumer recurrence profile customization.
- Non-RFC recurrence expression formats.

### Design Constraint Reminders

- Data migration must be reversible until legacy columns are dropped.
- Preserve existing event list/detail behavior for clients relying on `subEvent`.
- Keep recurrence semantics RFC 5545-aligned (`RRULE`, `EXDATE`, `RDATE`).
- Continue RFC 7807 error behavior for structural failures.

### Out-of-Phase Guardrails

- If work expands into broad interop fixture ecosystem validation, stop and create a
  Phase 4 follow-up bead.
- If work expands into Toronto source onboarding cohorts/reporting, stop and create a
  Phase 5 follow-up bead.
- Do not add new public endpoint families in Phase 3; keep scope to recurrence model,
  projection, and serializer recurrence fidelity.

---

## User Scenarios & Testing

### User Story 1 — Canonical RRULE Persistence (Priority: P1)

As a maintainer, recurrence intent is stored in one canonical format.

**Independent Test**: Migrate a fixture DB containing legacy repeat patterns and verify canonical RRULE fields are populated.

**Acceptance Scenarios**:

1. **Given** an `event_series` row with `repeat_frequency='weekly'` and `repeat_on_days=['MO','WE']`, **When** backfill runs, **Then** `rrule` is populated with equivalent RFC 5545 semantics.
2. **Given** an `event_series` row with monthly day-of-month recurrence (`repeat_on_dates`), **When** backfill runs, **Then** `rrule` carries BYMONTHDAY entries and preserves interval semantics.
3. **Given** unmappable/invalid legacy recurrence data, **When** backfill runs, **Then** row is marked for review and migration does not silently invent recurrence rules.

**Legacy column mapping reference** (audit existing `event_series` data before implementing — legacy columns may be unpopulated since no domain code reads/writes them):

| Legacy Pattern | RRULE Equivalent |
|---|---|
| `repeat_frequency='daily'` | `FREQ=DAILY` |
| `repeat_frequency='weekly'` + `repeat_on_days=['MO','WE']` | `FREQ=WEEKLY;BYDAY=MO,WE` |
| `repeat_frequency='monthly'` + `repeat_on_dates=[1,15]` | `FREQ=MONTHLY;BYMONTHDAY=1,15` |
| `repeat_frequency=NULL` + non-empty `repeat_on_*` | Unmappable (flag for review) |
| Any + invalid values (e.g., `repeat_on_dates` outside 1-31) | Unmappable (flag for review) |

### User Story 2 — JSON-LD Recurrence Projection (Priority: P1)

As an API consumer, I can retrieve recurrence information from canonical RRULE data.

**Independent Test**: Fetch event detail for recurring event and validate Schedule/recurrence projection consistency.

**Acceptance Scenarios**:

1. **Given** a recurring series with RRULE, **When** event JSON-LD detail is requested, **Then** response includes recurrence projection derived from RRULE.
2. **Given** RRULE + EXDATE exceptions, **When** detail is requested, **Then** projected recurrence excludes excepted dates.
3. **Given** existing clients reading `subEvent`, **When** response is returned, **Then** `subEvent` behavior remains backward compatible for the phase.

### User Story 3 — ICS Recurrence Fidelity (Priority: P1)

As an integration consumer, exported ICS preserves recurrence intent without flattening every occurrence.

**Independent Test**: Export recurring event via ICS endpoint and validate RRULE/EXDATE/RDATE fields in VEVENT.

**Acceptance Scenarios**:

1. **Given** a recurring series with canonical RRULE, **When** exported to ICS, **Then** VEVENT includes `RRULE`.
2. **Given** series exceptions, **When** exported, **Then** VEVENT includes `EXDATE`/`RDATE` values matching stored canonical data.
3. **Given** a non-recurring event, **When** exported, **Then** no recurrence properties are emitted.

### User Story 4 — Safe Legacy Column Removal (Priority: P2)

As an operator, schema cleanup does not break runtime behavior.

**Independent Test**: Run migration sequence in staging copy with integration tests before/after drop.

**Acceptance Scenarios**:

1. **Given** no domain code reads legacy repeat columns (verified: columns exist in schema/models only), **When** legacy columns are dropped, **Then** application tests continue passing.
2. **Given** rollback scenario before drop migration is applied, **When** rollback runs, **Then** data remains recoverable from canonical columns and migration artifacts.

---

## Technical Design

### Package Layout

```text
internal/
  storage/postgres/
    migrations/
      0000xx_event_series_rrule.up.sql      # NEW - add rrule/exdates/rdates (+ indexes)
      0000xx_event_series_rrule.down.sql    # NEW
      0000xy_event_series_drop_legacy_repeat.up.sql   # NEW - drop repeat_* columns after verification
      0000xy_event_series_drop_legacy_repeat.down.sql # NEW
    queries/
      events.sql                            # MODIFIED - series recurrence joins/queries as needed
      ...                                   # MODIFIED - any recurrence-aware query set
    models.go                               # GENERATED - picks up new columns after sqlc
  domain/events/
    repository.go                           # MODIFIED - series recurrence model/types
    service.go                              # MODIFIED - recurrence projection plumbing if needed
  jsonld/schema/
    types.go                                # MODIFIED - recurrence projection type(s) (Schedule)
  api/handlers/
    events.go                               # MODIFIED - include recurrence projection in detail output
  ical/
    serialize.go                            # MODIFIED - emit RRULE/EXDATE/RDATE
docs/
  api/openapi.yaml                          # MODIFIED - add eventSchedule recurrence projection to event detail schema
```

### Data Structures

```go
// Domain-level recurrence representation attached to a series/event.
type RecurrenceRule struct {
    RRule   string      // RFC 5545 RRULE string (without leading "RRULE:" prefix in storage)
    ExDates []time.Time // UTC timestamps excluded from recurrence set
    RDates  []time.Time // UTC timestamps explicitly added to recurrence set
    TZID    string      // loaded from existing event_series.schedule_timezone column
                        // (migration 000001, line 115) — no new column needed
}

// EventSeriesRecurrence stores recurrence metadata loaded from event_series.
type EventSeriesRecurrence struct {
    SeriesID string
    Rule     RecurrenceRule
}
```

### Migration Strategy

**Pre-step: Verify legacy data exists.** Before building migration tooling, query
`event_series` in staging to determine whether `repeat_frequency`, `repeat_on_days`,
or `repeat_on_dates` have any non-null values. These columns exist in the schema and
SQLc models but no domain code reads or writes them — they may be entirely empty. If
empty, skip backfill entirely and proceed directly to additive migration + drop.

1. **Additive migration**:
   - add `rrule TEXT`
   - add `exdates TIMESTAMPTZ[] NOT NULL DEFAULT '{}'`
   - add `rdates TIMESTAMPTZ[] NOT NULL DEFAULT '{}'`
2. **Backfill** (only if legacy columns have data):
   - map legacy repeat columns to RRULE equivalents
   - log unmappable rows to server stdout (no dedicated audit table — there are
     likely zero or single-digit rows to handle)
3. **Code cutover**:
   - read canonical recurrence fields
   - stop relying on legacy repeat columns
4. **Cleanup migration**:
   - drop `repeat_frequency`, `repeat_on_days`, `repeat_on_dates`
   - can run immediately after backfill verification since this is pre-launch data
     with no migration-gate ceremony needed

### JSON-LD Projection

- Continue returning occurrence-based `subEvent` for compatibility.
- Add recurrence projection derived from RRULE using explicit schema.org `Schedule` shape:
  - `eventSchedule.@type = "Schedule"`
  - `eventSchedule.repeatFrequency` — RRULE `FREQ` mapped to ISO 8601 duration:

    | RRULE FREQ | ISO 8601 | With INTERVAL=N |
    |---|---|---|
    | DAILY | P1D | P{N}D |
    | WEEKLY | P1W | P{N}W |
    | MONTHLY | P1M | P{N}M |
    | YEARLY | P1Y | P{N}Y |

    FREQ values below DAILY (HOURLY, MINUTELY, SECONDLY) are not expected in event
    data; omit from projection with a warning log.
  - `eventSchedule.byDay` (from RRULE `BYDAY` when present)
  - `eventSchedule.byMonthDay` (from RRULE `BYMONTHDAY` when present)
  - `eventSchedule.startDate` / `eventSchedule.endDate` (from series bounds where present)
  - `eventSchedule.scheduleTimezone` (from canonical TZID)
- Projection is derived at read time from canonical fields, not denormalized text blobs in event payload.

### ICS Serialization

- For recurring series, serializer emits one VEVENT with RRULE (+ EXDATE/RDATE where present).
- For non-recurring events, serializer behavior remains unchanged.
- UID stability follows Phase 1/2 source-event identity constraints.

### Error Handling

- Invalid RRULE in persistence/write path: structural validation error (400/422 for
  API-admin surfaces; internal errors wrapped with `%w`). Validation uses
  `teambition/rrule-go` parse in `internal/ical/rrule.go`, called from the domain
  service write path before persistence.
- Backfill conversion failures: recorded for review; do not crash whole migration unless integrity invariants fail.

### Security Model

- RRULE strings are treated as untrusted input until validated.
- Guard against expansion abuse in any preview/projection paths by enforcing horizon + occurrence caps.
- Avoid exposing raw internal migration diagnostics on public endpoints.

---

## Implementation Tasks

### Task 1: Check legacy data + add canonical recurrence columns to `event_series`

**What**: First, query staging/production `event_series` to determine if `repeat_*`
columns have any non-null data. Then create additive migration for `rrule`, `exdates`,
`rdates` on `event_series`. Regenerate SQLc models.

**Test**: migration up/down in local DB + `make sqlc` + compile.

**Acceptance**: schema supports canonical recurrence fields without breaking existing
runtime queries. If legacy columns are empty, document this and skip Task 2.

### Task 2: Implement legacy -> RRULE backfill path (if data exists)

**What**: Build deterministic conversion from `repeat_*` columns to RRULE/EXDATE/RDATE;
log unmappable rows to stdout. Skip entirely if Task 1 found no legacy data.

**Test**: migration fixture tests covering weekly/monthly/no-repeat and invalid legacy patterns.

**Acceptance**: mapped rows preserve recurrence semantics; unmappable rows are logged
with enough detail to review manually (likely zero or single-digit count).

### Task 3: Wire recurrence through repository/domain models

**What**: Add recurrence-aware query/model plumbing so event detail paths can load canonical recurrence data.

**Test**: repository tests for recurring and non-recurring events.

**Acceptance**: recurrence data available in domain models without regressing existing event reads.

### Task 4: Add JSON-LD recurrence projection from RRULE

**What**: Extend event detail serialization to include Schedule-style recurrence projection derived from canonical fields.

**Test**: handler/schema tests asserting recurrence projection + `subEvent` compatibility.

**Acceptance**: recurring event responses include recurrence projection with correct timezone and exceptions. OpenAPI spec updated with `eventSchedule` property on event detail response schema.

### Task 5: Upgrade ICS serializer to emit RRULE/EXDATE/RDATE

**What**: Update serializer to output recurrence properties for series events.

**Test**: serializer golden tests and endpoint tests for recurring exports.

**Acceptance**: recurring ICS exports preserve canonical rule + exception semantics.

### Task 6: Drop legacy repeat columns

**What**: Add cleanup migration removing `repeat_frequency`, `repeat_on_days`,
`repeat_on_dates` after verifying backfill (or confirming columns were empty).

**Test**: full migration chain test + targeted regression tests for event read/export paths.

**Acceptance**: application behavior unchanged after legacy columns are dropped.

### Task 7: Preserve deterministic VEVENT UID stability across Phase 2 -> Phase 3 cutover

**What**: Ensure Phase 2 UID determinism continues for recurring exports after RRULE cutover.

**Test**: snapshot test that compares VEVENT UID stability for same recurring source across repeated ingest/export cycles.

**Acceptance**: UID generation remains stable across runs and unaffected by legacy-column removal.

---

## Configuration

No new environment variables are required by default in Phase 3.

If RRULE preview/expansion endpoints are introduced during implementation, they must reuse existing recurrence safety limits (horizon and occurrence caps) instead of introducing hardcoded constants.

---

## Success Criteria

1. Canonical recurrence fields are present and populated for migrated series rows (or confirmed empty).
2. JSON-LD recurring event output includes recurrence projection derived from RRULE.
3. ICS recurring export emits RRULE/EXDATE/RDATE accurately.
4. Legacy repeat columns are removed without runtime regressions.
5. Recurring export UID stability is preserved across Phase 2 -> Phase 3 transition.

---

## Open Questions

1. Should RRULE storage include the `RRULE:` prefix or store property value only (recommended: value only)?

## Rollback Notes (Phase 3)

- Legacy repeat columns (`repeat_frequency`, `repeat_on_days`, `repeat_on_dates`)
  exist in schema and SQLc models but are NOT actively read or written by any domain
  code. There is no existing read path to "dual-read" from. The cutover is simply
  "start reading canonical columns."
- If canonical backfill misbehaves, roll back the additive migration. Since legacy
  columns likely have no data (or single-digit rows), the risk is minimal.
- The column-drop migration can follow immediately once tests pass — no elaborate
  audit gate needed for pre-launch data with no active consumers.
