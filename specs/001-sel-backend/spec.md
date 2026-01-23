# Feature Specification: SEL Backend Server with Admin Frontend

**Feature Branch**: `001-sel-backend`  
**Created**: 2026-01-23  
**Status**: Draft  
**Input**: User description: "Go backend server for Shared Events Library with admin frontend for event management"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Submit and Query Events via API (Priority: P1)

An external event aggregation agent (scraper, partner system, or data provider) needs to submit event data to SEL and retrieve events for downstream consumption. This is the foundational capability that makes SEL useful as infrastructure.

**Why this priority**: Without event ingestion and retrieval, there is no events library. This is the core data flow that all other capabilities depend on.

**Independent Test**: An agent can POST a valid Schema.org/Event payload to `/api/v1/events`, receive a 201 Created response with the event's canonical URI, and immediately GET that event back with full JSON-LD output. The event persists across server restarts.

**Acceptance Scenarios**:

1. **Given** an authenticated agent with valid API key, **When** the agent POSTs a JSON-LD event payload with name, startDate, and location, **Then** the system returns 201 Created with the event's canonical `@id` URI and the event is queryable via GET
2. **Given** a public user with no authentication, **When** the user requests GET `/api/v1/events?startDate=2026-02-01`, **Then** the system returns a paginated list of events in JSON-LD format with proper `@context`
3. **Given** an event submission with missing required field (startDate), **When** the agent POSTs, **Then** the system returns 400 Bad Request with RFC 7807 error envelope explaining the validation failure
4. **Given** multiple events exist, **When** a client requests GET `/api/v1/events/{ulid}`, **Then** the system returns the single event with full details including provenance metadata

---

### User Story 2 - Admin Reviews and Edits Events (Priority: P2)

An administrator or editor needs to review submitted events, correct data quality issues, approve pending submissions, and manage event lifecycle (cancel, mark complete, merge duplicates). This enables human oversight of automated ingestion.

**Why this priority**: Automated ingestion requires human quality control. Admins need to fix errors, handle edge cases, and maintain data quality standards.

**Independent Test**: An admin can log in, view a list of pending/problematic events, edit an event's details, and see those changes reflected in API responses. Changes are logged with admin attribution.

**Acceptance Scenarios**:

1. **Given** an admin user with valid credentials, **When** the admin requests GET `/api/v1/admin/events/pending`, **Then** the system returns events awaiting review with quality indicators
2. **Given** an event with incorrect venue information, **When** the admin PUTs corrected data to `/api/v1/admin/events/{id}`, **Then** the event is updated, version incremented, and change logged with admin attribution
3. **Given** two duplicate events detected by the system, **When** the admin views GET `/api/v1/admin/duplicates` and confirms the merge, **Then** one event becomes canonical with `sameAs` links and the other returns 410 Gone with tombstone
4. **Given** an event that should be cancelled, **When** the admin updates lifecycle_state to "cancelled", **Then** the event shows `eventStatus: EventCancelled` in JSON-LD output

---

### User Story 3 - Content Negotiation and Dereferenceable URIs (Priority: P3)

External systems (knowledge graphs, search engines, linked data consumers) need to resolve SEL URIs and receive appropriate representations based on their Accept header. Humans browsing should see HTML; machines should get JSON-LD or Turtle.

**Why this priority**: SEL URIs must be dereferenceable per linked data principles. This enables interoperability with Artsdata, Wikidata, and other knowledge graphs.

**Independent Test**: A browser request to `https://{domain}/events/{ulid}` returns human-readable HTML with embedded JSON-LD, while a curl request with `Accept: application/ld+json` returns pure JSON-LD.

**Acceptance Scenarios**:

1. **Given** an event exists, **When** a client requests its URI with `Accept: text/html`, **Then** the system returns an HTML page showing name, startDate, location, organizer, with JSON-LD embedded in a `<script type="application/ld+json">` tag
2. **Given** an event exists, **When** a client requests its URI with `Accept: application/ld+json`, **Then** the system returns canonical JSON-LD with proper `@context` and `@id`
3. **Given** an event exists, **When** a client requests its URI with `Accept: text/turtle`, **Then** the system returns valid RDF Turtle serialization
4. **Given** a deleted event, **When** any client requests its URI, **Then** the system returns HTTP 410 Gone with a JSON-LD tombstone showing `eventStatus`, `deletedAt`, and optional `supersededBy`

