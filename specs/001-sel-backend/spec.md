# Feature Specification: SEL Backend Server with Admin Frontend

**Feature Branch**: `001-sel-backend`  
**Created**: 2026-01-23  
**Status**: Draft  
**Input**: User description: "Go backend server for Shared Events Library with admin frontend for event management"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Clients Discover and Consume Events (Priority: P1)

Discovery applications, personal AI curators, and developers need to query and retrieve events from SEL to build user-facing event discovery experiences. This is the primary value proposition of the Shared Events Library.

**Why this priority**: SEL exists to make events discoverable. Without public read access, there's no events commons. This is the most common use case—many readers consuming data that few writers provide.

**Independent Test**: A client application can query `/api/v1/events` with filters (date range, location), receive a paginated JSON-LD response, and use the data to display events to users. No authentication required.

**Acceptance Scenarios**:

1. **Given** events exist in the system, **When** a public client requests GET `/api/v1/events?startDate=2026-02-01&endDate=2026-02-28`, **Then** the system returns a paginated list of events in JSON-LD format with proper `@context` and `next_cursor` for pagination
2. **Given** events exist in multiple cities, **When** a client requests GET `/api/v1/events?city=Toronto`, **Then** the system returns only events in Toronto with location details
3. **Given** a specific event exists, **When** a client requests GET `/api/v1/events/{ulid}`, **Then** the system returns the full event with all Schema.org fields, location, organizer, and source attribution
4. **Given** a large result set, **When** a client paginates using `after={cursor}&limit=50`, **Then** the system returns the next page of results with a new cursor for continued pagination
5. **Given** an invalid query parameter, **When** a client makes a malformed request, **Then** the system returns 400 Bad Request with RFC 7807 error explaining the issue

---

### User Story 2 - Agents Submit Events (Priority: P1)

External event aggregation agents (scrapers, partner systems, data providers) need to submit event data to SEL so it becomes part of the shared commons. This is how data enters the system.

**Why this priority**: Without event ingestion, there's nothing to discover. Agents are the primary data source for populating the events library.

**Independent Test**: An authenticated agent can POST a valid Schema.org/Event payload to `/api/v1/events`, receive a 201 Created response with the event's canonical URI, and the event immediately appears in query results.

**Acceptance Scenarios**:

1. **Given** an authenticated agent with valid API key, **When** the agent POSTs a JSON-LD event payload with name, startDate, and location, **Then** the system returns 201 Created with the event's canonical `@id` URI
2. **Given** an event submission with missing required field (startDate), **When** the agent POSTs, **Then** the system returns 400 Bad Request with RFC 7807 error envelope explaining the validation failure
3. **Given** an agent submits the same event twice (same source + source_event_id), **When** using idempotency, **Then** the system returns the existing event without creating a duplicate
4. **Given** an unauthenticated request, **When** attempting to POST an event, **Then** the system returns 401 Unauthorized

---

### User Story 3 - Admin Reviews and Edits Events (Priority: P2)

An administrator or editor needs to review submitted events, correct data quality issues, approve pending submissions, and manage event lifecycle (cancel, mark complete, merge duplicates). This enables human oversight of automated ingestion.

**Why this priority**: Automated ingestion requires human quality control. Admins need to fix errors, handle edge cases, and maintain data quality standards.

**Independent Test**: An admin can log in via `POST /api/v1/admin/login`, view a list of pending/problematic events, edit an event's details, and see those changes reflected in API responses. Changes are logged with admin attribution.

**Acceptance Scenarios**:

1. **Given** an admin user with valid credentials, **When** the admin requests GET `/api/v1/admin/events/pending`, **Then** the system returns events awaiting review with quality indicators
2. **Given** an event with incorrect venue information, **When** the admin PUTs corrected data to `/api/v1/admin/events/{id}`, **Then** the event is updated, version incremented, and change logged with admin attribution
3. **Given** two duplicate events detected by the system, **When** the admin views GET `/api/v1/admin/duplicates` and confirms the merge, **Then** one event becomes canonical with `sameAs` links and the other returns 410 Gone with tombstone
4. **Given** an event that should be cancelled, **When** the admin updates lifecycle_state to "cancelled", **Then** the event shows `eventStatus: EventCancelled` in JSON-LD output

---

### User Story 4 - Content Negotiation and Dereferenceable URIs (Priority: P3)

External systems (knowledge graphs, search engines, linked data consumers) need to resolve SEL URIs and receive appropriate representations based on their Accept header. Humans browsing should see HTML; machines should get JSON-LD or Turtle.

