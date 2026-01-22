# SEL Implementation Plan & Checklist

**Version:** 1.0  
**Date:** 2026-01-20  
**Status:** Comprehensive Implementation Roadmap

---

## Overview

This document provides the prioritized implementation checklist for the Shared Events Library (SEL) backend, incorporating critical gaps identified in the architecture review and requirements from the SEL Interoperability Profile v0.1.

---

## Phase 0: Foundational Contracts (BEFORE CODING)

**Goal:** Define all interoperability contracts and core data models before implementation begins.

### 0.1 URI and Identifier Strategy
- [ ] **Define canonical URI namespace pattern**
  - Format: `https://{node-domain}/{entity-type}/{ulid}`
  - Configure node domain per deployment
  - Document ULID generation (timestamp + random)
  - Require true ULIDs (not UUID re-encodings)
- [ ] **Establish identifier roles**
  - `@id` = canonical SEL URI
  - `url` = public human-readable webpage
  - `sameAs` = external authority links (Artsdata, Wikidata, ISNI, MusicBrainz)
  - Normalize `sameAs` to full URIs (e.g., `http://www.wikidata.org/entity/Q...`)
- [ ] **Define federation rules**
  - MUST preserve origin node URIs when syncing
  - Document cross-node reference patterns
- [ ] **Create URI validation regex patterns**
  - Artsdata: `^http://kg\.artsdata\.ca/resource/K\d+-\d+$`
  - Wikidata: `^http://www\.wikidata\.org/entity/Q\d+$`
  - Internal: `^https://{node-domain}/(events|places|organizations|persons)/[0-9A-Z]{26}$`

### 0.2 JSON-LD Context and Framing
- [ ] **Create versioned JSON-LD context**
  - Host at `https://schema.togather.foundation/context/v1.jsonld`
  - Define `sel:` namespace extensions
  - Document context evolution policy
- [ ] **Define canonical JSON-LD frames**
  - Event frame (`frames/event-v1.frame.json`)
  - Place frame (`frames/place-v1.frame.json`)
  - Organization frame (`frames/organization-v1.frame.json`)
  - EventSeries frame for recurring events
- [ ] **Specify minimal required fields per entity type**
  - Event: `@id`, `@type` (`Event`, `EventSeries`, or `Festival`), `name` (max 500), `startDate` (ISO 8601 with timezone), `location` (Place/VirtualLocation/URI)
  - Place: `@id`, `@type`, `name` (max 300), `address` OR `geo`
  - Organization: `@id`, `@type`, `name` (max 300)
  - Attendance rules: online events require `VirtualLocation` with `url`; mixed SHOULD include both Place + VirtualLocation
- [ ] **Document optional enrichment fields**
  - Recommended vs optional distinction
  - Mapping to schema.org properties

### 0.3 Event Lifecycle and Temporal Model
- [ ] **Design event/occurrence split architecture**
  - `events` table = canonical event identity
  - `event_occurrences` table = temporal instances
  - Document relationship patterns
- [ ] **Define temporal representation strategy**
  - Store UTC timestamp (`TIMESTAMPTZ`)
  - Store IANA timezone identifier (`TEXT`)
  - Generate local date/time fields for query optimization
  - Handle DST transitions correctly
- [ ] **Define lifecycle states**
  - States: `draft`, `published`, `postponed`, `rescheduled`, `sold_out`, `cancelled`, `completed`, `deleted`
  - Document state transition rules
  - Define which states require occurrences
- [ ] **Design recurring event model**
  - MVP: Pre-generate occurrences (6 months ahead)
  - Use `event_series` table for patterns
  - Document expansion job mechanics
  - Plan future RRULE support (deferred)
  - JSON-LD output: EventSeries MUST include `eventSchedule`
  - EventSeries MAY include bounded `subEvent` list (configurable window)
  - Serialize occurrences as `Event` with `superEvent` pointing to series; allow location overrides

### 0.4 Provenance and Trust Model
- [ ] **Design sources registry**
  - Table: `sources` with trust levels (1-10)
  - Fields: `id`, `name`, `type`, `base_url`, `license`, `trust_level`, `contact`
  - Define source types: `scraper`, `partner`, `user`, `federation`
- [ ] **Design field-level provenance tracking**
  - Table: `field_provenance` or `field_claims`
  - Track: event_id, field_path (JSON Pointer), value_hash, source_id, confidence, observed_at
  - Document when to track (critical fields only vs all)
