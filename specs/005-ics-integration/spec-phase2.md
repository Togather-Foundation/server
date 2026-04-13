# Phase 2 Specification: ICS Export

**Spec**: 005-ics-integration / Phase 2 | **Date**: 2026-04-13 | **Status**: Draft
**Parent**: `specs/005-ics-integration/plan.md`
**Goal**: `GET /api/v1/events.ics` and `GET /api/v1/events/{id}.ics` return valid iCalendar output optimized for agent/API consumers, while remaining compatible with calendar clients.

## Context

Phase 1 introduces ICS ingest (`extraction_method: ics`) and establishes parsing/mapping boundaries. Phase 2 adds the export slice so SEL can act as a calendar publisher, not only an ingest consumer.

### What Exists Today

| Component | Status | Relevant Code |
|---|---|---|
| Public events list API (`/api/v1/events`) | Production | `internal/api/router.go:548`, `internal/api/handlers/events.go:81` |
| Public event detail API (`/api/v1/events/{id}`) | Production | `internal/api/router.go:556`, `internal/api/handlers/events.go:263` |
| Public dereferenceable event page (`/events/{id}`) with HTML/Turtle/JSON-LD negotiation | Production | `internal/api/router.go:573`, `internal/api/handlers/public_pages.go:48`, `internal/api/handlers/public_pages.go:212` |
| Content negotiation helper | Production | `internal/api/middleware/negotiate.go:14-19` |
| Event list filtering and pagination parser | Production | `internal/domain/events/service.go:60-150` |
| Event service list/get methods | Production | `internal/domain/events/service.go:23-33` |
| Event schema output types (JSON-LD) | Production | `internal/jsonld/schema/types.go:6-54` |
| OpenAPI spec (no ICS responses/endpoints yet) | Production | `docs/api/openapi.yaml:99-103`, `docs/api/openapi.yaml:158-251` |
| ICS serializer package | **None (Phase 2 target)** | — |

### What Phase 2 Delivers

1. `internal/ical/serialize.go` with stable ICS serialization for single and multi-event output.
2. `GET /api/v1/events.ics` feed endpoint (filter-aware) for subscription clients.
3. `GET /api/v1/events/{id}.ics` single-event download endpoint.
4. `text/calendar` negotiation support wired into middleware and handlers where applicable.
5. API-first discovery headers for ICS alternates (`Link` header with `rel="alternate"`).
6. OpenAPI updates for ICS endpoints and response content types.

### Non-Goals (Phase 2)

- RRULE-native storage migration in `event_series` (Phase 3).
- Full VTIMEZONE component embedding (defer; UTC output in Phase 2).
- ETag/If-Modified-Since optimization for feed endpoint (defer).
- New admin UI for calendar feed diagnostics.
- New audit subsystem beyond existing scraper/admin review flows.

### Design Constraint Reminders

- Preserve SEL defaults: CC0 license behavior remains unchanged.
- No provenance schema rewrite in Phase 2.
- Keep handlers thin; serialization logic belongs in `internal/ical`.
- Any API surface change must be reflected in `docs/api/openapi.yaml`.
- Prefer agent/API ergonomics over browser UX choices when tradeoffs arise.

### Out-of-Phase Guardrails

- If implementation requires recurrence persistence schema/backfill changes,
  stop and create a Phase 3 follow-up bead.
- If work expands into broad interop fixture validation or operational onboarding
  workflow docs, stop and create a Phase 4 follow-up bead.
- If work expands into Toronto source rollout execution, stop and create a Phase 5
  follow-up bead.

---

## User Scenarios & Testing

### User Story 1 — Pull ICS Feed via API (Priority: P1)

As an agent/integration client, I can pull ICS feed pages and ingest valid calendar events.

**Independent Test**: Given published events in DB, request `GET /api/v1/events.ics` and import result into Apple/Google calendar validators.

**Acceptance Scenarios**:

1. **Given** at least 3 published events with occurrences, **When** `GET /api/v1/events.ics` is requested, **Then** response is `200`, `Content-Type: text/calendar; charset=utf-8`, and body contains one `VEVENT` per exported occurrence.
2. **Given** filter params (`startDate`, `endDate`, `city`, `q`), **When** feed endpoint is requested with those params, **Then** exported events match the same filter semantics as JSON list parsing.
3. **Given** no matching events, **When** feed endpoint is requested, **Then** a valid empty `VCALENDAR` is returned (not 404).
4. **Given** `Accept: text/calendar`, **When** client requests ICS endpoint, **Then** response is calendar content and includes `Vary: Accept`.
5. **Given** more matching events than one page, **When** endpoint is requested with pagination params, **Then** response includes `Link: <...?after=cursor>; rel="next"` header (RFC 8288) and clients can follow it to retrieve the next page.

### User Story 2 — Download Single Event (Priority: P1)

As a user, I can download one event as `.ics` for manual import.

**Independent Test**: Given existing event ULID, request `GET /api/v1/events/{id}.ics`, then import into a desktop calendar app.

**Acceptance Scenarios**:

1. **Given** an event with occurrences, **When** `GET /api/v1/events/{id}.ics` is requested, **Then** response is `200` and contains a `VCALENDAR` with event VEVENT(s) for that event only.
2. **Given** a missing event ULID, **When** single-event ICS endpoint is requested, **Then** response is RFC 7807 `404` (JSON problem response).
3. **Given** a deleted/tombstoned event, **When** single-event ICS endpoint is requested, **Then** response follows existing deleted-resource semantics (410 path).

### User Story 3 — Discover Feed via API Metadata (Priority: P2)

As an API consumer, I can discover ICS alternate URLs from API responses without relying on browser pages.

**Independent Test**: Request event list/detail representations and check response headers for `Link` alternates.

**Acceptance Scenarios**:

1. **Given** `GET /api/v1/events` response, **When** request completes, **Then** it includes `Link: <.../api/v1/events.ics...>; rel="alternate"; type="text/calendar"`.
2. **Given** API event detail response from `GET /api/v1/events/{id}`, **When** request completes, **Then** it includes `Link` alternate to `/api/v1/events/{id}.ics`.

### User Story 4 — Stable Serialization Contract (Priority: P1)

As a maintainer, I can rely on deterministic ICS output from typed domain events.

**Independent Test**: Serializer golden tests over known event fixtures.

**Acceptance Scenarios**:

1. **Given** an event occurrence with start/end + timezone, **When** serialized, **Then** DTSTART/DTEND are emitted in UTC (`Z`) for Phase 2 compatibility.
2. **Given** special characters in name/description/location, **When** serialized, **Then** ICS escaping is correct and file parses successfully.
3. **Given** multi-occurrence event, **When** serialized, **Then** each occurrence maps to distinct VEVENT with stable UID derivation: `UID = "{event.ULID}-{occurrence.ID}@togather.foundation"` for per-occurrence VEVENTs, `UID = "{event.ULID}@togather.foundation"` for single-occurrence events.

---

## Technical Design

### Package Layout

```text
internal/
  ical/
    serialize.go              # NEW - Event -> VCALENDAR/VEVENT serialization
    serialize_test.go         # NEW - unit + golden tests
  api/
    handlers/
      ics.go                  # NEW - /api/v1/events.ics feed + /api/v1/events/{id}.ics download
      events.go               # MODIFIED - add Link alternate headers on list/get
    middleware/
      negotiate.go            # MODIFIED - add text/calendar negotiated type
    router.go                 # MODIFIED - wire new routes
docs/
  api/
    openapi.yaml              # MODIFIED - add ICS endpoints/content types
  integration/
    ics-compatibility-matrix.md # SHARED - compatibility targets used by tests
tests/
  testdata/
    ics/
      README.md               # shared fixture ownership + naming rules
      export-*.ics            # Phase 2 serializer/export fixtures
```

### Data Structures