**Why this priority**: SEL URIs must be dereferenceable per linked data principles. This enables interoperability with Artsdata, Wikidata, and other knowledge graphs.

**Independent Test**: A browser request to `https://{domain}/events/{ulid}` returns human-readable HTML with embedded JSON-LD, while a curl request with `Accept: application/ld+json` returns pure JSON-LD.

**Acceptance Scenarios**:

1. **Given** an event exists, **When** a client requests its URI with `Accept: text/html`, **Then** the system returns an HTML page showing name, startDate, location, organizer, with JSON-LD embedded in a `<script type="application/ld+json">` tag
2. **Given** an event exists, **When** a client requests its URI with `Accept: application/ld+json`, **Then** the system returns canonical JSON-LD with proper `@context` and `@id`
3. **Given** an event exists, **When** a client requests its URI with `Accept: text/turtle`, **Then** the system returns valid RDF Turtle serialization
4. **Given** a deleted event, **When** any client requests its URI, **Then** the system returns HTTP 410 Gone with a JSON-LD tombstone showing `eventStatus`, `deletedAt`, and optional `supersededBy`

---

### User Story 5 - Provenance Tracking and Source Attribution (Priority: P3)

Data consumers need to understand where event information came from, which source provided which fields, and the confidence level of the data. This enables trust decisions and attribution requirements.

**Why this priority**: Federation produces conflicts; provenance enables transparent resolution. CC0 compliance and attribution tracking are foundational to the data commons model.

**Independent Test**: An event with data from multiple sources can be queried to show field-level attribution (e.g., "name from source A, startDate from source B"), and the API response includes confidence scores and source metadata.

**Acceptance Scenarios**:

1. **Given** an event created from a source, **When** the event is retrieved via API, **Then** the response includes a `source` block with source URI, retrievedAt timestamp, and trust level
2. **Given** two sources provide conflicting information for the same event, **When** the system merges them, **Then** the winning value is chosen based on trust score, and field_provenance records which source contributed each field
3. **Given** an event exists, **When** a data consumer requests detailed provenance via extended API parameter, **Then** the response includes per-field attribution showing source, confidence, and timestamp for critical fields

---

### User Story 6 - Change Feed for Federation (Priority: P4)

Peer SEL nodes and external consumers need to sync changes efficiently without polling all events repeatedly. A cursor-based change feed enables incremental synchronization.

**Why this priority**: Federation requires efficient sync. Change feeds enable peer nodes and downstream systems to stay current with minimal bandwidth.

**Independent Test**: A consumer can request the change feed with a cursor, receive ordered changes since that point, process them, and use the returned `next_cursor` to continue syncing without missing or duplicating changes.

**Acceptance Scenarios**:

1. **Given** events have been created and modified, **When** a consumer requests GET `/api/v1/feeds/changes?since=seq_1000&limit=50`, **Then** the system returns ordered changes (create, update, delete) with action type, changed_at timestamp, and event snapshot
2. **Given** a consumer has a valid cursor, **When** events are modified after that cursor, **Then** the next request returns only the new changes, and the `next_cursor` advances correctly
3. **Given** an event is deleted, **When** the change feed includes that event, **Then** the action is "delete" and the snapshot includes tombstone information

---

### Edge Cases

- What happens when an agent submits an event with a future date beyond 2 years?
  - System accepts but flags for review with lower confidence
- How does the system handle submissions with invalid or expired external links (url, image_url)?
  - Link validation is deferred to background job; event is accepted with warning flag
- What happens when a source provides an event in a non-CC0 license?
  - System rejects at ingestion boundary with clear error message
- How does system handle concurrent updates to the same event?
  - Optimistic locking with version field; 409 Conflict on version mismatch
- What happens when content negotiation header is missing or invalid?
  - Default to `application/json` (aliased to JSON-LD)

## Clarifications

### Session 2026-01-23

- Q: When do submitted events become visible to public queries? Do agents submit directly to "published" or require admin approval? → A: Auto-publish with flagging — events publish immediately and are visible to public queries; low-confidence or flagged events appear in admin review queue for reactive oversight
- Q: How are API keys created and managed for agent authentication? → A: Admin-provisioned only — admins create API keys via admin UI/API; no self-service registration for MVP
- Q: What filters should the public query endpoint support? → A: Extended filters — date range (startDate/endDate), city/region, venue ID, organizer ID, lifecycle_state, free-text search on name/description, keywords/tags, and event domain (arts/music/culture/etc)
- Q: What rate limiting approach should be applied? → A: Role-based tiers — Public: 60 req/min, Agents: 300 req/min, Admins: unlimited