- [ ] **Define conflict resolution policy**
  - Merge algorithm: trust_level DESC, confidence DESC, observed_at DESC
  - Never overwrite higher-trust values with lower-trust values
  - Document when to flag for manual review
  - Define conflict notification mechanism
- [ ] **Design observation storage**
  - Table: `event_sources` for immutable observations
  - Store raw payloads with hash for deduplication
  - Link to source registry
- [ ] **Document license tracking per source**
  - Validate CC0 compatibility
  - Define policy for non-CC0 sources (facts only, summarize descriptions)
  - Store license URL per source

### 0.5 SHACL Validation Strategy
- [ ] **Select SHACL shapes for validation**
  - Pin Artsdata shapes versions
  - Define custom SEL shapes for extensions
  - Store shapes in `/shapes/` directory
- [ ] **Choose SHACL validation library**
  - Evaluate Go SHACL libraries or call external validator
  - Document validation integration points
- [ ] **Define validation pipeline**
  - When to validate: on ingestion, on export, on demand
  - How to handle validation failures
  - Store validation reports as artifacts
- [ ] **CI integration**
  - Validate all test fixtures against shapes
  - Block merges on validation failures
  - Generate validation reports in CI

### 0.6 Federation Fundamentals
- [ ] **Design node identity model**
  - Table: `nodes` for federation partners
  - Fields: `id`, `domain`, `name`, `api_base_url`, `trust_level`, `last_sync_at`
  - Configure local node identity
- [ ] **Define cross-node event identity**
  - Use `origin_node_id` to track source
  - Preserve original URIs from federated nodes
  - Use `sameAs` for cross-node equivalence
- [ ] **Design simple sync protocol (pre-ActivityPub)**
  - Bulk export endpoint: `GET /api/v1/exports/events.jsonld`
  - Change feed endpoint: `GET /api/v1/feeds/changes?since={cursor}&limit={n}`
  - Export query parameters: `changed_since`, `start_from`, `start_to`, `include_deleted`
  - Authenticate federation requests via API keys (`Authorization: Bearer <key>`)
- [ ] **Design change capture mechanism**
  - Table: `event_changes` outbox
  - Fields: `id`, `seq`, `event_id`, `action`, `changed_fields JSONB`, `snapshot JSONB`, `tombstone JSONB`, `created_at`
  - Enable webhook notifications
  - Support ActivityPub outbox generation (future)

### 0.7 License and Policy Documentation
- [ ] **Define ingestion license policy**
  - Accept: CC0, CC-BY, public facts
  - Reject: proprietary copyrighted content
  - Transform: LLM-summarize descriptions from non-CC0 sources
  - If source is restrictive: ingest facts only; retain descriptions with takedown flags + provenance
- [ ] **Define publication policy**
  - All outputs under CC0 1.0 Universal
  - Include license in every JSON-LD document
  - Dataset-level metadata includes license
- [ ] **Define license flags and takedown handling**
  - `sel:licenseStatus` values: `cc0`, `cc-by`, `proprietary`, `unknown`
  - `sel:takedownRequested` boolean + timestamp
  - Support removal on request
- [ ] **Create attribution guidelines**
  - Requested format: "Shared Events Library - {City Node}"
  - Document provenance blocks in JSON-LD

### 0.8 Content Negotiation and HTML Rendering
- [ ] **Define content negotiation rules**
  - `text/html`, `application/ld+json`, `application/json`, `text/turtle`
  - Require dereferenceable URIs to support HTML + JSON-LD
- [ ] **Specify HTML minimums**
  - Render `name`, `startDate`, `location`, `organizer` when available
  - Embed canonical JSON-LD in the page

---

## Phase 1: Core Implementation

**Goal:** Build foundational data storage, ingestion pipeline, and basic API.

### 1.1 Database Schema Implementation
- [ ] **Create migration framework setup**
  - Choose migration tool (golang-migrate, goose, atlas)
  - Initialize migrations directory
  - Document migration workflow
- [ ] **Implement events and occurrences tables**
  - `events` table with lifecycle fields
  - `event_occurrences` table with temporal fields
  - Proper indexes for time-range queries
  - Soft delete support (`deleted_at`)
- [ ] **Implement entity tables**
  - `places` table with address and geo fields
  - `organizations` table
  - `persons` table (for performers/organizers)
- [ ] **Implement provenance tables**
  - `sources` registry
  - `event_sources` observations
  - `field_provenance` tracking
- [ ] **Implement federation tables**
  - `nodes` registry
  - `event_changes` outbox
  - `tombstones` for soft deletes