```go
package ical

import (
    "time"

    "github.com/Togather-Foundation/server/internal/domain/events"
)

// SerializeOptions controls ICS output formatting and feed metadata.
type SerializeOptions struct {
    ProductID    string // e.g. "-//Togather Foundation//SEL//EN"
    CalendarName string // feed title
    CalendarURL  string // canonical feed URL
    Method       string // optional; default "PUBLISH"
    // Phase 3 adds: Timezone string, IncludeRRule bool
}

// SerializeResult contains serialized bytes and derived metadata.
type SerializeResult struct {
    Content      []byte
    EventCount   int
    GeneratedAt  time.Time
}

// SerializeEvents converts domain events to a VCALENDAR payload.
func SerializeEvents(items []events.Event, opts SerializeOptions) (SerializeResult, error)

// SerializeSingleEvent converts one domain event to a VCALENDAR payload.
func SerializeSingleEvent(item *events.Event, opts SerializeOptions) (SerializeResult, error)
```

### Interfaces

Phase 2 reuses existing domain interfaces; no new repository contract is required.

```go
// Existing (internal/domain/events/service.go)
func (s *Service) List(ctx context.Context, filters Filters, pagination Pagination) (ListResult, error)
func (s *Service) GetByULID(ctx context.Context, ulid string) (*Event, error)
```

Handler strategy for feed endpoint:
- Parse filters with existing `events.ParseFilters(...)` for semantic parity.
- Use cursor pagination for feed generation (`limit`/`after`) with the same bounds as existing list APIs.
- Use `List()` results directly for serialization — `ListResult.Events` already contains full `Event` objects with populated `Occurrences`. Do NOT call `GetByULID` per item (N+1 anti-pattern). `GetByULID` is used only by the single-event handler (`/events/{id}.ics`).

### Endpoint Contracts

1) **GET `/api/v1/events.ics`**
- Auth: public
- Query params: same filter set as `/api/v1/events` plus `after`/`limit`
- Response `200`: `text/calendar; charset=utf-8`
- Headers:
  - `Content-Type: text/calendar; charset=utf-8`
  - `Content-Disposition: attachment; filename="events.ics"`
  - `Link: </api/v1/events.ics?after={cursor}&limit={n}>; rel="next"` when more
    results are available (RFC 8288 — standard pagination for binary/opaque response
    bodies that cannot embed pagination metadata; cursor value is the same opaque
    token used by the JSON list endpoint)
  - `Link` alternates for JSON-LD endpoint

2) **GET `/api/v1/events/{id}.ics`**
- Auth: public
- Response `200`: `text/calendar; charset=utf-8`
- Headers:
  - `Content-Disposition: attachment; filename="event-{id}.ics"`
- Errors: existing not-found/gone semantics via problem responses.

### API Discovery via Link Headers

- Emit `Link` header with `rel="alternate"; type="text/calendar"` on JSON API
  responses to enable ICS feed discovery.
- HTTPS ICS URL is canonical for agent/API integrations.
- `webcal://` support is deferred — not needed for API/agent consumers. If calendar
  client compatibility requires it, add in a future phase.

### Error Handling

- Structural handler errors reuse existing RFC 7807 problem responses and type URIs
  (e.g., `/problems/not-found`, `/problems/gone`). No new problem types needed for Phase 2.
- Serialization errors wrap with `%w` and map to `500` server error (generic internal error type).
- Empty datasets are non-errors and return valid empty `VCALENDAR`. Note: some clients
  (notably older Outlook) may display warnings for empty feeds — document client-specific
  behavior in `docs/integration/ics-compatibility-matrix.md`.

### Security Model

- Input surface: request filters only (existing parser/validation path).
- Output safety: serializer escapes ICS text values and never concatenates raw user text into control lines.
- No remote fetch in export path (DB-backed only).
- Maintain existing rate limiting tier on public read endpoints.

---

## Implementation Tasks

### Task 1: Implement ICS serializer in `internal/ical/serialize.go`

**What**: Build deterministic Event/Occurrence -> VCALENDAR serialization using Phase 1 contracts and UTC timestamps.

**Test**: `go test ./internal/ical -run Serialize`

**Acceptance**: Golden fixtures parse in at least one strict ICS parser and preserve expected fields.

