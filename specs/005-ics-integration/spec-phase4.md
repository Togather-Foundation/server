# Phase 4 Specification: Interop and Operations

**Spec**: 005-ics-integration / Phase 4 | **Date**: 2026-04-13 | **Status**: Draft — PROVISIONAL
**Parent**: `specs/005-ics-integration/plan.md`

> **PROVISIONAL**: This spec captures current thinking but depends on learnings from
> Phases 1-3. Task details and acceptance criteria will be revised when Phase 3 is
> complete. The value of specifying Phase 4 now is to inform earlier phase design —
> particularly: interop fixture shapes guide Phase 1 parser edge-case coverage, and
> platform discovery heuristics may reveal `extraction_method` dispatch requirements
> that should be wired in Phase 1 rather than patched later.
**Goal**: Prove bidirectional ICS interoperability with community-calendar-style feeds and publish operational documentation/runbooks for reliable production use.

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
| Platform recognition knowledge base | Production | `docs/integration/event-platforms.md:12-42` |
| Public events API baseline | Production | `internal/api/router.go:548-557`, `docs/api/openapi.yaml:158-251` |
| Batch ingest endpoint used by scraper | Production | `internal/api/router.go:552-554`, `internal/api/handlers/events.go` |
| Content negotiation behavior | Production | `internal/api/middleware/negotiate.go:21-64` |
| Dedicated ICS integration runbook doc | Missing (Phase 4 deliverable) | — |
| Explicit community-calendar interop test suite | Missing (Phase 4 deliverable) | — |

### What This Phase Delivers

1. End-to-end interop test: community-calendar-style ICS -> SEL ingest.
2. End-to-end interop test: SEL ICS export -> community-calendar-style consumer expectations.
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

### User Story 1 - Ingest Community Calendar Feeds (Priority: P1)

As an operator, I can ingest a representative community-calendar-style ICS feed and get expected events into SEL.

**Independent Test**: Run integration test with curated ICS fixtures matching common external feed patterns (Meetup, Google Calendar, WordPress Tribe, Tockify-like output).

**Acceptance Scenarios**:

1. **Given** a valid external ICS fixture, **When** it is processed via configured `extraction_method: ics` source, **Then** events are ingested and visible through SEL events API.
2. **Given** malformed entries mixed with valid entries, **When** ingest runs, **Then** valid entries succeed and malformed entries are captured in run diagnostics without aborting the full run.
3. **Given** recurring feed items, **When** ingest + recurrence persistence complete, **Then** stored recurrence metadata and generated occurrences are consistent with phase rules.

### User Story 2 - Export SEL to External Consumers (Priority: P1)

As an integrator, I can consume SEL ICS output in a community-calendar-style consumer pipeline.

**Independent Test**: Run integration test that fetches SEL ICS endpoints and validates required ICS properties and recurrence semantics.

**Acceptance Scenarios**:

1. **Given** events in SEL, **When** `/api/v1/events.ics` is fetched, **Then** response is valid `text/calendar` and parseable by strict parser checks.
2. **Given** a recurring event, **When** event ICS is exported, **Then** recurrence properties (`RRULE`, `EXDATE`, `RDATE` where applicable) match canonical storage.
3. **Given** paginated feed export, **When** consumer follows continuation cursor metadata, **Then** all pages are retrievable without contract ambiguity.

### User Story 3 - ICS Source Discovery Guidance (Priority: P2)

As a source-config author, I can identify ICS opportunities quickly and consistently.

**Independent Test**: Review updated `event-platforms.md` with a checklist covering each targeted platform class.

**Acceptance Scenarios**:

1. **Given** a WordPress Tribe site, **When** consulting the guide, **Then** heuristics clearly indicate ICS URL patterns and verification steps.
2. **Given** a Google Calendar or Meetup source, **When** consulting the guide, **Then** expected feed URL formats and caveats are documented.
3. **Given** unsupported or brittle patterns, **When** consulting the guide, **Then** fallback behavior (T0-T3 scraping vs ICS) is explicit.

### User Story 4 - Operational Runbook for ICS Feeds (Priority: P1)

As an operator, I can onboard, validate, monitor, and troubleshoot ICS sources in staging/production.

**Independent Test**: Dry-run the runbook commands in staging against one known-good and one known-problem source.

**Acceptance Scenarios**:

1. **Given** a new ICS source YAML, **When** following runbook steps, **Then** source is synced, scraped, and verified in DB-backed runtime mode.
2. **Given** parser warnings or partial failures, **When** following runbook diagnostics, **Then** operator can locate run metadata and determine next action.
3. **Given** cleanup/retry need, **When** following runbook, **Then** remediation path is documented and safe.

---

## Technical Design

### Package Layout