- [ ] **Implement identifier normalization**
  - `entity_identifiers` table
  - Schema: `entity_type`, `entity_id`, `scheme`, `uri`, `confidence`, `source`
- [ ] **Implement reconciliation cache**
  - `reconciliation_cache` table
  - Fields: `lookup_key`, `entity_type`, `candidates JSONB`, `decision JSONB`, `cached_at`, `expires_at`
- [ ] **Add JSONB support**
  - Store original payloads
  - Use for flexible provenance metadata
- [ ] **Set up SQLc code generation**
  - Write queries in `.sql` files
  - Generate type-safe Go code
  - Document query organization

### 1.2 Job Queue Infrastructure
- [ ] **Set up River job queue**
  - Configure database-backed queue
  - Define job types and handlers
  - Set up worker pools
- [ ] **Implement transactional job enqueueing**
  - Enqueue jobs in same transaction as data changes
  - Document patterns for reliable background work
- [ ] **Create job monitoring dashboard**
  - Track job status, retries, failures
  - Alerting on job failures
- [ ] **Implement core job types**
  - Reconciliation jobs
  - Vector index update jobs
  - Occurrence expansion jobs (for series)
  - Bulk import processing jobs

### 1.3 Vector Search Implementation
- [ ] **Set up pgvector extension**
  - Install extension in PostgreSQL
  - Create embeddings column
  - Add vector indexes
- [ ] **Choose embedding model**
  - Evaluate Ollama with local models
  - Document model selection rationale
  - Set up model serving (sidecar or API)
- [ ] **Implement embedding generation**
  - Job for generating embeddings
  - Retry logic for failures
  - Batch processing for efficiency
- [ ] **Implement vector search queries**
  - Similarity search via SQL
  - Hybrid search (vector + filters)
  - Result ranking and scoring
- [ ] **Consider USearch acceleration (production)**
  - In-memory index as cache
  - Rebuild from DB on startup
  - Sync via River jobs
  - Document hybrid approach

### 1.4 Core API Server
- [ ] **Set up Huma framework**
  - Configure with Chi router
  - Auto-generate OpenAPI 3.1 spec
  - Set up content negotiation
- [ ] **Implement health check endpoint**
  - `GET /health` (liveness)
  - `GET /readyz` (readiness - checks DB, queue)
- [ ] **Implement authentication middleware**
  - API key validation
  - JWT verification (for admin)
  - Role extraction and context passing
- [ ] **Implement RBAC middleware**
  - Role-based route protection
  - Per-endpoint permission checks
- [ ] **Implement rate limiting**
  - Per-API-key rate limits
  - Configurable limits per role
  - Return 429 with Retry-After header
- [ ] **Implement idempotency middleware**
  - Support `Idempotency-Key` header
  - Cache responses in database or Redis
  - Document idempotency semantics and TTL
- [ ] **Implement request logging**
  - Structured logging with request ID
  - Log authentication info
  - Performance metrics

### 1.5 Event Ingestion Pipeline
- [ ] **Implement event submission endpoint**
  - `POST /api/v1/events`
  - Request validation against schema
  - Idempotency support
  - Transactional create + job enqueue
- [ ] **Implement bulk submission endpoint**
  - `POST /api/v1/events:batch`
  - Return 202 with job ID
  - Progress tracking endpoint
- [ ] **Implement validation layer**
  - Schema.org field validation
  - Business rule validation (e.g., start <= end)
  - External ID format validation
  - Use go-playground/validator
  - Enforce online/mixed attendance location rules
- [ ] **Implement normalization**
  - Date/time parsing and timezone handling
  - Text cleaning (trim, normalize whitespace)
  - Address normalization
  - URL validation and normalization
- [ ] **Implement deduplication**
  - Identity key: normalized name + startDate (±15 min, round to 5 min) + location/virtual_url
  - If series, include occurrence index in identity key
  - Similarity detection (fuzzy match)
  - Merge vs reject decisions
  - Deduplication audit trail
- [ ] **Implement source tracking**
  - Record source metadata
  - Store raw payload
  - Link to source registry
  - Require ingestion from a registered source
- [ ] **Enqueue enrichment jobs**
  - Reconciliation job
  - Embedding generation job
  - Image processing (if applicable)

### 1.6 Basic Query API
- [ ] **Implement event listing endpoint**
  - `GET /api/v1/events`
  - Filter by date range
  - Filter by location (city, venue)
  - Filter by status
  - Cursor-based pagination
  - Response envelope: `items[]` + `next_cursor`
  - Pagination defaults: limit 50, max 200
