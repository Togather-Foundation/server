# ICS Fixture Conventions

Shared fixture root for ICS-related phases: `tests/testdata/ics/`.

## Ownership and Scope

- `parse-*.ics` -> Phase 1 ingest parser/mapper fixtures
- `export-*.ics` -> Phase 2 serializer/export fixtures
- `export-recurring-rrule.ics` -> Phase 3 RRULE-mode serializer/export fixture
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

## Regenerating `export-*.ics` Fixtures

`export-*.ics` files are generated from the serializer — do not hand-edit them.
To regenerate after changing `internal/ical/serialize.go`:

```bash
go test ./internal/ical/... -run TestExportFixturesParse -update
```

Review the diff carefully before committing.

Note: `DTSTAMP:` lines are stripped during comparison (they are non-deterministic).

