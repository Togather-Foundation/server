# Phase 3 Specification: Recurrence Model Upgrade

**Spec**: 005-ics-integration / Phase 3 | **Date**: 2026-04-13 | **Status**: Delivered
**Parent**: `specs/005-ics-integration/plan.md`
**Goal**: Replace legacy repeat fields with canonical RRULE storage so recurring events round-trip cleanly across ingest, JSON-LD, and ICS export.

## Context

Phase 1 introduces ICS ingest and RRULE expansion into occurrence rows. Phase 2 introduces ICS export endpoints. Phase 3 upgrades persistence so recurrence intent is stored canonically (not inferred from legacy repeat columns or derived occurrences only).

### What Exists Today

| Component | Status | Relevant Code |
|---|---|---|
| `event_series` table | Schema exists, **dormant** — no SQLc queries defined, not joined in any event query | `internal/storage/postgres/migrations/000001_core.up.sql:104-129` |
| Legacy repeat columns (`repeat_frequency TEXT`, `repeat_on_days TEXT[]`, `repeat_on_dates INTEGER[]`) | Schema + SQLc model only — **no domain code reads or writes them** | `internal/storage/postgres/migrations/000001_core.up.sql:112-114`, `models.go:193-195` |
| `events.series_id` FK | Schema only — field is in the DB table but NOT loaded into the domain `Event` struct | `internal/storage/postgres/migrations/000001_core.up.sql:169` |
| SQLc `EventSeries` model | Generated but unused — no query file references `event_series` | `internal/storage/postgres/models.go:187-203` |
| Domain `Event` struct | Occurrence-centric: has `Occurrences []Occurrence` but no `SeriesID` or recurrence fields | `internal/domain/events/repository.go:48-78` |
| JSON-LD `Event` type | Has `SubEvents []EventSummary` for occurrences — **no `eventSchedule`/`Schedule` field** | `internal/jsonld/schema/types.go:7-29` |
| ICS serializer | Delivered (Phase 2) — occurrence-based only, no RRULE output | `internal/ical/serialize.go:50,64` |
| RRULE expansion library | Available in `internal/ical/rrule.go`, **used only for parsing incoming ICS** | `internal/ical/rrule.go:27` |
| Latest migration | `000042_scraper_sources_extraction_method` | `internal/storage/postgres/migrations/` |

### What Phase 3 Delivers

1. Canonical recurrence columns on `event_series`: `rrule`, `exdates`, `rdates` (migration 000043).
2. First SQLc queries for `event_series` — the table is dormant today with no query coverage.
3. Domain `Event` struct gains `SeriesID` and optional `RecurrenceRule` fields.
4. Repository/domain wiring so recurrence metadata is loaded with event detail payloads.
5. JSON-LD recurrence projection from RRULE (`eventSchedule`/Schedule shape), while preserving existing `subEvent` compatibility.
6. ICS export upgrade to emit RRULE/EXDATE/RDATE for recurring series (instead of flattened VEVENTs).
7. Legacy repeat columns removed (migration 000044) — confirmed empty, no backfill needed.

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

**Independent Test**: After migration 000043 runs, create an `event_series` row with
canonical RRULE fields, load it via the repository, and verify the domain struct has
the correct values.

**Acceptance Scenarios**:

1. **Given** migration 000043 has run, **When** an `event_series` row is inserted with
   `rrule='FREQ=WEEKLY;BYDAY=MO,WE'` and `exdates='{2026-05-04T18:00:00Z}'`,
   **Then** the repository returns `RecurrenceRule{RRule: "FREQ=WEEKLY;BYDAY=MO,WE",
   ExDates: [2026-05-04T18:00:00Z]}`.
2. **Given** migration 000044 (legacy column drop) has run, **When** `make sqlc` is
   re-run, **Then** `RepeatFrequency`, `RepeatOnDays`, `RepeatOnDates` are removed from
   the generated `EventSeries` model and all tests still pass.
3. **Given** migration chain is applied then rolled back, **When** `make migrate-down`
   runs migration 000044 down, **Then** legacy columns are restored; `make migrate-down`
   again restores the pre-000043 schema.

> **Pre-confirmed**: `repeat_frequency`, `repeat_on_days`, `repeat_on_dates` have
> never been written by any domain code — no backfill is needed. Task 2 from the
> original plan (backfill) is eliminated. Proceed directly to additive migration + drop.

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