### Session 2026-01-24

- Q: What admin authentication approach should be used? → A: Local admin credentials stored in the database with `POST /api/v1/admin/login` issuing JWTs
- Q: How is the first admin created? → A: Bootstrap first admin from environment variables, then manage admins via admin UI/API
- Q: How should admin JWTs be transported? → A: Authorization header for API requests; HttpOnly cookie for admin HTML UI
- Q: Where should source vs server timestamps be recorded? → A: Both provenance fields and change feed entries should include source-provided and server-received timestamps
- Q: What database migration strategy should be used? → A: golang-migrate/migrate for sequential versioned migrations, supporting potential multi-database future
- Q: How should event occurrences be handled at MVP? → A: Multiple occurrences - events can have multiple occurrence records with API support for querying by occurrence date range to handle festivals, runs, and recurring events from the start
- Q: What level of error detail should be exposed in responses? → A: Environment-aware verbosity - stack traces and SQL errors in dev/test environments; sanitized messages only in production
- Q: How should background job failures be handled? → A: Exponential backoff with alerting - configurable max attempts per job type (deduplication: 1 attempt, reconciliation/enrichment: 5-10 attempts); alert admins on final failure
- Q: What defines "low-confidence or flagged events" for admin review? → A: Events with confidence score < 0.6, missing optional Schema.org fields (description, image, offers), unresolved external links (HTTP 4xx/5xx on image_url/url), or events with startDate >730 days in future

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept event submissions via `POST /api/v1/events` with Schema.org/Event compatible payloads
- **FR-002**: System MUST validate incoming events have required fields: `name`, `startDate`, and at least one of `location` or `virtualLocation`
- **FR-003**: System MUST generate ULID-based canonical URIs following pattern `https://{domain}/events/{ulid}`
- **FR-004**: System MUST provide content negotiation supporting `text/html`, `application/ld+json`, `application/json`, and `text/turtle`
- **FR-005**: System MUST track field-level provenance for all event data with source attribution
- **FR-006**: System MUST implement cursor-based pagination for all list endpoints with configurable limit (default 50, max 200)
- **FR-007**: System MUST authenticate agent writes via API keys provisioned by admins
- **FR-008**: System MUST authenticate admin access via JWT tokens
- **FR-009**: System MUST allow public read-only access without authentication
- **FR-010**: System MUST return RFC 7807 compliant error responses for all failures with environment-aware detail levels: stack traces and SQL errors in dev/test; sanitized messages only in production
- **FR-011**: System MUST return HTTP 410 Gone with JSON-LD tombstone for deleted entities
- **FR-012**: System MUST expose OpenAPI 3.1 specification at `/api/v1/openapi.json`
- **FR-013**: System MUST provide admin endpoints for event review, editing, lifecycle management, and API key provisioning
- **FR-014**: System MUST provide change feed endpoint with cursor-based sync for federation
- **FR-015**: System MUST reject events from non-CC0 compatible sources at ingestion boundary
- **FR-016**: System MUST render human-readable HTML pages for event URIs with embedded JSON-LD
- **FR-017**: System MUST provide health check endpoints at `/healthz` (liveness) and `/readyz` (readiness)
- **FR-018**: System MUST auto-publish submitted events immediately; low-confidence or flagged events also appear in admin review queue
- **FR-019**: System MUST support query filters: date range, city/region, venue ID, organizer ID, lifecycle_state, free-text search, keywords, and event domain
- **FR-020**: System MUST apply role-based rate limits: Public 60 req/min, Agents 300 req/min, Admins unlimited
- **FR-021**: System MUST accept authenticated federation sync submissions at `POST /api/v1/federation/sync` and preserve foreign `@id` values
- **FR-022**: System MUST validate canonical URI patterns and normalize `sameAs` links to full URIs
- **FR-023**: System MUST support multiple occurrences per event; queries filter by occurrence date ranges to handle recurring events, festivals, and rescheduling
- **FR-024**: System MUST include license information in JSON-LD responses
- **FR-025**: System MUST provide admin management for federation node registry
- **FR-026**: System MUST authenticate admins via local credentials stored in the database and issue JWTs from `POST /api/v1/admin/login`
- **FR-027**: System MUST bootstrap the first admin from environment variables and allow subsequent admin management via admin UI/API
- **FR-028**: System MUST accept admin JWTs via `Authorization: Bearer` for API requests and via HttpOnly cookie for the admin HTML UI
- **FR-029**: System MUST record both source-provided timestamps and server-received timestamps in provenance fields and in change feed entries
- **FR-030**: System MUST support Schema.org Place properties: `name`, `address` (PostalAddress with `streetAddress`, `addressLocality`, `addressRegion`, `postalCode`, `addressCountry`), `geo` (GeoCoordinates with `latitude`, `longitude`), `telephone`, `url`, and `sameAs` for external identifiers
- **FR-031**: System MUST support Schema.org Organization properties: `name`, `legalName`, `alternateName`, `description`, `email`, `telephone`, `url`, `address`, and `sameAs` for external identifiers

