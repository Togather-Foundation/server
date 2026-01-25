# Implementation Plan: SEL Backend Server with Admin Frontend

**Branch**: `001-sel-backend` | **Date**: 2026-01-23 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-sel-backend/spec.md`

## Summary

Build a Go backend server implementing the Shared Events Library (SEL) with:
- RESTful API for event submission (agents) and discovery (public)
- JSON-LD serialization with Schema.org alignment and content negotiation
- PostgreSQL storage with ULID identifiers, field-level provenance, and federated URI preservation
- Event occurrences + series model with tombstone handling
- Cursor-based pagination and RFC 7807 error responses
- HTTP 410 tombstones for deleted entities
- API key auth for agents, JWT for admins, public read access
- Minimal admin HTML UI embedded in the binary
- OpenAPI 3.1 spec served at /api/v1/openapi.json
- Change feed endpoint plus minimal federation sync endpoint and node registry for peer submissions

## Technical Context

**Language/Version**: Go 1.22+  
**Primary Dependencies**: Huma (HTTP/OpenAPI 3.1), SQLc (type-safe SQL), River (transactional job queue), piprate/json-gold (JSON-LD), oklog/ulid/v2, golang-jwt/jwt/v5, go-playground/validator/v10  
**Supporting Libraries**: rs/zerolog (logging), knadh/koanf (config), spf13/cobra (CLI framework), hashicorp/go-retryablehttp (HTTP client), stretchr/testify (testing), golang.org/x/time/rate (rate limiting), oklog/run (graceful shutdown), golang-migrate/migrate (migrations)  
**Storage**: PostgreSQL 16+ with PostGIS, pgvector, pg_trgm extensions  
**Testing**: go test, testcontainers-go for integration tests, go test -race  
**Target Platform**: Linux server (Docker), single binary deployment  
**Project Type**: Single Go project with embedded admin UI assets  
**Performance Goals**: 100 concurrent submissions without errors, <500ms p95 for filtered queries over 10k events, <1s p95 end-to-end latency for submit-then-retrieve  
**Constraints**: Single-binary deployment, Postgres-centric (no external vector DB for MVP), CPU-only (no GPU required)  
**Scale/Scope**: Per-city deployment model, initially thousands to tens of thousands of events per node

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Semantic Web Correctness (NON-NEGOTIABLE)

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Schema.org alignment | ✅ PASS | All entities map to Schema.org vocabulary per Interoperability Profile v0.1 |
| Dereferenceable URIs | ✅ PASS | Content negotiation (HTML, JSON-LD, Turtle) required per FR-004 and FR-016 |
| Federation URI preservation | ✅ PASS | `origin_node_id` tracking, never re-mint foreign URIs per architecture doc |
| SHACL validation | ✅ PASS | CI/CD gate on `shapes/*.ttl` per SC-009 |
| License compliance | ✅ PASS | CC0 rejection at ingestion boundary per FR-015 |

### II. Provenance as First-Class Concern

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Field-level attribution | ✅ PASS | `field_provenance` table in schema design |
| Source trust levels | ✅ PASS | `sources` registry with 1-10 trust scale |
| Conflict resolution | ✅ PASS | Priority rules in schema: `trust_level DESC, confidence DESC, observed_at DESC` |
| Audit trails | ✅ PASS | `event_history` table with change tracking trigger |

### III. Test Coverage for Public Contracts (NON-NEGOTIABLE)

| Requirement | Status | Evidence |
|-------------|--------|----------|
| API integration tests | ✅ PASS | testcontainers-go approach documented |
| JSON-LD round-trip tests | ✅ PASS | Required per constitution |
| SHACL validation in CI | ✅ PASS | SC-009 requires CI gate |
| Race condition tests | ✅ PASS | `make test-race` in Makefile |

### IV. Domain-Driven Structure

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Feature packages | ✅ PASS | Structure follows `events/`, `places/`, `organizations/`, `provenance/` |
| Thin handlers | ✅ PASS | Huma + service layer pattern per architecture doc |
| Explicit constructors | ✅ PASS | Dependency injection via constructors (no global state) |

### V. Operational Simplicity

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Single-binary deployment | ✅ PASS | Embedded admin UI assets, single Go binary |
| Postgres-centric | ✅ PASS | PostgreSQL for all storage (JSONB, pgvector, PostGIS) |
| Transactional job queue | ✅ PASS | River queue for background work |
| Explicit SQL via SQLc | ✅ PASS | Type-safe Go from SQL, no ORM |
| YAGNI compliance | ✅ PASS | pgvector for MVP (no USearch acceleration yet), no vector search in MVP scope |

**Gate Status**: ✅ ALL GATES PASS — Proceed to Phase 0

## Project Structure

### Documentation (this feature)

```text
specs/001-sel-backend/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (OpenAPI spec)
│   └── openapi.yaml
├── checklists/
│   └── requirements.md  # Already exists
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
└── server/
    └── main.go              # Application entrypoint

internal/
├── api/
│   ├── handlers/            # HTTP handlers (thin, delegate to services)
│   │   ├── events.go
│   │   ├── places.go
│   │   ├── organizations.go
│   │   ├── admin.go
│   │   ├── feeds.go
│   │   ├── federation.go
│   │   └── health.go
│   ├── middleware/          # Auth, rate limiting, content negotiation
│   │   ├── auth.go
│   │   ├── ratelimit.go
│   │   └── negotiate.go
│   └── router.go            # Huma router setup
├── domain/
│   ├── events/              # Event domain logic
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── validation.go
│   ├── places/
│   │   ├── service.go
│   │   └── repository.go
│   ├── organizations/
│   │   ├── service.go
│   │   └── repository.go
│   ├── provenance/
│   │   ├── service.go
│   │   └── repository.go
│   └── federation/
│       ├── changefeed.go
│       └── sync.go
├── jsonld/
│   ├── serializer.go        # JSON-LD output generation
│   ├── context.go           # SEL context handling
│   └── framing.go           # piprate/json-gold integration
├── storage/
│   ├── postgres/
│   │   ├── db.go            # SQLc generated code
│   │   ├── queries/         # SQL query files for SQLc
│   │   └── migrations/      # Database migrations
│   └── repository.go        # Repository interfaces
├── auth/
│   ├── jwt.go               # JWT handling
│   ├── apikey.go            # API key validation
│   └── rbac.go              # Role-based access control
├── jobs/
│   └── river.go             # River job queue setup
└── config/
    └── config.go            # Configuration loading

web/
└── admin/
    ├── templates/           # Go HTML templates
    └── static/              # CSS, JS assets (embedded)

contexts/
└── sel/
    └── v0.1.jsonld          # SEL JSON-LD context (exists)

shapes/
├── event-v0.1.ttl           # SHACL shapes (exist)
├── organization-v0.1.ttl
└── place-v0.1.ttl

tests/
├── integration/
│   ├── api_test.go          # Full API integration tests
│   └── jsonld_test.go       # JSON-LD round-trip tests
└── fixtures/
    └── events/              # Test event payloads
```

**Structure Decision**: Single Go project following domain-driven package organization per Constitution Principle IV. Admin UI is embedded in the binary (templates + static assets) per Constitution Principle V (single-binary deployment).

## Complexity Tracking

> No constitution violations requiring justification. All design decisions align with principles.

| Item | Decision | Justification |
|------|----------|---------------|
| Single binary | ✅ Compliant | Admin UI embedded, no separate frontend build |
| PostgreSQL only | ✅ Compliant | pgvector for embeddings (if added), PostGIS for geo, no external services |
| SQLc over ORM | ✅ Compliant | Explicit SQL, type-safe Go code generation |
| River for jobs | ✅ Compliant | Transactional job queue, same DB, avoids goroutine leaks |

---

## Constitution Check (Post-Design)

*Re-evaluation after Phase 1 design artifacts completed.*

### I. Semantic Web Correctness (NON-NEGOTIABLE)

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Schema.org alignment | ✅ PASS | data-model.md includes explicit Schema.org mappings for all entities |
| Dereferenceable URIs | ✅ PASS | OpenAPI spec defines content negotiation for all entity endpoints |
| Federation URI preservation | ✅ PASS | `origin_node_id` and `federation_uri` fields in Events table |
| SHACL validation | ✅ PASS | shapes/*.ttl referenced; CI gate documented in research.md |
| License compliance | ✅ PASS | `license_status` field with CC0 default; FR-015 in API validation |

### II. Provenance as First-Class Concern

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Field-level attribution | ✅ PASS | `field_provenance` table defined in data-model.md |
| Source trust levels | ✅ PASS | `sources` table with `trust_level` (1-10) defined |
| Conflict resolution | ✅ PASS | Priority query documented: `trust_level DESC, confidence DESC, observed_at DESC` |
| Audit trails | ✅ PASS | `event_changes` outbox + `event_history` tables for audit |

### III. Test Coverage for Public Contracts (NON-NEGOTIABLE)

| Requirement | Status | Evidence |
|-------------|--------|----------|
| API integration tests | ✅ PASS | research.md defines testcontainers-go strategy |
| JSON-LD round-trip tests | ✅ PASS | Listed in testing strategy |
| SHACL validation in CI | ✅ PASS | Contract tests defined |
| Race condition tests | ✅ PASS | `go test -race` in Makefile |

### IV. Domain-Driven Structure

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Feature packages | ✅ PASS | Project structure shows `domain/events/`, `domain/places/`, etc. |
| Thin handlers | ✅ PASS | `api/handlers/` separate from `domain/` services |
| Explicit constructors | ✅ PASS | No global state; DI via constructors (Go idiom) |

### V. Operational Simplicity

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Single-binary deployment | ✅ PASS | `web/admin/` embedded; single `cmd/server/main.go` |
| Postgres-centric | ✅ PASS | All storage in PostgreSQL (no Redis, no external vector DB) |
| Transactional job queue | ✅ PASS | River with PostgreSQL backend |
| Explicit SQL via SQLc | ✅ PASS | `internal/storage/postgres/queries/` defined |
| YAGNI compliance | ✅ PASS | Vector search deferred; minimal MVP scope |

**Post-Design Gate Status**: ✅ ALL GATES PASS — Design aligns with constitution