**Independent Test**: Apply migration 000044 in local DB and run full CI tests.

**Acceptance Scenarios**:

1. **Given** legacy repeat columns are confirmed empty and no domain code reads them,
   **When** migration 000044 drops `repeat_frequency`, `repeat_on_days`, `repeat_on_dates`,
   **Then** `make sqlc` succeeds and all CI tests pass without modification.
2. **Given** migration 000044 has been applied, **When** `make migrate-down` rolls it
   back, **Then** legacy columns are restored to the schema (`.down.sql` is valid).

---

## Technical Design

### Package Layout

```text
internal/
  storage/postgres/
    migrations/
      000043_event_series_rrule.up.sql      # NEW - add rrule/exdates/rdates
      000043_event_series_rrule.down.sql    # NEW
      000044_event_series_drop_legacy_repeat.up.sql   # NEW - drop repeat_* columns
      000044_event_series_drop_legacy_repeat.down.sql # NEW
    events_repository.go                    # MODIFIED - GetByULID gains inline LEFT JOIN
                                            # to event_series; scans rrule/exdates/rdates/
                                            # schedule_timezone/series_start_date/
                                            # series_end_date into RecurrenceRule
    models.go                               # GENERATED - picks up new columns after sqlc
  domain/events/
    repository.go                           # MODIFIED - add SeriesID + RecurrenceRule
                                            # to Event struct; add RecurrenceRule type
  jsonld/schema/
    types.go                                # MODIFIED - add Schedule type +
                                            # EventSchedule *Schedule on Event
    recurrence.go                           # NEW - ScheduleFromRecurrence() helper
  api/handlers/
    events.go                               # MODIFIED - populate eventSchedule from
                                            # Recurrence; set startDate/endDate from
                                            # RecurrenceRule.SeriesStart/SeriesEnd
  ical/
    serialize.go                            # MODIFIED - add IncludeRRule to
                                            # SerializeOptions; emit RRULE/EXDATE/RDATE
                                            # when IncludeRRule=true and event has series
docs/
  api/openapi.yaml                          # MODIFIED - add eventSchedule to event
                                            # detail response schema
```

### Data Structures

```go
// internal/domain/events/repository.go

// RecurrenceRule holds canonical recurrence data for an event series.
// RRule is stored and exposed WITHOUT the "RRULE:" prefix — value only
// (e.g. "FREQ=WEEKLY;BYDAY=MO,WE", not "RRULE:FREQ=WEEKLY;BYDAY=MO,WE").
// The serializer adds the "RRULE:" property prefix on wire output.
type RecurrenceRule struct {
    RRule       string      // RFC 5545 RRULE value string (no "RRULE:" prefix)
    ExDates     []time.Time // UTC timestamps excluded from recurrence set (EXDATE)
    RDates      []time.Time // UTC timestamps added to recurrence set (RDATE)
    TZID        string      // IANA timezone from event_series.schedule_timezone
    SeriesStart *time.Time  // series_start_date from event_series (nil if absent)
    SeriesEnd   *time.Time  // series_end_date from event_series (nil if absent)
}

// Event — fields added to the existing struct (internal/domain/events/repository.go):
//   SeriesID   *string          // non-nil when event belongs to a series
//   Recurrence *RecurrenceRule  // non-nil when series has a canonical RRULE
```

```go
// internal/ical/serialize.go — add to existing SerializeOptions

// SerializeOptions controls ICS output formatting and feed metadata.
type SerializeOptions struct {
    CalendarName        string // feed title (default: "Togather Events")
    CalendarDescription string // optional feed description
    // Phase 3:
    IncludeRRule bool   // if true, emit RRULE/EXDATE/RDATE for series events
                       // instead of flattening each occurrence to a VEVENT
}
```