```text
tests/
  integration/
    ics_interop_ingest_test.go          # NEW - external ICS fixture -> SEL ingest checks
    ics_interop_export_test.go          # NEW - SEL export -> external consumer checks
  testdata/
    ics/
      README.md                         # SHARED - fixture ownership + naming rules
      interop/
        interop-community-meetup.ics     # NEW
        interop-community-google-calendar.ics # NEW
        interop-community-tribe.ics      # NEW
        interop-community-mixed-malformed.ics # NEW
docs/
  integration/
    event-platforms.md                  # MODIFIED - ICS discovery heuristics expansion
    ics-compatibility-matrix.md         # SHARED - consumer/parser compatibility targets
    ics-feeds.md                        # NEW - operational runbook
specs/
  005-ics-integration/
    spec-phase4.md                      # THIS FILE
```

### Data Structures

These are test-internal types used only in Go table-driven tests; JSON serialization
tags are not needed.

```go
// InteropCase defines one end-to-end ICS interop scenario.
type InteropCase struct {
    Name                string
    FixturePath         string
    ExpectedMinEvents   int
    ExpectWarnings      bool
    ExpectRecurrence    bool
}

// ExportExpectation defines assertions against SEL ICS output.
type ExportExpectation struct {
    RequireContentType  string
    RequireCalendarMeta bool
    RequireRRULE        bool
    RequireEXDATE       bool
    RequireRDATE        bool
}
```

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

### Task 1: Add community-calendar -> SEL ingest interop tests

**What**: Build integration tests and representative fixtures to validate external ICS ingestion end-to-end.

**Test**: `scripts/agent-run.sh go test ./tests/integration/ -run ICSInterop` (development);
`scripts/agent-run.sh make test-ci` (CI gate).

**Acceptance**: Tests assert successful ingest, expected warnings, and non-fatal handling
of malformed items. `tests/testdata/ics/README.md` updated with `interop/` subdirectory
naming conventions.

### Task 2: Add SEL -> community-calendar export interop tests

**What**: Add export-focused integration tests validating parseability, recurrence fields, and pagination semantics against `docs/integration/ics-compatibility-matrix.md`.

**Test**: targeted integration test command + full CI suite.

**Acceptance**: Exported ICS satisfies parser checks and recurrence expectations for recurring/non-recurring cases.

### Task 3: Add ICS discovery heuristics section to `docs/integration/event-platforms.md`

**What**: Create a new ICS-discovery section in `event-platforms.md` (currently 898 lines
of scraper-focused content with zero ICS content). Cover ~8 platform fingerprints:
WordPress Tribe (`?ical=1`), WordPress MEC (`?mec-ical-feed=1`), WordPress Events
Manager, Tockify (`tockify.com/api/feeds/ics/<cal>`), Google Calendar (`basic.ics`),
Meetup (`events/ical/`), Eventbrite bridge, and static ICS (`<link rel="alternate"
type="text/calendar">`). Include detection signals, URL patterns, verification steps,
and fallback guidance (when to use T0-T3 scraping instead).

**Test**: manual checklist review + docs lint/format checks used in repo.

**Acceptance**: Discovery steps are explicit enough that another agent can classify source
strategy without guesswork. All 8 platform patterns from plan.md (lines 568-578) covered.

### Task 4: Create `docs/integration/ics-feeds.md` runbook

**What**: Write operational guide covering onboarding, validation, sync, diagnostics, recurrence checks, and troubleshooting.

**Test**: execute documented commands in staging dry-run path and verify no missing prerequisites.

**Acceptance**: Runbook is complete, executable, and references existing operational commands/paths accurately.

### Task 5: Validate `ics-compatibility-matrix.md` coverage in interop tests

**What**: Verify that Phase 4 interop tests (Tasks 1-2) explicitly map to rows in
`docs/integration/ics-compatibility-matrix.md`. Update the matrix if interop testing
reveals gaps or new compatibility data.

**Test**: each matrix row referenced by at least one interop test assertion; `make lint` passes.

**Acceptance**: compatibility matrix is current, every row is tested, and any new
platform/client entries discovered during interop testing are added.

---

## Configuration

No new required environment variables for Phase 4.

Documentation must reference existing config knobs from prior phases (e.g., ICS parser limits) rather than introducing undocumented flags.

---

## Success Criteria

1. Interop ingest tests pass against representative community-calendar-style fixtures.
2. Interop export tests pass and verify recurrence + pagination behavior.
3. Phase 4 interop tests explicitly map to `docs/integration/ics-compatibility-matrix.md` targets.
4. `docs/integration/event-platforms.md` includes clear ICS heuristics and fallback rules.
5. `docs/integration/ics-feeds.md` exists and is executable as an operations runbook.

---

## Open Questions

1. Should Phase 4 include a versioned fixture refresh process for external feed format drift?
2. Should parser-compatibility checks include a second ICS parsing library for stronger interop confidence?

## Rollback Notes (Phase 4)

- If interop tests are unstable due to external drift, pin fixtures and mark volatile
  checks separately while retaining deterministic core contract tests.
- If documentation updates conflict with runtime behavior, revert docs changes to last
  verified state and reopen implementation follow-up beads for contract corrections.