- [ ] **Implement event detail endpoint**
  - `GET /api/v1/events/{id}`
  - Support both UUID and ULID
  - Handle tombstones (410 Gone)
- [ ] **Implement JSON-LD output**
  - Content negotiation for `application/ld+json`
  - Use piprate/json-gold for framing
  - Apply versioned frames
  - Include @context in responses
  - Include license in every JSON-LD response
  - Include `prov:wasDerivedFrom` when source is known
  - Provide `text/html` rendering for dereferenceable URIs
  - Embed canonical JSON-LD in HTML pages
- [ ] **Implement place listing/detail**
  - `GET /api/v1/places`
  - `GET /api/v1/places/{id}`
- [ ] **Implement organization listing/detail**
  - `GET /api/v1/organizations`
  - `GET /api/v1/organizations/{id}`
- [ ] **Publish OpenAPI reference**
  - `GET /api/v1/openapi.json`
- [ ] **Standardize error responses**
  - RFC 7807 problem+json envelope

### 1.7 Soft Delete and Tombstones
- [ ] **Implement soft delete mechanism**
  - Set `deleted_at` timestamp
  - Lifecycle state = `deleted`
  - Preserve data for audit
- [ ] **Implement tombstone generation**
  - Trigger on soft delete
  - Store minimal tombstone JSON-LD
  - Record deletion reason and timestamp
  - Include `eventStatus`, `sel:tombstone`, `sel:deletedAt`, `sel:deletionReason`, `sel:supersededBy` when applicable
- [ ] **Implement tombstone endpoint behavior**
  - Return 410 Gone for deleted entities
  - Return tombstone JSON-LD
  - Include `supersededBy` if merged

---

## Phase 2: Artsdata Integration & Enrichment

**Goal:** Integrate with Artsdata for entity reconciliation and enrichment.

### 2.1 Artsdata Client Library
- [ ] **Implement reconciliation API client**
  - Use go-retryablehttp or resty
  - Implement exponential backoff
  - Handle rate limiting (429 responses)
  - Configurable timeout and retry policy
  - Follow W3C Reconciliation API spec
- [ ] **Implement request builder**
  - Build reconciliation requests per entity type
  - Include disambiguating properties
  - Document required vs optional properties
- [ ] **Implement response parser**
  - Parse candidate matches
  - Extract confidence scores
  - Store match metadata
- [ ] **Implement SPARQL query client**
  - Query Artsdata for enrichment data
  - Support both core and all-graphs
  - Parse JSON-LD responses
- [ ] **Implement graph scoping strategy**
  - Document when to use core vs all-graphs
  - Track provenance of facts by graph
- [ ] **Implement Query API client (optional)**
  - Support event list retrieval with source graph scoping
  - Allow selecting frames and formats per Artsdata docs

### 2.2 Reconciliation Service
- [ ] **Implement reconciliation job handler**
  - Process events from queue
  - Reconcile venue (place)
  - Reconcile organizer (organization)
  - Reconcile performers (persons)
- [ ] **Implement cache lookup**
  - Check reconciliation_cache before API call
  - Use normalized lookup key
  - Respect cache TTL
- [ ] **Define deterministic resolution order**
  - Use existing Artsdata URI if present
  - Otherwise reconcile, then mint only when no match exists
- [ ] **Implement confidence thresholds**
  - `auto_high`: >= 95% + match=true → accept
  - `auto_low`: 80-94% → flag for review
  - `reject`: < 80% → no match
  - Make thresholds configurable per entity type
- [ ] **Enforce Artsdata minting rule**
  - Do not mint Artsdata IDs if any candidate >= auto_low unless admin overrides with audit trail
- [ ] **Implement decision recording**
  - Store decision in cache
  - Record method (auto_high, auto_low, manual)
  - Track confidence score
- [ ] **Implement identifier storage**
  - Insert into `entity_identifiers` table
  - Set scheme = "artsdata", "wikidata", etc.
  - Record confidence and source
- [ ] **Implement negative caching**
  - Cache "no match" results
  - Shorter TTL for negatives
  - Avoid repeated failed lookups
- [ ] **Define reconciliation cache normalization + TTLs**
  - Normalize type/name/url/locality/postal code
  - Positive TTL: 30 days, negative TTL: 7 days
  - Cache lookups must be deterministic for same normalized key
- [ ] **Implement manual review workflow**
  - Flag low-confidence matches
  - Admin endpoint to approve/reject
  - Audit trail of decisions

