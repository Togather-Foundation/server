# ICS Compatibility Matrix

This matrix defines the compatibility targets that Phase 2 and Phase 4 tests must
cover.

## Targets

| Target Consumer/Parser | Scope | Must Pass In Phase | Key Assertions |
|---|---|---|---|
| Strict ICS parser (library-based) | Structural correctness | Phase 2, Phase 4 | Valid VCALENDAR/VEVENT structure, escaped text, recurrence properties parse cleanly |
| Apple Calendar import/subscription smoke | Client compatibility | Phase 2, Phase 4 | Feed imports/subscribes, recurring events render, no fatal parse rejection |
| Google Calendar import/subscription smoke | Client compatibility | Phase 2, Phase 4 | Feed imports/subscribes, UTC timestamps render correctly, recurring events preserved |
| Community-calendar-style consumer checks | Interop profile | Phase 4 | Feed endpoints, pagination continuation, recurrence fields, malformed-item tolerance expectations |

## Required Coverage Mapping

- Phase 2 tests must cover:
  - strict parser target
  - Apple + Google smoke targets
- Phase 4 tests must cover:
  - strict parser target
  - community-calendar-style consumer target
  - regression confirmation for Apple + Google smoke targets

## Notes

- This matrix is a contract doc; update it when compatibility targets change.
- Tests should reference matrix rows by name in assertions/comments to keep coverage explicit.
