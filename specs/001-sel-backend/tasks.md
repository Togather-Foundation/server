---
description: "Task list for SEL backend implementation"
---

# Tasks: SEL Backend Server with Admin Frontend

**Input**: Design documents from /specs/001-sel-backend/
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml, SEL_Implementation_Plan.md, docs/
**Tests**: REQUIRED (TDD). Write tests first, confirm red, then implement until passing. Target 80%+ coverage (unit + integration + E2E where applicable).

## Format: [ID] [P?] [Story] Description

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1..US6)
- **File paths** are required in every task description

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and repo scaffolding per plan.md

 - [X] T001 Create sqlc configuration in sqlc.yaml
 - [X] T002 [P] Add golang-migrate/migrate tooling config in internal/storage/postgres/migrate.go
 - [X] T003 [P] Scaffold base package folders per plan.md (cmd/server, internal/api, internal/domain, internal/jsonld, internal/storage, internal/auth, internal/jobs, internal/config, web/admin)
 - [X] T004 [P] Create .env file from .env.example template and update SETUP.md with configuration instructions

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure required before any user story

 - [X] T005 Create base config loader (include auth + rate limit tiers + admin bootstrap env vars: ADMIN_USERNAME, ADMIN_PASSWORD, ADMIN_EMAIL + job retry policies) in internal/config/config.go
 - [X] T006 [P] Add structured logger setup in internal/config/logging.go
 - [X] T007 Implement HTTP server bootstrap in cmd/server/main.go
 - [X] T007a Implement bootstrap admin user creation from env vars (ADMIN_USERNAME, ADMIN_PASSWORD, ADMIN_EMAIL) on first startup (FR-027) in cmd/server/main.go
 - [X] T008 Implement router wiring in internal/api/router.go