```go
// internal/jsonld/schema/types.go — add Schedule type and eventSchedule field

// Schedule represents schema.org/Schedule for recurring event recurrence metadata.
type Schedule struct {
    AtType          string   `json:"@type"`                    // "Schedule"
    RepeatFrequency string   `json:"repeatFrequency,omitempty"` // ISO 8601 duration (P1W, P1M, etc.)
    ByDay           []string `json:"byDay,omitempty"`           // RRULE BYDAY mapped values
    ByMonthDay      []int    `json:"byMonthDay,omitempty"`      // RRULE BYMONTHDAY
    StartDate       string   `json:"startDate,omitempty"`       // series_start_date
    EndDate         string   `json:"endDate,omitempty"`         // series_end_date (if present)
    ScheduleTimezone string  `json:"scheduleTimezone,omitempty"` // IANA timezone
}

// Add to Event type (internal/jsonld/schema/types.go:7-29):
//   EventSchedule *Schedule `json:"eventSchedule,omitempty"`
```

### Migration Strategy

**Pre-confirmed: legacy columns are empty.** No domain code has ever read or written
`repeat_frequency`, `repeat_on_days`, or `repeat_on_dates`. These columns are confirmed
dead — no backfill is needed. Proceed directly to additive migration then drop.

**Migration 000043** — additive (run first):
```sql
ALTER TABLE event_series
  ADD COLUMN rrule    TEXT,
  ADD COLUMN exdates  TIMESTAMPTZ[] NOT NULL DEFAULT '{}',
  ADD COLUMN rdates   TIMESTAMPTZ[] NOT NULL DEFAULT '{}';
```

**Migration 000044** — drop legacy columns (run after code cutover and tests pass):
```sql
ALTER TABLE event_series
  DROP COLUMN IF EXISTS repeat_frequency,
  DROP COLUMN IF EXISTS repeat_on_days,
  DROP COLUMN IF EXISTS repeat_on_dates;
```

No backfill step exists. The two migrations can run in sequence once the
codebase is updated to use the canonical columns.

### JSON-LD Projection

- Continue returning occurrence-based `subEvent` for compatibility.
- Add recurrence projection derived from RRULE using explicit schema.org `Schedule` shape.
  The projection is split across two locations:
  - `internal/jsonld/schema/recurrence.go` — `ScheduleFromRecurrence()` maps RRULE params
    to the `Schedule` struct: `repeatFrequency` (FREQ → ISO 8601), `byDay` (BYDAY),
    `byMonthDay` (BYMONTHDAY), `scheduleTimezone` (TZID).
  - `internal/api/handlers/events.go` — after calling `ScheduleFromRecurrence()`, the
    handler sets `startDate`/`endDate` from `RecurrenceRule.SeriesStart`/`SeriesEnd`
    (date-only format `2006-01-02`). Falls back to the first occurrence start time for
    `startDate` if `SeriesStart` is nil. This separation keeps the projection helper
    pure (no date arithmetic) and locates series-boundary decisions in the handler.
  - `eventSchedule.@type = "Schedule"`
  - `eventSchedule.repeatFrequency` — RRULE `FREQ` mapped to ISO 8601 duration:

    | RRULE FREQ | ISO 8601 | With INTERVAL=N |
    |---|---|---|
    | DAILY | P1D | P{N}D |
    | WEEKLY | P1W | P{N}W |
    | MONTHLY | P1M | P{N}M |
    | YEARLY | P1Y | P{N}Y |

    FREQ values below DAILY (HOURLY, MINUTELY, SECONDLY) are not expected in event
    data; omit from projection with a `slog.Warn`.
  - `eventSchedule.byDay` (from RRULE `BYDAY` when present)
  - `eventSchedule.byMonthDay` (from RRULE `BYMONTHDAY` when present)
  - `eventSchedule.startDate` / `eventSchedule.endDate` — date-only format (`2006-01-02`),
    populated from `RecurrenceRule.SeriesStart`/`SeriesEnd` in the handler (not in
    `ScheduleFromRecurrence`). `startDate` falls back to the first occurrence start if
    `SeriesStart` is nil; `endDate` is omitted when `SeriesEnd` is nil.
  - `eventSchedule.scheduleTimezone` (from canonical TZID)
- Projection is derived at read time from canonical fields, not denormalized text blobs in event payload.

### ICS Serialization

Phase 2 serializer always flattens occurrences to individual VEVENTs. Phase 3 adds
opt-in RRULE emission via `SerializeOptions.IncludeRRule`:

- **`IncludeRRule = false` (default / backward compatible)**: behavior unchanged —
  one VEVENT per occurrence. Feed and single-event handlers keep existing behavior.