### 2.3 Enrichment Service
- [ ] **Implement venue enrichment**
  - Fetch place data from Artsdata
  - Extract: coordinates, address, capacity, official name
  - Merge with local data (conflict resolution)
- [ ] **Implement organization enrichment**
  - Fetch org data from Artsdata
  - Extract: official name, website, alternate names
  - Handle sameAs links to Wikidata
- [ ] **Implement person enrichment**
  - Fetch person data from Artsdata
  - Extract: official name, roles
  - Link to external authorities (ISNI, MusicBrainz)
- [ ] **Implement merge policy**
  - Higher trust sources win
  - Don't overwrite high-trust with low-trust
  - Flag conflicts for review
  - Preserve all observations

### 2.4 Artsdata Contribution (Optional)
- [ ] **Define Databus publishing workflow**
  - Publish RDF dataset artifacts (JSON-LD, Turtle, N-Quads)
  - Ensure dataset URL remains downloadable
  - Provide dataset metadata for Databus
- [ ] **Enforce contribution constraints**
  - Do not upload triples with `kg.artsdata.ca` subject URIs
  - Do not upload ontologies via Databus
  - Validate with SHACL before upload
  - Require credentials (X-API-KEY or WebID)
- [ ] **Implement Artsdata minting integration (optional)**
  - Mint only after reconciliation yields no match
  - Validate Artsdata ID format on receipt

### 2.5 SHACL Validation
- [ ] **Integrate SHACL validator**
  - Choose library (or external service)
  - Validate on ingestion (optional)
  - Validate on export (required)
- [ ] **Implement validation in CI**
  - Validate test fixtures
  - Validate generated examples
  - Fail CI on invalid data
- [ ] **Store validation reports**
  - Generate report files
  - Include in dataset exports
  - Track validation status per event

---

## Phase 3: Federation & Data Publishing

**Goal:** Enable multi-node federation and bulk data export.

### 3.1 Bulk Export API
- [ ] **Implement dataset export endpoint**
  - `GET /api/v1/exports/events.jsonld`
  - Support filters: `changed_since`, `start_from`, `start_to`
  - Include/exclude deleted events (default false)
  - Single JSON-LD graph output
- [ ] **Implement NDJSON export**
  - `GET /api/v1/exports/events.ndjson`
  - Newline-delimited JSON-LD
  - Better for streaming and bulk processing
- [ ] **Implement compressed dumps**
  - `GET /datasets/events.jsonld.gz`
  - Nightly scheduled exports
  - Include checksums and signatures
- [ ] **Implement Turtle export**
  - `GET /api/v1/exports/events.ttl`
  - RDF serialization
  - Use piprate/json-gold for conversion
- [ ] **Implement N-Triples export**
  - `GET /api/v1/exports/events.nt`
  - For RDF tooling compatibility
- [ ] **Support export content types**
  - JSON-LD (`application/ld+json`)
  - JSON (`application/json`)
  - Turtle (`text/turtle`)
  - N-Triples (`application/n-triples`)
  - NDJSON (`application/x-ndjson`)

### 3.2 Change Feed API
- [ ] **Implement change feed endpoint**
  - `GET /api/v1/feeds/changes`
  - Query param: `since={cursor}`, `limit={n}`
  - Return ordered change envelopes with `cursor` and `next_cursor`
- [ ] **Implement cursor-based pagination**
  - Opaque but stable cursors
  - Based on per-node monotonic sequence
  - Document cursor format
- [ ] **Implement change envelope format**
  - Include: action, event_id, event_uri, changed_at, changed_fields
  - Require `snapshot` for create/update
  - Require `tombstone` for delete
  - Embed entity data or reference only
  - Handle deletes/tombstones
- [ ] **Guarantee delete visibility**
  - Emit delete changes even if tombstone-only
- [ ] **Implement change capture**
  - Use `event_changes` outbox table
  - Record on create/update/delete
  - Capture changed fields as JSONB

### 3.3 Inter-Node Sync
- [ ] **Implement profile discovery endpoint**
  - `GET /.well-known/sel-profile`
  - Return `{ node, version, profile }`
- [ ] **Implement sync protocol**
  - `POST /api/v1/federation/sync`
  - Accept bulk event submissions from peer nodes
  - Validate source node identity
  - Ensure sync is idempotent; do not advance cursor past failed item
  - Authenticate peer requests with API keys (Authorization Bearer)