### Key Entities

- **Event**: A cultural or community happening with name, timing, location, organizer. Has lifecycle_state field (draft, published, cancelled, etc.) and quality indicators. Core entity of the system.
- **Event Occurrence**: A specific temporal instance of an event. Events can have multiple occurrences to handle recurring events, festival runs, and rescheduling. Queries filter by occurrence date ranges. At least one occurrence must exist per event.
- **Place**: A physical venue or location. Normalized to enable venue-based queries and reconciliation with external place identifiers (Artsdata, Wikidata).
- **Organization**: An entity that organizes events. Normalized for reconciliation and to enable "events by this organizer" queries.
- **Source**: A data provider (scraper, partner API, manual entry). Has trust level (1-10), license status, and metadata for attribution.
- **Field Provenance**: Per-field attribution tracking which source provided which value with confidence and timestamp.
- **API Key**: Authentication credential for agent access. Scoped to specific roles and rate limits.
- **Admin User**: Human account with JWT-based authentication for administrative access.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Agents can submit an event and retrieve it via API within 1 second end-to-end latency (p95)
- **SC-002**: System handles 100 concurrent event submissions without errors or significant degradation
- **SC-003**: Public query endpoint returns filtered results within 500ms for date-range queries over 10,000 events (p95)
- **SC-004**: Content negotiation returns correct format for 100% of requests with valid Accept headers
- **SC-005**: Admin can review, edit, and approve an event within a single browser session (no multi-page workflows)
- **SC-006**: All submitted events have complete provenance tracking (source, timestamp, trust level)
- **SC-007**: Change feed consumers can sync 1000 changes in under 2 seconds using cursor pagination
- **SC-008**: Deleted entities return 410 Gone with valid tombstone for 100% of delete operations
- **SC-009**: System passes SHACL validation against Artsdata event shapes for all exported JSON-LD
- **SC-010**: OpenAPI specification is always in sync with implemented endpoints (validated in CI)

## Assumptions

The following assumptions were made based on the architecture documentation and SEL Interoperability Profile:

- **Authentication method**: API keys for agents, JWT for admin (as specified in architecture doc)
- **Rate limiting**: Per-role rate limits applied but specific limits determined during implementation
- **Embedding/vector search**: Not in MVP scope for this specification; deferred to later feature
- **Artsdata reconciliation**: Background enrichment exists but automatic reconciliation is out of scope for MVP
- **Admin frontend**: Minimal HTML-based interface embedded in Go binary; not a separate SPA application
- **Database**: PostgreSQL with JSONB and PostGIS; SQLc for type-safe queries
- **Database migrations**: golang-migrate/migrate for sequential versioned migrations to support potential multi-database expansion
- **Duplicate detection**: Layered strategy per architecture doc (exact hash, fuzzy matching, vector similarity) with `duplicate_candidates` table and admin review workflow
- **Timezone handling**: Events store UTC + timezone string + computed local time as specified in schema design
- **Background jobs**: River queue for async work with exponential backoff and job-type-specific retry limits (deduplication: 1 attempt, reconciliation/enrichment: 5-10 attempts); admin alerts on final failure

## Out of Scope

The following capabilities are explicitly excluded from this specification:

- Vector/semantic search (future feature)
- Automatic Artsdata reconciliation (requires separate integration work)
- Full multi-node federation sync protocol is deferred; a minimal sync endpoint for peer submissions is in scope
- MCP server for LLM agents (future feature)
- Public event submission forms (agents only for MVP)
- Full-featured admin SPA (minimal embedded HTML only)
- Email notifications or alerts
- User accounts beyond admin (no public user registration)
