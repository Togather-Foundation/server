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

## Interop Fixtures

Phase 4 interop fixtures test real-world ICS feed shape ingestion and export
consumer expectations. Each fixture maps to a row in `docs/integration/ics-compatibility-matrix.md`.

| Fixture | Shape | VEVENTs | Key Properties |
|---|---|---|---|
| `interop-outlook-vtimezone.ics` | Outlook/Exchange with VTIMEZONE block | 2 | `DTSTART;TZID=America/New_York`, STANDARD/DAYLIGHT sub-components |
| `interop-tribe-ical.ics` | WordPress Tribe `?ical=1` | 3 | `X-WR-CALNAME`, `X-WR-TIMEZONE`, `ORGANIZER`, `CATEGORIES`, `X-TRIBE-*` |
| `interop-google-basic.ics` | Google Calendar `basic.ics` | 2 | `DTSTART` in UTC (trailing Z), `STATUS:CONFIRMED`, `DESCRIPTION` with URLs |
| `interop-meetup.ics` | Meetup export | 2 | `URL` with meetup.com pattern, `LOCATION` with full address, `ORGANIZER;CN=` |
| `interop-tockify.ics` | Tockify feed | 2 | `X-PUBLISHED-TTL:PT1H`, `DESCRIPTION` with HTML markup, UTC timestamps |
| `interop-recurrence-exdate.ics` | RFC 5545 §3.3.5 regression guard | 1 | `RRULE:FREQ=WEEKLY;BYDAY=MO,WE`, `EXDATE;TZID=America/Toronto` (no trailing Z) |
| `interop-mixed-malformed.ics` | Valid + malformed mix | 2 valid + 1 missing-DTSTART | One VEVENT missing DTSTART (skipped with warning), two valid VEVENTs succeed |