- [ ] **Implement origin preservation**
  - Store `origin_node_id` for federated events
  - Never mint URIs with another node's domain
  - Preserve original `@id` values
- [ ] **Implement node registry**
  - Admin API to register peer nodes
  - Store: domain, trust level, API credentials
  - Track last successful sync
- [ ] **Implement conflict strategy**
  - Origin node wins for its events
  - Use source trust + provenance for merges
  - Flag conflicts for review
- [ ] **Implement sync monitoring**
  - Track sync status and errors
  - Alert on sync failures
  - Metrics for lag and throughput

### 3.4 Webhook System
- [ ] **Implement webhook subscription API**
  - `POST /api/v1/webhooks/subscriptions`
  - Subscribe to event types (create, update, delete)
  - Filter by entity type or criteria
- [ ] **Implement webhook delivery**
  - Queue webhook jobs in River
  - Retry with exponential backoff
  - Dead letter queue for failures
- [ ] **Implement webhook payload format**
  - Include event envelope
  - Signature for verification (HMAC)
  - Delivery attempt metadata
- [ ] **Implement webhook management**
  - List subscriptions
  - Test webhook endpoint
  - Delete subscription

---

## Phase 4: Semantic Search & Advanced Features

**Goal:** Enable semantic search and agentic interfaces.

### 4.1 Semantic Search Implementation
- [ ] **Implement search endpoint**
  - `GET /api/v1/events/search`
  - Query parameter: `query="{text}"`
  - Optional filters: date range, location
- [ ] **Implement query embedding**
  - Generate embedding for query text
  - Use same model as event embeddings
  - Cache frequent queries
- [ ] **Implement similarity search**
  - Query pgvector for nearest neighbors
  - Apply filters in SQL
  - Return with similarity scores
- [ ] **Implement hybrid search**
  - Combine vector search with keyword search
  - Weighted scoring
  - Configurable strategy
- [ ] **Implement result ranking**
  - Consider recency
  - Consider popularity/engagement
  - User preference signals (future)

### 4.2 MCP Server (Core Integration)
- [ ] **Implement embedded MCP server**
  - Run alongside HTTP server
  - Share database and service layer
  - Expose tool-based interface
- [ ] **Implement core tools**
  - `find_events(query: string, filters: object)`
  - `get_event(id: string)`
  - `search_places(query: string)`
  - `reconcile_entity(type: string, name: string, properties: object)`
- [ ] **Implement tool result formatting**
  - Return summaries, not raw JSON
  - Natural language descriptions
  - Agent-friendly error messages
- [ ] **Implement agent context endpoint**
  - `GET /agent-context.md`
  - Natural language schema documentation
  - Examples and patterns
  - Domain rules and constraints
- [ ] **Implement LLM-optimized outputs**
  - Simplified result formats
  - Markdown tables
  - Structured but readable

### 4.3 Advanced Admin Features
- [ ] **Implement event merge tool**
  - Admin API to merge duplicate events
  - Create tombstone for merged event
  - Preserve provenance from both
- [ ] **Implement manual reconciliation UI**
  - Review low-confidence matches
  - Approve or reject suggestions
  - Override auto-decisions with audit trail
- [ ] **Implement bulk edit tools**
  - Update multiple events
  - Batch reassignment (venue, organizer)
  - Audit log of changes
- [ ] **Implement data quality dashboard**
  - Metrics: completeness, enrichment status
  - Validation failure rates
  - Source quality scores
- [ ] **Implement provenance visualization**
  - Show field-level sources
  - Confidence scores
  - Conflict indicators

---

## Phase 5: Production Hardening

**Goal:** Prepare for production deployment with monitoring, performance, and reliability.

### 5.1 Observability
- [ ] **Implement OpenTelemetry**
  - Distributed tracing
  - Trace ingestion pipeline
  - Trace external API calls (Artsdata)
- [ ] **Implement structured logging**
  - Use zerolog or zap
  - Consistent log levels
  - Request ID propagation
- [ ] **Implement Prometheus metrics**
  - API latency histograms
  - Request rate counters
  - Error rate counters
  - Job queue metrics
  - Database pool metrics
- [ ] **Implement health check details**
  - Separate liveness and readiness
  - Check database connectivity
  - Check job queue status
  - Include version info
- [ ] **Set up alerting**
  - High error rates
  - Job queue backlog
  - Slow queries
  - External service failures

### 5.2 Performance Optimization
- [ ] **Optimize database queries**
  - Add missing indexes
  - Use EXPLAIN ANALYZE
  - Query optimization for hot paths
