# ICS Fixture Conventions

Shared fixture root for ICS-related phases: `tests/testdata/ics/`.

## Ownership and Scope

- `parse-*.ics` -> Phase 1 ingest parser/mapper fixtures
- `export-*.ics` -> Phase 2 serializer/export fixtures
- `interop-*.ics` -> Phase 4 integration/interoperability fixtures

Ownership means the phase maintains fixture intent and updates dependent specs/tests
when fixture semantics change.

## Naming Rules

- Format: `<category>-<shape>.ics`
- Use lowercase kebab-case
- Keep names stable once referenced by tests/specs

Examples:
- `parse-basic-event.ics`
- `export-recurring-weekly.ics`
- `interop-community-google-calendar.ics`

## Fixture Quality Rules

- Prefer deterministic content over live captures
- Include comments in neighboring test files, not inside `.ics` payloads
- Avoid secrets/tokens/private URLs
- Keep malformed fixtures intentionally malformed but minimal
