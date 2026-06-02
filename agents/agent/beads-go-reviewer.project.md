# Project-Specific Review Extension — Togather (SEL) Server

This is read by `beads-go-reviewer` (the generic Go reviewer) when it reviews
this project. It layers **on top of** the generic Go review + the `web-backend`
domain profile. Define only what's unique to this project.

## SEL Specification Compliance

**Events:**
- JSON-LD context is `https://schema.togather.foundation/contexts/event-v0.1`
  (versioned, not unversioned).
- Events validate against `shapes/event-v0.1.ttl` SHACL shapes.
- `license` field defaults to CC0 (`https://creativecommons.org/publicdomain/zero/1.0/`).
- `sameAs` array handles cross-platform deduplication.
- Source `provenance` preserved (the raw scraped payload as `source_provenance` JSONB).

**Places:** validate against `shapes/place-v0.1.ttl` SHACL shapes.

**Organizations:** validate against `shapes/organization-v0.1.ttl` SHACL shapes.

**Content negotiation:** all `/events`, `/places`, `/organizations` endpoints
must support `Accept: application/ld+json`. Error responses use RFC 7807
Problem Details (`application/problem+json`).

**Response envelopes:** stable format `{"items": [...], "next_cursor": "..."}`
(not data/hits/results). Pagination is cursor-based, not offset.

## Project-Stack-Specific Checks

### Storage & SQL
- **SQLc only.** No raw SQL strings in Go — all queries are in `.sql` files
  under `internal/storage/postgres/`, parameterized via SQLc-generated code.
  Flag any `db.Query(fmt.Sprintf(...))` or string-built queries.
- **pgx** native driver; use `pgx.Tx` for multi-step writes (not raw
  `db.Begin`). Rollback on every error path.
- **JSONB fidelity.** Source provenance columns preserve the original scraped
  payload; no lossy transformations.
- **ULIDs** for entity IDs (`oklog/ulid/v2`), globally unique, monotonic within
  the same millisecond. Flag non-ULID identifiers on new entities.
- **Migrations** in `internal/storage/postgres/migrations/`. Check they're
  backward-compatible (no destructive column drops without a plan).

### API & HTTP
- **Huma** generates OpenAPI from Go struct tags. New/changed endpoints must
  update `docs/api/openapi.yaml`. Run `make lint-openapi` to verify.
- **REST conventions:** `GET /api/v1/events` (list), `POST /api/v1/events`
  (create), `GET /api/v1/events/{id}` (get). Admin routes under
  `/api/v1/admin/`.
- **Auth:** JWT (admin tokens, 8h TTL) + API keys (agent role). JWT validation
  checks signature, expiry, and claims. API key comparison uses
  `subtle.ConstantTimeCompare`.
- **Rate limiting:** configurable, default per `internal/config/config.go`.
  Check `RATE_LIMIT_*` env vars.

### Background Jobs
- **River** for job queue; workers registered in `internal/jobs/`. Job insertion
  must be transactional (same tx as the data change) where atomicity matters.
- Job context respects cancellation; retry with exponential backoff.

### Documentation (HIGH priority — this is open-source community infrastructure)
- Every exported function/type needs godoc. Public APIs need `Example` functions.
- API changes → `docs/api/openapi.yaml` updated. Run `make lint-openapi`.
- `CHANGELOG.md` updated for user-facing changes.
- `docs/` directory reflects architecture decisions.
- Code readable by newcomers (not just the original author).

### Testing
- Run `make test-race` in CI; all tests must pass with `-race`.
- `make coverage` target; aim 80%+.
- Integration tests use `tests/` directory with ephemeral PostGIS DB.
- Contract tests validate API responses against the OpenAPI spec.
- SHACL shapes tested: create a valid event, serialize to JSON-LD, validate
  against the shape — and test a shape violation is rejected.

## Common bead-worthy findings (SEL-specific)

- SQL built by string concatenation instead of SQLc → P0–P1.
- JSON-LD context URL wrong/versioned → P1.
- Missing SHACL validation on entity save → P1.
- CC0 license not defaulted → P2.
- Provenance data lost in transformation → P1.
- New endpoint without OpenAPI spec update → P1.
- ULID not used for new entity IDs → P2.
- River job enqueued outside transaction (when atomicity needed) → P1–P2.