- [ ] **Implement query result caching**
  - Redis or in-memory cache
  - Cache popular queries
  - Cache-Control headers
- [ ] **Implement connection pooling**
  - pgx pool configuration
  - Optimal pool size tuning
  - Monitor pool exhaustion
- [ ] **Implement batch processing**
  - Bulk insert optimizations
  - Batch embedding generation
  - Batch reconciliation requests
- [ ] **Profile application**
  - CPU profiling (pprof)
  - Memory profiling
  - Identify bottlenecks

### 5.3 Reliability
- [ ] **Implement graceful shutdown**
  - Wait for in-flight requests
  - Drain job workers
  - Close connections cleanly
- [ ] **Implement circuit breakers**
  - For Artsdata API calls
  - For embedding service calls
  - Fail fast on degraded services
- [ ] **Implement timeout configuration**
  - Request timeouts
  - Database query timeouts
  - External API timeouts
- [ ] **Implement retry policies**
  - Idempotent operation retries
  - Exponential backoff
  - Max retry limits
- [ ] **Implement request validation**
  - Input sanitization
  - SQL injection prevention
  - XSS prevention
  - Rate limit bypass prevention

### 5.4 Security
- [ ] **Implement API authentication**
  - Secure API key generation
  - Key rotation mechanism
  - JWT signing and validation
- [ ] **Implement authorization**
  - Role-based permissions
  - Resource-level access control
  - Admin action audit log
- [ ] **Implement TLS/HTTPS**
  - Certificate management
  - Force HTTPS in production
  - HSTS headers
- [ ] **Implement security headers**
  - CORS configuration
  - CSP headers
  - X-Frame-Options
- [ ] **Implement PII handling**
  - Define PII fields
  - Redaction in logs
  - Secure storage
  - Compliance with privacy laws (GDPR, etc.)

### 5.5 Deployment
- [ ] **Create Dockerfile**
  - Multi-stage build
  - Minimal production image
  - Non-root user
- [ ] **Create Docker Compose setup**
  - PostgreSQL with pgvector
  - SEL service
  - Optional: Redis, Prometheus, Grafana
- [ ] **Create Kubernetes manifests**
  - Deployment
  - Service
  - ConfigMap
  - Secrets
  - HPA (Horizontal Pod Autoscaler)
- [ ] **Implement configuration management**
  - Environment variables
  - Config file support
  - Secret management
  - Per-environment configs
- [ ] **Set up CI/CD**
  - Automated testing
  - Docker image build
  - Deploy to staging
  - Deploy to production
- [ ] **Implement database migrations**
  - Automated migration on deploy
  - Rollback capability
  - Migration testing

---

## Phase 6: Advanced Features (Post-MVP)

**Goal:** Add advanced capabilities for richer functionality.

### 6.1 Multilingual Support
- [ ] **Implement language tagging**
  - Store content with `@language` tags
  - Support multiple languages per event
- [ ] **Implement language filtering**
  - API query parameter: `lang=en,fr`
  - Content negotiation based on Accept-Language
- [ ] **Implement translation workflow**
  - Admin tools for managing translations
  - Integration with translation services (optional)

### 6.2 Accessibility Features
- [ ] **Add accessibility fields**
  - Schema.org accessibility properties
  - Structured accessibility info
  - Search/filter by accessibility
- [ ] **Implement accessibility validation**
  - Required fields for public venues
  - Data completeness metrics

### 6.3 Media and Assets
- [ ] **Implement image storage**
  - Local storage or S3-compatible
  - Image processing pipeline
  - Thumbnail generation
  - CDN integration
- [ ] **Implement image metadata**
  - Alt text
  - License information
  - Attribution
- [ ] **Implement media validation**
  - Allowed formats
  - Size limits
  - Virus scanning

### 6.4 Advanced Temporal Features
- [ ] **Implement RRULE support**
  - Parse RFC 5545 RRULE
  - Generate occurrences
  - Handle exceptions (EXDATE)
- [ ] **Implement timezone conversion API**
  - Display events in user's timezone
  - Handle DST transitions
  - Multi-timezone event displays
- [ ] **Implement calendar export**
  - iCal/ICS format
  - Google Calendar integration
  - Apple Calendar integration

### 6.5 ActivityPub Integration
- [ ] **Implement ActivityPub server**
  - Actor endpoint
  - Inbox/Outbox
  - Follow/Unfollow
- [ ] **Implement Activity types**
  - Create (event)
  - Update (event)
  - Delete (event)
  - Announce (event)