---

### User Story 4 - Provenance Tracking and Source Attribution (Priority: P3)

Data consumers need to understand where event information came from, which source provided which fields, and the confidence level of the data. This enables trust decisions and attribution requirements.

**Why this priority**: Federation produces conflicts; provenance enables transparent resolution. CC0 compliance and attribution tracking are foundational to the data commons model.

**Independent Test**: An event with data from multiple sources can be queried to show field-level attribution (e.g., "name from source A, startDate from source B"), and the API response includes confidence scores and source metadata.

**Acceptance Scenarios**:

1. **Given** an event created from a source, **When** the event is retrieved via API, **Then** the response includes a `source` block with source URI, retrievedAt timestamp, and trust level
2. **Given** two sources provide conflicting information for the same event, **When** the system merges them, **Then** the winning value is chosen based on trust score, and field_provenance records which source contributed each field
3. **Given** an event exists, **When** a data consumer requests detailed provenance via extended API parameter, **Then** the response includes per-field attribution showing source, confidence, and timestamp for critical fields

---

### User Story 5 - Change Feed for Federation (Priority: P4)

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

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept event submissions via `POST /api/v1/events` with Schema.org/Event compatible payloads
- **FR-002**: System MUST validate incoming events have required fields: `name`, `startDate`, and at least one of `location` or `virtualLocation`
- **FR-003**: System MUST generate ULID-based canonical URIs following pattern `https://{domain}/events/{ulid}`
- **FR-004**: System MUST provide content negotiation supporting `text/html`, `application/ld+json`, `application/json`, and `text/turtle`
- **FR-005**: System MUST track field-level provenance for all event data with source attribution
- **FR-006**: System MUST implement cursor-based pagination for all list endpoints with configurable limit (default 50, max 200)
- **FR-007**: System MUST authenticate agent writes via API keys
- **FR-008**: System MUST authenticate admin access via JWT tokens
- **FR-009**: System MUST allow public read-only access without authentication
- **FR-010**: System MUST return RFC 7807 compliant error responses for all failures
- **FR-011**: System MUST return HTTP 410 Gone with JSON-LD tombstone for deleted entities
- **FR-012**: System MUST expose OpenAPI 3.1 specification at `/api/v1/openapi.json`
- **FR-013**: System MUST provide admin endpoints for event review, editing, and lifecycle management
- **FR-014**: System MUST provide change feed endpoint with cursor-based sync for federation
- **FR-015**: System MUST reject events from non-CC0 compatible sources at ingestion boundary
- **FR-016**: System MUST render human-readable HTML pages for event URIs with embedded JSON-LD
- **FR-017**: System MUST provide health check endpoints at `/healthz` (liveness) and `/readyz` (readiness)

### Key Entities

- **Event**: A cultural or community happening with name, timing, location, organizer. Has lifecycle states (draft, published, cancelled, etc.) and quality indicators. Core entity of the system.
- **Event Occurrence**: A specific temporal instance of an event. Separates identity from timing to handle reschedules, recurring events, and postponements cleanly.
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
- **Timezone handling**: Events store UTC + timezone string + computed local time as specified in schema design
- **Background jobs**: River queue for async work (reconciliation, cleanup) but minimal MVP scope

## Out of Scope

The following capabilities are explicitly excluded from this specification:

- Vector/semantic search (future feature)
- Automatic Artsdata reconciliation (requires separate integration work)
- Multi-node federation sync protocol (change feed enables it; actual sync is future)
- MCP server for LLM agents (future feature)
- Public event submission forms (agents only for MVP)
- Full-featured admin SPA (minimal embedded HTML only)
- Email notifications or alerts
- User accounts beyond admin (no public user registration)