### Task 2: Add `text/calendar` content negotiation support

**What**: Add `contentCalendar = "text/calendar"` constant (matching existing `contentJSON`, `contentJSONLD`, etc. naming) and extend routing logic for calendar media type without regressing JSON-LD/HTML/Turtle behavior.

**Test**: add/extend `internal/api/middleware/negotiate_test.go` and any affected handler tests.

**Acceptance**: Existing negotiation tests pass; new calendar cases pass.

### Task 3: Add feed handler for `GET /api/v1/events.ics`

**What**: Implement handler that parses filters, loads events, serializes ICS, supports cursor pagination (`after`/`limit`), and writes feed headers.

**Test**: handler tests covering success, empty feed, filter behavior, pagination continuation header, and serializer failure path.

**Acceptance**: Endpoint returns valid ICS, respects filter parser semantics from `events.ParseFilters`, and supports cursor continuation.

### Task 4: Add single-event ICS handler `GET /api/v1/events/{id}.ics`

**What**: Implement per-event download endpoint using `GetByULID` and serializer single-event path.

**Test**: handler tests for 200, 404, tombstone/410 behavior.

**Acceptance**: Valid ICS returned for found events; existing not-found/deleted semantics preserved.

### Task 5: Add API discovery headers (`Link rel="alternate"`)

**What**: Add alternate link emission on relevant API list/detail responses so clients can discover feed URLs without browser-specific paths.

**Test**: response-header tests for HTTPS `Link` alternates with correct `rel` and `type`.

**Acceptance**: Headers emitted consistently on API endpoints with correct rel/type and canonical HTTPS URL formatting.

### Task 6: Update OpenAPI for ICS endpoints and content

**What**: Document new `.ics` endpoints and `text/calendar` responses, including link-discovery behavior notes.

**Test**: `scripts/agent-run.sh make lint-openapi`

**Acceptance**: OpenAPI lint passes and generated docs render endpoint/media types correctly.

### Task 7: Bind tests to compatibility matrix

**What**: Reference `docs/integration/ics-compatibility-matrix.md` in Phase 2 export
tests and assert matrix scenarios relevant to serializer and endpoint behavior.

**Test**: serializer/endpoint tests cover strict parser + Apple/Google target rows.

**Acceptance**: compatibility expectations are explicit, versioned, and validated by tests.

---

## Configuration

No new environment variables are required for Phase 2.

Serializer defaults:
- `ProductID`: `-//Togather Foundation//SEL//EN`
- `Method`: `PUBLISH`
- Output timestamps: UTC (`Z` suffix)

---

## Success Criteria

1. `GET /api/v1/events.ics` works end-to-end and is consumable by at least Apple Calendar + Google Calendar import tests.
2. `GET /api/v1/events/{id}.ics` returns a valid single-event ICS artifact.
3. OpenAPI includes new ICS endpoints and `text/calendar` response media types.
4. No regressions in existing JSON-LD/HTML/Turtle content negotiation tests.
5. Phase 2 tests explicitly cover relevant rows in `docs/integration/ics-compatibility-matrix.md`.

---

## Phase 2 Decisions

1. **Pagination**: support cursor pagination in Phase 2 (`after`, `limit`) and emit `Link: <...>; rel="next"` (RFC 8288) when additional pages exist. No custom headers.
2. **Discovery scope**: add alternate discovery headers on API endpoints first (`/api/v1/events`, `/api/v1/events/{id}`); browser-oriented discovery is secondary. `webcal://` is deferred.
3. **Content disposition**: use `attachment` for both feed and single-event ICS for consistency with ICS convention and deterministic client behavior.
4. **Caching**: defer `ETag`/`Last-Modified` optimization to a follow-up phase/task.

## Rollback Notes (Phase 2)

- If ICS endpoints regress API behavior, remove `.ics` route wiring and keep JSON-LD
  endpoints as canonical output while preserving compatibility docs.
- If serializer output regresses compatibility, revert to last known-good golden
  fixture outputs and re-run matrix validations.
- Keep OpenAPI in sync with any route rollback to avoid contract drift.