- [ ] **Implement federation protocol**
  - Deliver activities to followers
  - Receive activities from peers
  - Signature verification

---

## Testing Strategy

### Unit Tests
- [ ] **Test database queries (SQLc)**
  - Test all CRUD operations
  - Test complex queries
  - Test transactions
- [ ] **Test business logic**
  - Validation functions
  - Normalization functions
  - Deduplication logic
  - Merge logic
- [ ] **Test utilities**
  - URI generation
  - ULID generation
  - Identifier validation

### Integration Tests
- [ ] **Test API endpoints**
  - Happy path tests
  - Error handling
  - Authentication/authorization
  - Content negotiation
- [ ] **Test job processing**
  - Reconciliation jobs
  - Embedding jobs
  - Occurrence expansion jobs
- [ ] **Test Artsdata integration**
  - Reconciliation flow
  - Enrichment flow
  - Cache behavior
  - Error handling
  - SPARQL smoke tests (core vs all graphs)
  - Identifier format validation (Artsdata, Wikidata)

### Contract Tests
- [ ] **Test JSON-LD output**
  - Validate against frames
  - Validate against SHACL shapes
  - Test context resolution
- [ ] **Test URI patterns**
  - ULID format
  - Entity type segments
  - Node domain configuration
- [ ] **Test federation protocol**
  - Event sync
  - Tombstone handling
  - Conflict resolution

### Performance Tests
- [ ] **Load testing**
  - API endpoint throughput
  - Concurrent request handling
  - Database query performance
- [ ] **Scalability testing**
  - Large dataset queries
  - Vector search performance
  - Job queue throughput

---

## Documentation

### API Documentation
- [ ] **OpenAPI spec (auto-generated)**
  - Keep in sync with code
  - Examples for all endpoints
  - Error response schemas
- [ ] **Integration guide**
  - Authentication setup
  - Common workflows
  - Code examples (curl, Python, Go)
- [ ] **Agent integration guide**
  - MCP tool usage
  - LLM prompting patterns
  - Context document usage

### Data Model Documentation
- [ ] **Schema documentation**
  - ER diagrams
  - Table descriptions
  - Relationship mapping
- [ ] **JSON-LD examples**
  - Canonical output examples
  - Frame examples
  - Context explanation
- [ ] **Migration guide**
  - How to run migrations
  - Rollback procedures
  - Testing migrations

### Operator Documentation
- [ ] **Deployment guide**
  - Docker deployment
  - Kubernetes deployment
  - Configuration reference
- [ ] **Operations runbook**
  - Common issues and solutions
  - Monitoring and alerting
  - Backup and recovery
- [ ] **Troubleshooting guide**
  - Debug logging
  - Performance tuning
  - Error investigation

---

## Success Metrics

### Phase 0 (Foundational)
- All contracts documented
- All schemas defined
- All validation rules specified
- Team alignment achieved

### Phase 1 (Core)
- System ingests and stores events
- Basic API functional
- Job queue processing reliably
- Tests passing

### Phase 2 (Artsdata)
- Reconciliation working with >90% success rate
- Enrichment adding value to >80% of events
- SHACL validation passing

### Phase 3 (Federation)
- Successful sync with at least one peer node
- Export formats validated by external consumers
- Change feed consumed by webhook subscribers

### Phase 4 (Advanced)
- Semantic search returning relevant results
- MCP tools used by AI agents successfully
- Admin features enable efficient management

### Phase 5 (Production)
- System handles target load (define SLA)
- <1% error rate
- <500ms p95 latency for API calls
- Zero data loss

---

## Risk Mitigation

### Technical Risks
- **Risk:** ULID collision  
  **Mitigation:** Proper ULID library, entropy source, collision detection
  
- **Risk:** Artsdata API unavailability  
  **Mitigation:** Caching, circuit breakers, graceful degradation, queue retry
  
- **Risk:** Vector search performance at scale  
  **Mitigation:** Hybrid pgvector + USearch, query optimization, caching
  
- **Risk:** Database performance bottlenecks  
  **Mitigation:** Proper indexing, query optimization, connection pooling, read replicas

### Process Risks
- **Risk:** Scope creep  
  **Mitigation:** Strict phase discipline, MVP focus, defer non-critical features
  
- **Risk:** Integration complexity  
  **Mitigation:** Incremental integration, extensive testing, fallback modes
  
- **Risk:** Maintenance burden  
  **Mitigation:** Simple architecture, good documentation, automated testing