- [ ] T009 [P] Expose OpenAPI at /api/v1/openapi.json in internal/api/router.go
- [ ] T010 [P] Add RFC 7807 error helpers with environment-aware detail levels (stack traces in dev/test, sanitized in production) in internal/api/problem/problem.go
- [ ] T011 [P] Add content negotiation middleware in internal/api/middleware/negotiate.go
- [ ] T012 [P] Add request logging middleware in internal/api/middleware/logging.go
- [ ] T013 [P] Add rate limiter middleware with tiered limits (public/agent/admin) in internal/api/middleware/ratelimit.go
- [ ] T014 [P] Add idempotency key middleware in internal/api/middleware/idempotency.go
- [ ] T015 [P] Add API key auth in internal/auth/apikey.go
- [ ] T016 [P] Add JWT generation and validation in internal/auth/jwt.go
- [ ] T016b [P] Add JWT cookie middleware for /admin/* HTML routes in internal/api/middleware/auth_cookie.go (validates HttpOnly cookie; API routes use Authorization header)
- [ ] T017 [P] Add RBAC helpers in internal/auth/rbac.go
 - [X] T018 Create migrations for core tables (events, event_occurrences, event_series) in internal/storage/postgres/migrations/000001_core.up.sql
 - [X] T019 Create migrations for provenance tables in internal/storage/postgres/migrations/000002_provenance.up.sql
- [X] T020 Create migrations for federation/changefeed + tombstones tables in internal/storage/postgres/migrations/000003_federation.up.sql
- [ ] T021 Create migrations for auth tables in internal/storage/postgres/migrations/000004_auth.up.sql (admin users + API keys)
- [ ] T022 [P] Add down migrations in internal/storage/postgres/migrations/00000X_*.down.sql
- [ ] T023 [P] Add SQLc query files per domain in internal/storage/postgres/queries/events.sql, places.sql, organizations.sql, sources.sql, provenance.sql, feeds.sql, federation.sql, auth.sql
- [ ] T024 [P] Add SQLc build targets to Makefile (sqlc-generate, migrate-up/down)
- [ ] T025 Implement repository interfaces in internal/storage/repository.go
- [ ] T026 Implement Postgres repo wiring in internal/storage/postgres/db.go
- [ ] T027 [P] Implement ULID + canonical URI helpers (regex validation, identifier roles: canonical=local entity, foreign=federated entity with origin_node_id, alias=sameAs link; sameAs normalization to full URIs) in internal/domain/ids/ids.go
- [ ] T028 [P] Add versioned JSON-LD context loader in internal/jsonld/context.go
- [ ] T029 [P] Add canonical JSON-LD frames and framing utilities in internal/jsonld/framing.go
- [ ] T030 [P] Add JSON-LD serializer in internal/jsonld/serializer.go
- [ ] T031 Add SHACL validation harness for tests in tests/contracts/shacl_validation_test.go
- [ ] T032 [P] Add /healthz and /readyz endpoints in internal/api/handlers/health.go
- [ ] T033a [P] Setup River job queue with job-type-specific retry configuration (deduplication: 1 attempt, reconciliation/enrichment: 5-10 attempts, exponential backoff) in internal/jobs/river.go
- [ ] T033b [P] Add job failure alerting hooks in internal/jobs/alerts.go

**Checkpoint**: Foundation ready

---

## Phase 3: User Story 1 - Clients Discover and Consume Events (Priority: P1) 🎯 MVP

**Goal**: Public read access for events, places, organizations with filtering and pagination

**Independent Test**: A public client queries /api/v1/events with filters and pagination and receives JSON-LD with @context and next_cursor; /events/{id} returns full event; RFC 7807 on invalid params

### Tests for User Story 1 (TDD - write first)

- [ ] T034 [P] [US1] Integration tests for GET /api/v1/events filters/pagination in tests/integration/events_list_test.go
- [ ] T035 [P] [US1] Integration tests for GET /api/v1/events/{id} in tests/integration/events_get_test.go
- [ ] T036 [P] [US1] Integration tests for GET /api/v1/places and /places/{id} in tests/integration/places_test.go
- [ ] T037 [P] [US1] Integration tests for GET /api/v1/organizations and /organizations/{id} in tests/integration/organizations_test.go
- [ ] T038a [P] [US1] Contract test for RFC 7807 errors: invalid ULID format in tests/contracts/problem_details_test.go
- [ ] T038b [P] [US1] Contract test for RFC 7807 errors: malformed date range parameters in tests/contracts/problem_details_test.go
- [ ] T038c [P] [US1] Contract test for RFC 7807 errors: missing/invalid Accept header handling (default to application/json) in tests/contracts/problem_details_test.go
- [ ] T038d [P] [US1] Contract test for RFC 7807 environment-aware detail levels (stack traces in dev, sanitized in prod) in tests/contracts/problem_details_test.go
- [ ] T039 [P] [US1] JSON-LD framing tests for list + detail in tests/contracts/jsonld_events_test.go
- [ ] T039a [P] [US1] Integration tests for /healthz and /readyz endpoints (FR-017) in tests/integration/health_test.go
- [ ] T039b [P] [US1] Contract test for OpenAPI spec validation (FR-012): ensure /api/v1/openapi.json matches implemented routes in tests/contracts/openapi_validation_test.go
- [ ] T039c [P] [US1] Contract test for pagination cursor encoding/decoding and next_cursor behavior (FR-006) in tests/contracts/pagination_test.go
- [ ] T039d [P] [US1] Contract test for URI validation: ULID format, canonical URI pattern, sameAs normalization (FR-022) in tests/contracts/uri_validation_test.go

### Implementation for User Story 1

- [ ] T040 [P] [US1] Implement event query SQL with required filters (including occurrence date range filtering) in internal/storage/postgres/queries/events.sql
- [ ] T041 [P] [US1] Implement place query SQL in internal/storage/postgres/queries/places.sql
- [ ] T042 [P] [US1] Implement organization query SQL in internal/storage/postgres/queries/organizations.sql
- [ ] T043 [US1] Implement event repository in internal/domain/events/repository.go
- [ ] T044 [US1] Implement event service in internal/domain/events/service.go
- [ ] T045 [US1] Implement filter parsing/validation (date range, city/region, venue ID, organizer ID, lifecycle_state, query, keywords, event domain) in internal/domain/events/service.go and internal/api/handlers/events.go
- [ ] T046 [US1] Implement place repository/service in internal/domain/places/repository.go and internal/domain/places/service.go
- [ ] T047 [US1] Implement organization repository/service in internal/domain/organizations/repository.go and internal/domain/organizations/service.go
- [ ] T048 [US1] Implement handlers for list/get events in internal/api/handlers/events.go
- [ ] T049 [US1] Implement handlers for list/get places in internal/api/handlers/places.go
- [ ] T050 [US1] Implement handlers for list/get organizations in internal/api/handlers/organizations.go
- [ ] T051 [US1] Implement cursor pagination helpers (for events list: base64(timestamp+ULID) for stable ordering; for change feed: base64(sequence_number BIGSERIAL) per schema design) in internal/api/pagination/cursor.go
- [ ] T052 [US1] Wire routes for public endpoints in internal/api/router.go

**Checkpoint**: User Story 1 functional and independently testable

---

## Phase 4: User Story 2 - Agents Submit Events (Priority: P1) 🎯 MVP

**Goal**: Authenticated event ingestion with validation, idempotency, and source tracking

**Independent Test**: An authenticated agent can POST /api/v1/events and immediately retrieve it via GET

### Tests for User Story 2 (TDD - write first)

- [ ] T052 [P] [US2] Integration tests for POST /api/v1/events (happy path) in tests/integration/events_create_test.go
- [ ] T053 [P] [US2] Integration tests for validation errors in tests/integration/events_create_validation_test.go
- [ ] T054 [P] [US2] Integration tests for auth failures in tests/integration/events_create_auth_test.go
- [ ] T055 [P] [US2] Integration tests for idempotency in tests/integration/events_idempotency_test.go
- [ ] T056 [P] [US2] Unit tests for validation rules in internal/domain/events/validation_test.go
- [ ] T057 [P] [US2] Contract tests for license rejection (FR-015) in tests/contracts/license_rejection_test.go

### Implementation for User Story 2

- [ ] T058 [US2] Implement event validation (required fields, license compliance FR-015, URI validation FR-022) in internal/domain/events/validation.go
- [ ] T059 [US2] Implement normalization utilities in internal/domain/events/normalize.go
- [ ] T059a [US2] Implement occurrence validation (at least one occurrence required, valid dates, timezone handling) in internal/domain/events/validation.go
- [ ] T060 [US2] Implement deduplication logic (exact hash: SHA-256 of normalized JSON-LD canonical name+venue+startDate; fuzzy: pg_trgm; vector: deferred) in internal/domain/events/dedup.go
- [ ] T061 [US2] Implement event creation repository methods in internal/domain/events/repository.go
- [ ] T062 [US2] Implement ingestion service (create event + one or more occurrence rows based on event data) in internal/domain/events/ingest.go
- [ ] T063 [US2] Implement auto-publish + review-queue flagging rules (low-confidence threshold: confidence < 0.6, missing description/image, invalid external links returning HTTP 4xx/5xx, startDate >730 days future) in internal/domain/events/ingest.go
- [ ] T064 [US2] Implement source registry access in internal/domain/provenance/service.go
- [ ] T065 [US2] Implement POST handler in internal/api/handlers/events.go
- [ ] T066 [US2] Add idempotency storage queries in internal/storage/postgres/queries/events.sql
- [ ] T067 [US2] Wire auth + rate limit middleware for write endpoints in internal/api/router.go
- [ ] T067a [US2] Implement background job workers for reconciliation/enrichment with retry policies in internal/jobs/workers.go

**Checkpoint**: User Story 2 functional and independently testable

---

## Phase 5: User Story 3 - Admin Reviews and Edits Events (Priority: P2)

**Goal**: Admin can review, edit, and manage events + API keys via HTML UI and admin APIs

**Independent Test**: Admin can login, view pending list, edit event, and see changes reflected in API

### Tests for User Story 3 (TDD - write first)

- [ ] T069 [P] [US3] Integration tests for admin login + JWT transport (header + cookie) in tests/integration/admin_auth_test.go
- [ ] T069a [P] [US3] Integration tests for JWT routing: /api/v1/admin/* accepts Bearer token, /admin/* HTML routes require cookie (FR-028) in tests/integration/admin_auth_routing_test.go
- [ ] T070 [P] [US3] Integration tests for admin pending list in tests/integration/admin_events_pending_test.go
- [ ] T071 [P] [US3] Integration tests for admin update event in tests/integration/admin_events_update_test.go
- [ ] T072 [P] [US3] Integration tests for admin merge duplicates in tests/integration/admin_events_merge_test.go
- [ ] T073 [P] [US3] Integration tests for admin delete + tombstone in tests/integration/admin_events_delete_test.go
- [ ] T074 [P] [US3] E2E HTML smoke tests for /admin pages in tests/e2e/admin_ui_test.go

### Implementation for User Story 3

- [ ] T075 [US3] Implement admin auth handlers in internal/api/handlers/admin_auth.go (POST /api/v1/admin/login issues JWT as Authorization header for API clients AND HttpOnly cookie for HTML UI; GET /admin/login renders login page)
- [ ] T076 [US3] Implement admin event review handlers in internal/api/handlers/admin.go
- [ ] T077 [US3] Implement admin services for review/merge in internal/domain/events/admin_service.go
- [ ] T081b [US3] Implement federation node registry CRUD (create/list/update/delete trusted peer nodes for federation sync) in internal/api/handlers/admin.go and internal/domain/federation/nodes.go
- [ ] T078 [US3] Implement API key management in internal/domain/auth/apikeys.go
- [ ] T079 [US3] Implement admin delete (soft delete + tombstone generation) in internal/domain/events/admin_service.go and internal/api/handlers/admin.go
- [ ] T080 [US3] Add admin templates in web/admin/templates/*.html (login.html for GET /admin/login, dashboard.html, events_list.html, event_edit.html, duplicates.html, api_keys.html)
- [ ] T081 [US3] Add admin static assets in web/admin/static/*
- [ ] T082 [US3] Wire admin routes + auth in internal/api/router.go

**Checkpoint**: User Story 3 functional and independently testable

---

## Phase 6: User Story 4 - Content Negotiation and Dereferenceable URIs (Priority: P3)

**Goal**: URI dereferencing returns HTML or JSON-LD/Turtle based on Accept header

**Independent Test**: Accept: text/html returns HTML with embedded JSON-LD; Accept: application/ld+json returns JSON-LD; deleted event returns 410 with tombstone

### Tests for User Story 4 (TDD - write first)

- [ ] T083 [P] [US4] Integration tests for Accept header behavior in tests/integration/content_negotiation_test.go
- [ ] T084 [P] [US4] Contract tests for HTML with embedded JSON-LD in tests/contracts/html_embedding_test.go
- [ ] T085 [P] [US4] Contract tests for Turtle output in tests/contracts/turtle_output_test.go
- [ ] T086 [P] [US4] Integration tests for 410 Gone tombstone on deleted events in tests/integration/events_tombstone_test.go
- [ ] T086a [P] [US4] Integration tests for 410 Gone tombstone on deleted places in tests/integration/places_tombstone_test.go
- [ ] T086b [P] [US4] Integration tests for 410 Gone tombstone on deleted organizations in tests/integration/organizations_tombstone_test.go

### Implementation for User Story 4

- [ ] T087 [US4] Implement HTML render helpers (name, startDate, location, organizer, embedded JSON-LD) in internal/api/render/html.go
- [ ] T088 [US4] Implement Turtle serialization in internal/jsonld/turtle.go
- [ ] T089 [US4] Add event URI handlers for /events/{id} without /api prefix in internal/api/handlers/public_pages.go
- [ ] T090 [US4] Return 410 with tombstone JSON-LD for deleted events in internal/api/handlers/events.go and internal/api/handlers/public_pages.go
- [ ] T091 [US4] Wire dereferenceable routes in internal/api/router.go

**Checkpoint**: User Story 4 functional and independently testable

---

## Phase 7: User Story 5 - Provenance Tracking and Source Attribution (Priority: P3)

**Goal**: Field-level provenance and source attribution in responses

**Independent Test**: Retrieved event includes source attribution and optional per-field provenance via query parameter

### Tests for User Story 5 (TDD - write first)

- [ ] T092 [P] [US5] Integration tests for source attribution in tests/integration/provenance_event_source_test.go
- [ ] T093 [P] [US5] Integration tests for field provenance parameter in tests/integration/provenance_field_test.go
- [ ] T094 [P] [US5] Unit tests for conflict resolution in internal/domain/provenance/conflict_test.go
- [ ] T094a [P] [US5] Contract test for license information in JSON-LD responses (FR-024) in tests/contracts/license_response_test.go
- [ ] T094b [P] [US5] Contract test for dual timestamp tracking: source-provided vs server-received (FR-029) in tests/contracts/timestamp_tracking_test.go

### Implementation for User Story 5

- [ ] T095 [US5] Implement provenance repository queries in internal/storage/postgres/queries/provenance.sql (include source + received timestamps)
- [ ] T096 [US5] Implement provenance service in internal/domain/provenance/service.go
- [ ] T097 [US5] Embed provenance + license in JSON-LD responses in internal/jsonld/serializer.go
- [ ] T098 [US5] Add provenance query param handling in internal/api/handlers/events.go

**Checkpoint**: User Story 5 functional and independently testable

---

## Phase 8: User Story 6 - Change Feed for Federation (Priority: P4)

**Goal**: Cursor-based change feed for create/update/delete events

**Independent Test**: GET /api/v1/feeds/changes returns ordered changes with cursor and tombstones for deletes

### Tests for User Story 6 (TDD - write first)

- [ ] T099 [P] [US6] Integration tests for change feed pagination in tests/integration/feeds_changes_test.go
- [ ] T100 [P] [US6] Integration tests for delete tombstones in tests/integration/feeds_tombstone_test.go
- [ ] T101 [P] [US6] Unit tests for cursor encoding in internal/domain/federation/cursor_test.go
- [ ] T102 [P] [US6] Integration tests for federation sync auth + validation in tests/integration/federation_sync_auth_test.go
- [ ] T103 [P] [US6] Integration tests for federation sync idempotency in tests/integration/federation_sync_idempotency_test.go
- [ ] T104 [P] [US6] Integration tests for federation sync URI preservation in tests/integration/federation_sync_uri_preservation_test.go

### Implementation for User Story 6

- [ ] T105 [US6] Implement change capture SQL in internal/storage/postgres/queries/feeds.sql (include source + received timestamps)
- [ ] T106 [US6] Implement change feed service in internal/domain/federation/changefeed.go
- [ ] T107 [US6] Implement change feed handler in internal/api/handlers/feeds.go
- [ ] T108 [US6] Implement federation sync SQL in internal/storage/postgres/queries/federation.sql
- [ ] T109 [US6] Implement federation sync service in internal/domain/federation/sync.go
- [ ] T110 [US6] Implement federation sync handler in internal/api/handlers/federation.go
- [ ] T111 [US6] Wire feed + federation sync routes in internal/api/router.go

**Checkpoint**: User Story 6 functional and independently testable

---

## Phase 9: Polish & Cross-Cutting Concerns

- [ ] T112 [P] Update docs/ and specs/001-sel-backend/quickstart.md with TDD steps and test commands
- [ ] T112a [P] Add terminology glossary to docs/glossary.md (canonical terms: change_feed=ordered log of modifications, lifecycle_state=event state enum, field_provenance=source attribution, federation_uri=original URI for federated entities)
- [ ] T113 [P] Add Go coverage thresholds in Makefile (go test -coverprofile + coverage check)
- [ ] T114 [P] Add CI test targets for contract + integration tests in .github/workflows/ci.yml
- [ ] T115 [P] Add CI target for SHACL validation against shapes/*.ttl in .github/workflows/ci.yml
- [ ] T116 [P] Add CI target for federation sync contract/integration tests in .github/workflows/ci.yml
- [ ] T117 [P] Add docs updates referencing SEL_Implementation_Plan.md in docs/README.md
- [ ] T118a [P] Edge case test: Event submission with future date >730 days (accept with flagging) in tests/integration/edge_cases_test.go
- [ ] T118b [P] Edge case test: Invalid/expired external links (defer validation, accept with warning) in tests/integration/edge_cases_test.go
- [ ] T118c [P] Edge case test: Non-CC0 license submission (reject at boundary with error) in tests/integration/edge_cases_test.go
- [ ] T118d [P] Edge case test: Concurrent update conflict (optimistic locking, 409 response) in tests/integration/edge_cases_test.go
- [ ] T118e [P] Edge case test: Missing Accept header (default to application/json) in tests/integration/edge_cases_test.go
- [ ] T119 Run full test suite and record results in specs/001-sel-backend/quickstart.md

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Blocks all user stories
- **User Stories (Phase 3+)**: Depend on Foundational completion
- **Polish (Phase 9)**: Depends on all desired user stories

### User Story Dependencies

- **US1** and **US2** are MVP and can run in parallel after Foundational
- **US3** depends on US2 (admin edits require ingestion)
- **US4** depends on US1 (content negotiation for event retrieval)
- **US5** depends on US2 (provenance created at ingestion)
- **US6** depends on US2 (change feed from ingestion/update/delete)

### Parallel Execution Examples

**US1 (Discovery)**
- T033, T034, T035, T036, T037, T038 can run in parallel
- T039, T040, T041 can run in parallel after tests exist

**US2 (Ingestion)**
- T052, T053, T054, T055, T056, T057 can run in parallel

**US3 (Admin)**
- T068, T069, T070, T071, T072, T073 can run in parallel

---

## Implementation Strategy

### MVP First (US1 + US2)
1. Complete Phase 1 + Phase 2
2. Complete US1 tests → implement → green
3. Complete US2 tests → implement → green
4. Validate MVP: list, get, create

### Incremental Delivery
- Add US3 (Admin) → green
- Add US4 (Content negotiation) → green
- Add US5 (Provenance) → green
- Add US6 (Change feed) → green

---

## Notes

- All tests must be written first and verified failing before implementation.
- Keep each task small enough for a single agent to complete.
- Align with SEL_Implementation_Plan.md and docs/ architecture decisions.