- **`IncludeRRule = true`**: for events with a non-nil `Recurrence` field, emit one
  VEVENT with RRULE (+ EXDATE/RDATE where present) instead of one-per-occurrence.
  Non-recurring events are unaffected. The feed handler passes `IncludeRRule = true`
  when the request includes a future `?recurrence=rrule` query param (exact param name
  TBD at implementation; leave as `false`-default to avoid changing the existing
  stable ICS API contract).

RRULE strings are passed as-is (no `RRULE:` prefix in storage; the library or
serializer adds the property prefix on wire output). Use
`internal/ical/rrule.go`'s `ExpandRRule` for any preview/projection operations — do
not write a second RRULE parser.

UID stability follows the Phase 2 contract:
- Multi-occurrence (flattened): `{event.ULID}-{occurrence.ID}@togather.foundation`
- RRULE-mode single VEVENT: `{event.ULID}@togather.foundation`

### Error Handling

- Invalid RRULE in persistence/write path: structural validation error (400/422 for
  API-admin surfaces; internal errors wrapped with `%w`). Validation uses
  `teambition/rrule-go` parse in `internal/ical/rrule.go`, called from the domain
  service write path before persistence.
- EXDATE/RDATE timestamp format: when `TZID` is set, timestamps are formatted as local
  time without a trailing `Z` suffix (RFC 5545 §3.3.5 — `TZID=` and `Z` are mutually
  exclusive). When no TZID, format as UTC with `Z` suffix.

### Security Model

- RRULE strings are treated as untrusted input until validated.
- Guard against expansion abuse in any preview/projection paths by enforcing horizon + occurrence caps.
- Avoid exposing raw internal migration diagnostics on public endpoints.

---

## Implementation Tasks

### Task 1: Add canonical recurrence columns to `event_series` (migration 000043)

**What**: Create `000043_event_series_rrule.up.sql` adding `rrule TEXT`, `exdates TIMESTAMPTZ[] NOT NULL DEFAULT '{}'`, `rdates TIMESTAMPTZ[] NOT NULL DEFAULT '{}'` to `event_series`. Create matching `.down.sql`. Run `make sqlc` to regenerate models.

**Test**: `make migrate-up` + `make migrate-down` in local DB + `make sqlc` + `make build` (compile only).

**Acceptance**: Schema accepts canonical recurrence fields. `make sqlc` succeeds and `EventSeries` model gains the new fields.

### Task 2: Wire event_series queries and load recurrence into domain `Event`

**What**: Extend the domain `Event` struct (`internal/domain/events/repository.go`) with
`SeriesID *string` and `Recurrence *RecurrenceRule` fields, and add the `RecurrenceRule`
type definition. Update `GetByULID` in the repository to LEFT JOIN `event_series` inline
in the existing SQL query (not via a separate SQLc query file) and scan
`rrule`, `exdates`, `rdates`, `schedule_timezone`, `series_start_date`, `series_end_date`
into `RecurrenceRule`. Populate `RecurrenceRule.SeriesStart`/`SeriesEnd` from
`event_series.series_start_date`/`series_end_date`.

The join is written as inline SQL in `events_repository.go:GetByULID` rather than through
a separate `queries/event_series.sql` SQLc file. This is the first time `event_series`
data is surfaced via the domain layer.

**Test**: Repository unit tests for:
- Event with no series → `SeriesID = nil`, `Recurrence = nil`
- Event with series but no RRULE → `SeriesID` set, `Recurrence = nil`
- Event with series + RRULE → both populated, `SeriesStart`/`SeriesEnd` correct

**Acceptance**: `GetByULID` returns `Recurrence` correctly for recurring events. Existing event query behavior unchanged for non-series events.

### Task 3: Add JSON-LD recurrence projection from RRULE

**What**: Add `Schedule` type and `EventSchedule *Schedule` field to `internal/jsonld/schema/types.go`. Implement FREQ → ISO 8601 duration mapping (see table below). Extend the events detail handler (`internal/api/handlers/events.go`) to populate `EventSchedule` when the domain `Event` carries a non-nil `Recurrence`. Update `docs/api/openapi.yaml` to document the `eventSchedule` property on the event detail schema.

FREQ → `repeatFrequency` mapping:

| RRULE FREQ | ISO 8601 | With INTERVAL=N |
|---|---|---|
| DAILY | P1D | P{N}D |
| WEEKLY | P1W | P{N}W |
| MONTHLY | P1M | P{N}M |
| YEARLY | P1Y | P{N}Y |

HOURLY/MINUTELY/SECONDLY are not expected in event data — omit from projection with a `slog.Warn`.

**Test**: Handler tests asserting `eventSchedule` in JSON-LD response for recurring event; `subEvent` still present for compatibility; non-recurring event has no `eventSchedule`.

**Acceptance**: Recurring event detail includes `eventSchedule` with correct fields. OpenAPI lint passes.

### Task 4: Upgrade ICS serializer to emit RRULE/EXDATE/RDATE

**What**: Add `IncludeRRule bool` to `SerializeOptions`. When `IncludeRRule = true` and the event has `Recurrence != nil`, emit one VEVENT with `RRULE:` + `EXDATE:` + `RDATE:` properties instead of one-per-occurrence. Reuse `internal/ical/rrule.go`'s parser for any roundtrip validation. Golden fixture update: add an `export-recurring-rrule.ics` fixture and update `tests/testdata/ics/README.md`.

**Test**: Serializer tests for:
- RRULE-mode recurring event: single VEVENT with RRULE in wire bytes
- Flattened-mode (default `IncludeRRule = false`): unchanged behavior
- Non-recurring event in RRULE mode: no RRULE emitted

**Acceptance**: RRULE-mode output passes the ical library parse roundtrip and the compatibility matrix targets. `IncludeRRule = false` (default) is wire-identical to Phase 2 output.

### Task 5: Drop legacy repeat columns (migration 000044)

**What**: Create `000044_event_series_drop_legacy_repeat.up.sql` dropping `repeat_frequency`, `repeat_on_days`, `repeat_on_dates`. Create matching `.down.sql`. Run `make sqlc`. Remove the now-absent fields from any test fixtures or integration test assertions that reference them.

**Test**: Full migration chain test (`make migrate-up` then `make migrate-down` × 2) + `scripts/agent-run.sh make test-ci`.

**Acceptance**: No test references `RepeatFrequency`, `RepeatOnDays`, `RepeatOnDates` after the sqlc regeneration. All CI tests pass.

### Task 6: Preserve deterministic VEVENT UID stability across Phase 2 → Phase 3 cutover

**What**: Confirm UID derivation contract is unchanged when switching from flattened-occurrence mode to RRULE mode. Add a snapshot/contract test that exports the same recurring event in both modes and asserts: (a) flattened mode UIDs match Phase 2 golden format, (b) RRULE mode UID is `{event.ULID}@togather.foundation`.

**Test**: snapshot test covering both modes with fixed ULID input.

**Acceptance**: UID generation is stable and not accidentally changed by the Phase 3 serializer changes.

---

## Configuration

No new environment variables are required by default in Phase 3.

If RRULE preview/expansion endpoints are introduced during implementation, they must reuse existing recurrence safety limits from `internal/ical/rrule.go` (`DefaultHorizonDays = 90`, `DefaultMaxOccurrences = 100`) instead of introducing hardcoded constants.

---

## Success Criteria

1. Canonical recurrence columns (`rrule`, `exdates`, `rdates`) are present on `event_series` and loadable via domain queries.
2. Domain `Event` struct carries `SeriesID` and `Recurrence` fields populated by `GetByULID`.
3. JSON-LD recurring event detail includes `eventSchedule` recurrence projection from RRULE.
4. ICS export with `IncludeRRule = true` emits RRULE/EXDATE/RDATE; default mode (false) is wire-identical to Phase 2.
5. Legacy repeat columns are removed without runtime regressions.
6. Recurring export UID stability is preserved across Phase 2 → Phase 3 transition.

---

## Resolved Decisions

1. **RRULE prefix in storage**: store value only, no `RRULE:` prefix
   (e.g. `FREQ=WEEKLY;BYDAY=MO,WE`). Serializer adds the `RRULE:` property
   prefix on wire output.
2. **Legacy column backfill**: eliminated — confirmed empty (no domain code
   ever wrote these columns). Migration 000044 drops them directly.
3. **`IncludeRRule` default**: `false` — existing ICS endpoints remain
   wire-compatible with Phase 2. RRULE mode is opt-in.
