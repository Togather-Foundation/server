# 005 ICS Integration Release Gate Checklist

Phase-agnostic release checklist for shipping the full ICS feature set.

## Functional Gates

- [ ] Race detection tests pass (`make test-ci-race`).
- [ ] Ingest parity verified: representative ICS sources ingest successfully with expected field mapping.
- [ ] Export parity verified: `/api/v1/events.ics` and `/events/{id}.ics` satisfy documented contracts.
- [ ] Recurrence fidelity verified: canonical RRULE/EXDATE/RDATE persistence and export consistency.
- [ ] UID determinism verified across repeated ingest/export cycles.

## Data and Audit Gates

- [ ] Migration audit metadata exists and unmappable recurrence rows are resolved or explicitly accepted.
- [ ] Scraper run metadata includes required warning/error diagnostics for ICS runs.
- [ ] No unresolved data-shape drift between specs and implementation.

## Compatibility Gates

- [ ] Phase 2 + 4 test coverage matches `docs/integration/ics-compatibility-matrix.md`.
- [ ] Strict parser compatibility checks pass.
- [ ] Apple and Google smoke compatibility checks pass.
- [ ] Community-calendar-style interop checks pass.

## Documentation and Operations Gates

- [ ] `docs/integration/event-platforms.md` includes current ICS discovery heuristics.
- [ ] `docs/integration/ics-feeds.md` runbook is current and executable.
- [ ] OpenAPI reflects ICS endpoint/media-type behavior.
- [ ] Rollback notes per phase remain accurate.

## Rollout Readiness Gates

- [ ] Toronto inventory manifest/cohort tracking artifacts exist (Phase 5).
- [ ] First cohort report completed with metrics and follow-up actions.
- [ ] Outstanding blockers are represented in beads and prioritized.