4. **`event_series` queries**: recurrence data is loaded via an inline LEFT JOIN in
   `GetByULID` (`events_repository.go`) rather than a separate SQLc query file. A
   `queries/event_series.sql` file was created during T2 but the only query it contained
   (`GetEventSeriesByID`) was never called — the inline approach was cleaner. The file was
   removed and `sqlc` was re-run. The spec's intent (first access to the dormant
   `event_series` table) is satisfied by the inline SQL in `GetByULID`.

## Rollback Notes (Phase 3)

- Migration 000043 is additive (new nullable `rrule` column) — safe to roll back any time before 000044 is applied.
- Migration 000044 drops legacy columns confirmed to be empty — minimal risk. The `.down.sql` restores them for safety.
- If `IncludeRRule = true` causes unexpected client issues, set it back to `false` in the handler — no schema change required.
- OpenAPI changes are additive (`eventSchedule` is `omitempty`) — no client breakage if the field is absent for non-recurring events.

---

## Delivery Reflection

**Completed**: 2026-04-14 | **Branch**: `feat/srv-r71db-ics-phase3-recurrence`
**Commits**: `273612a`, `751fb39`, `fa7164f`, `38b2192`, `8d05474`, `38e7746`

### Success Criteria vs. Delivery

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Canonical `rrule`/`exdates`/`rdates` on `event_series`, loadable via domain queries | ✅ Migration 000043 adds columns; `GetByULID` LEFT JOINs and scans them |
| 2 | `Event` struct carries `SeriesID` and `Recurrence` fields from `GetByULID` | ✅ Both fields added; nil for non-series events |
| 3 | JSON-LD detail includes `eventSchedule` recurrence projection | ✅ `ScheduleFromRecurrence()` in `internal/jsonld/schema/recurrence.go`; 14 unit tests |
| 4 | `IncludeRRule = true` emits RRULE/EXDATE/RDATE; default false is wire-identical to Phase 2 | ✅ 5 new serializer tests; golden fixture `export-recurring-rrule.ics` |
| 5 | Legacy repeat columns removed without regressions | ✅ Migration 000044 drops them; all tests pass |
| 6 | UID stability preserved across Phase 2 → Phase 3 | ✅ `uid_stability_test.go` covers both modes with fixed ULID inputs |

All 6 success criteria met. All 6 spec tasks implemented and reviewed. 4 post-review
fixes committed.

### Deviations and Additions Beyond Spec

1. **`RecurrenceRule` gained `SeriesStart`/`SeriesEnd *time.Time`** — not in spec.
   Necessary to fix `eventSchedule.startDate`/`endDate` boundaries: the spec said
   "series bounds where present" but the initial implementation incorrectly loaded
   occurrence times. The authoritative bounds are `event_series.series_start_date`/
   `series_end_date`; `RecurrenceRule` now carries them from the repository JOIN.

2. **`event_series.sql` query file created then deleted** — the spec called for a
   new `queries/event_series.sql` file to hold SQLc queries for the dormant table.
   An initial `GetEventSeriesByID` query was created in Task 2 but was never called
   (the code used an inline LEFT JOIN in `GetByULID`). Post-review, the dead query
   file was removed and `sqlc` was re-run. The spec's intent — first queries for
   the dormant table — was satisfied by the inline SQL in `GetByULID`.

3. **`eventSchedule.startDate`/`endDate` is date-only format** — spec said "series
   bounds where present" without specifying format. Date-only (`2006-01-02`) is
   correct per schema.org/Schedule, not full RFC3339 timestamps.

4. **RFC 5545 EXDATE/RDATE format bug fixed post-review** — initial T4 implementation
   emitted `TZID=` parameter and trailing `Z` simultaneously (RFC 5545 §3.3.5
   violation). Fixed in commit `8d05474`. Not anticipated in spec.

5. **Dead `negative := true` variable in `recurrence.go`** — minor dead code from
   `parseInt` helper; cleaned up in same fix commit.

6. **Plan Task 2 (backfill) eliminated** — pre-confirmed before spec was written;
   spec correctly excluded it. Delivery confirmed: legacy columns were empty.

### Discovered Follow-up Work

None arising from Phase 3 that requires a new bead. Phase 4 (Interop & Docs) is the
natural next phase per `plan.md`.
