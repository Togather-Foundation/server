# Architecture Guide

**Version:** 0.2  
**Date:** 2026-01-27  
**Status:** Living Document

> **Document Role:** This is the implementation guide for Togather contributors, covering code organization, data architecture, and development patterns. For the formal system design specification (intended for node implementers and architects), see [SEL Server Architecture Design](../togather_SEL_server_architecture_design_v1.md).

This document provides an architectural overview of the Togather Shared Events Library (SEL) backend. It focuses on system design, component architecture, technology choices, and key design patterns. For implementation details, see [DATABASE.md](DATABASE.md) and [TESTING.md](TESTING.md).

---

## Table of Contents

1. [Architecture Philosophy](#architecture-philosophy)
2. [System Overview](#system-overview)
3. [URI Scheme and Linked Data Foundation](#uri-scheme-and-linked-data-foundation)
4. [Core Components](#core-components)
5. [API Design](#api-design)
6. [Data Architecture](#data-architecture)
7. [Integration Strategy](#integration-strategy)
8. [Quality Assurance](#quality-assurance)
9. [Access Control and Authentication](#access-control-and-authentication)
10. [Deployment Architecture](#deployment-architecture)
11. [Technology Stack](#technology-stack)
12. [Design Patterns](#design-patterns)

---

## Architecture Philosophy

The Togather SEL backend follows **Specification Driven Development** with these core principles:

### Observability Over Opacity
Everything must be inspectable through CLI interfaces. The system provides bd (beads) for issue tracking, structured logging with zerolog, and explicit SQL queries via SQLc instead of opaque ORMs.

### Simplicity Over Cleverness
Start simple, add complexity only when proven necessary. The architecture uses a single Go service with modular packages rather than microservices. External dependencies are minimized (PostgreSQL + optional acceleration layers).

### Integration Over Isolation
Test with real dependencies in real environments. Integration tests use testcontainers with real PostgreSQL instances, real job queues, and real HTTP clients.

### Modularity Over Monoliths
Every feature is a reusable library with clear boundaries. Components are abstracted behind interfaces for swappability while maintaining a single deployment unit.

### Linked Open Data First
SEL is not "a database with JSON output" but **a node in the linked open data web**. Every entity has a stable URI, explicit provenance, and federation-ready contracts.

---

## System Overview

The SEL MVP backend is designed as a **single Go service** (with optional auxiliary components) to maximize simplicity and ease of deployment. The monolithic-but-modular approach means developers and AI agents can run the entire system easily and iterate on parts in isolation.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     External Clients                        │
│  (Scrapers, Mobile Apps, Web UIs, LLM Agents, Federation)   │
└────────────┬──────────────────────────────────┬─────────────┘
             │                                   │
    ┌────────▼────────┐                 ┌───────▼──────────┐
    │   HTTP API      │                 │  Agent Tooling   │
    │   (net/http)    │                 │  (CLI + Scripts) │
    │                 │                 │                  │
    │ - REST endpoints│                 │ - Tool interface │
    │ - Content neg.  │                 │ - Natural lang   │
    │ - OpenAPI Spec │                 │ - Context docs   │
    └────────┬────────┘                 └───────┬──────────┘
             │                                   │
             └───────────────┬───────────────────┘
                             │
                    ┌────────▼────────┐
                    │  Service Layer  │
                    │                 │
                    │ - Validation    │
                    │ - Normalization │
                    │ - Enrichment    │
                    │ - Deduplication │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
   ┌────▼──────┐       ┌─────▼───────┐     ┌──────▼──────┐
   │ Repository│       │ Background  │     │ Integration │
   │  Layer    │       │   Jobs      │     │   Modules   │
   │           │       │  (River)    │     │             │
   │ - Events  │       │             │     │ - Artsdata  │
   │ - Places  │       │ - Reconcile │     │ - Wikidata  │
   │ - Orgs    │       │ - Vectors   │     │ - KG APIs   │
   └────┬──────┘       │ - Sync      │     └─────────────┘
        │              └─────────────┘
        │
   ┌────▼─────────────────────────────────────────┐
   │         PostgreSQL 16+ Database              │
   │                                              │
   │ - Events & Occurrences (temporal split)      │
   │ - Places, Organizations, Persons             │
   │ - Provenance & Field Attribution             │
   │ - Federation Infrastructure                  │
   │ - pgvector (embeddings)                      │
   │ - PostGIS (geospatial)                       │
   │ - pg_trgm (fuzzy search)                     │
   └──────────────────────────────────────────────┘
```

### Key Characteristics

- **Single Binary Deployment**: Entire system runs as one Go process (or with optional workers)
- **Postgres-Centric**: Database handles search, vectors, jobs, geospatial - minimal external deps
- **Transactional Job Queue**: River ensures background work is never lost via ACID guarantees
- **Content Negotiation**: Public pages support HTML and JSON-LD; API defaults to JSON-LD
- **Federation-Ready**: Built-in change feeds, bulk export, and peer synchronization
- **Agent-Friendly**: CLI tooling and docs keep the system inspectable

---

## URI Scheme and Linked Data Foundation

Before describing components, we establish the **foundational linked data contracts** that define how SEL participates in the broader semantic web and federated event ecosystem.

### Canonical URI Pattern

SEL uses a **Federated Identity Model** where every entity belongs to an origin node but allows global linking.

```
https://{node-domain}/{entity-type}/{ulid}
```

**Components:**
- `node-domain`: Fully qualified domain name of authoritative SEL node (e.g., `toronto.togather.foundation`)
- `entity-type`: Plural lowercase entity type (`events`, `places`, `organizations`, `persons`)
- `ulid`: Universally Unique Lexicographically Sortable Identifier (26-character Base32)

**Examples:**
```
https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF
https://toronto.togather.foundation/places/01HYX4ABCD1234567890VENUE
https://toronto.togather.foundation/organizations/01HYX5EFGH0987654321ORGAN
```

### Why ULID?

- **Timestamp-prefixed**: First 10 chars encode creation time (enables efficient time-based queries)
- **Globally unique**: 128-bit cryptographically random suffix (no coordination needed between nodes)
- **URL-safe**: Base32 encoding works in URLs without escaping
- **Sortable**: Lexicographic sorting equals chronological sorting
- **True ULIDs**: Generated by application (not UUID re-encoding)

**Implementation**: Use `oklog/ulid/v2` package in Go

### Identifier Roles

SEL distinguishes three identifier types with different purposes:

| Field | Purpose | Controlled By | Example |
|-------|---------|---------------|---------|
| `@id` | Canonical URI for linked data | SEL node (this system) | `https://toronto.togather.foundation/events/01HYX...` |
| `url` | Public webpage for humans | External (venue/ticketing) | `https://masseyhall.com/events/jazz-night` |
| `sameAs` | Equivalent entities in external systems | External authorities | `http://kg.artsdata.ca/resource/K12-345` |

**Critical Rule**: `@id` ≠ `url`

The `@id` is YOUR stable semantic identifier. The `url` is where the event is promoted publicly. Never conflate these - they serve different purposes in the linked data web.

### Federation Rule (Preservation of Origin)

> **MUST**: A node MUST NOT generate an `@id` using another node's domain. When ingesting data from a peer node, the receiving node MUST preserve the original `@id`.

This ensures URI stability across the federation and prevents identifier collisions. Store `origin_node_id` with each event to track which node minted the URI.

### Content Negotiation

Dereferenceable URIs (e.g., `https://toronto.togather.foundation/events/{id}`) MUST support content negotiation:

| Accept Header | Response Format | Use Case |
|---------------|----------------|----------|
| `text/html` | Human-readable HTML page | Web browsers, human review |
| `application/ld+json` | Canonical JSON-LD document | Semantic web agents, harvesters |
| `application/json` | JSON-LD document (alias) | Generic API clients |
| `text/turtle` | RDF Turtle document | Semantic web tooling (planned) |

**MVP Requirement**: Provide **both HTML and JSON-LD** for dereferenceable URIs. HTML enables human review of data, JSON-LD enables machine consumption. Embed canonical JSON-LD in HTML pages using `<script type="application/ld+json">`.

**API Endpoints** (e.g., `/api/v1/events/{id}`) serve JSON-LD only. They do not support HTML or Turtle negotiation.

### Tombstones for Deleted Entities

Deleted entities MUST return **HTTP 410 Gone** with a minimal JSON-LD tombstone:

```json
{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://toronto.togather.foundation/events/01HYX3KQW7...",
  "eventStatus": "https://schema.org/EventCancelled",
  "sel:tombstone": true,
  "sel:deletedAt": "2025-01-20T15:00:00Z",
  "sel:deletionReason": "duplicate_merged",
  "sel:supersededBy": "https://toronto.togather.foundation/events/01HYX4MERGED..."
}
```

This preserves URI stability even after deletion. Consumers can discover merges via `supersededBy` links.

---

## Core Components

### Ingestion API Server

**Technology**: Standard library `net/http` with explicit handlers and middleware

An HTTP RESTful API that accepts incoming event submissions and serves event queries. This layer handles:

- **Request Validation**: Schema validation via go-playground/validator
- **Authentication**: API key and JWT verification
- **Content Negotiation**: `application/ld+json` for APIs, HTML on public pages
- **Routing**: Clean REST-style URLs with versioned endpoints
- **Error Handling**: RFC 7807 Problem Details for consistent error responses

**Design Decision**: Use explicit handlers and SQLc-backed queries for clarity and observability. API docs live in `specs/` and `docs/` and are kept in sync via CI.

### Core Processing Pipeline

Logic for **validation, normalization, enrichment, and deduplication** of incoming events. The pipeline:

1. **Validates** inputs at the boundary (schema conformance, required fields, type checking)
2. **Normalizes** data (timezone conversion, text cleaning, address standardization)
3. **Enriches** with external data (geocoding, knowledge graph reconciliation)
4. **Deduplicates** against existing events (layered strategy: exact hash, fuzzy, vectors)

**Key Insight**: Validation happens synchronously (fail fast), but enrichment and reconciliation happen asynchronously via job queue to keep API response times low.

### Relational Data Store (PostgreSQL)

PostgreSQL serves as the **source of truth** for all event data with these extensions:

- **pgvector**: Semantic search embeddings (384-dimensional vectors)
- **PostGIS**: Geospatial queries (bounding boxes, radius search, distance sorting)
- **pg_trgm**: Fuzzy text matching for deduplication
- **River**: Transactional job queue built on Postgres

**Split Model**: Events use a two-table pattern:
- `events` table: Canonical identity and metadata
- `event_occurrences` table: Temporal instances (handles reschedules, recurring events)

This separation cleanly handles:
- Postponements (update occurrence, preserve event identity)
- Recurring events (one event, many occurrences)
- Venue changes per occurrence (series moves between venues)

**Technology Choice**: Use **SQLc** to generate type-safe Go from explicit SQL queries rather than a heavy ORM. This keeps the SQL schema explicit, deterministic, and easy for agents and humans to inspect. See [Why SQLc](#why-sqlc-over-gorm) in Design Patterns.

### Vector Search

**MVP**: pgvector in Postgres as source of truth  
**Production**: Optional USearch in-memory acceleration cache

Store embeddings in Postgres using pgvector to avoid external services and ensure ACID semantics. For production performance, consider a hybrid:

1. **pgvector**: Canonical store (persistent, transactional)
2. **USearch**: In-memory acceleration (rebuilt from DB on startup, kept in sync via River jobs)

This hybrid provides correctness (pgvector) and speed (USearch) while keeping initial deployment simple.

**Embedding Model**: Use sentence transformers (e.g., all-MiniLM-L6-v2) for event text. Model runs on CPU without GPU dependency (aligns with self-contained deployment goal).

### Provenance Tracking System

A comprehensive system for **field-level provenance** and source trust. Every piece of data is attributed to a source with confidence scores.

**Components:**
- `sources` registry: Trust levels (1-10), license info, rate limits, contact data
- `field_provenance` table: Tracks which source provided each critical field value
- Conflict resolution: Priority based on `trust_level DESC, confidence DESC, observed_at DESC`
- `field_conflicts` view: Identifies fields with multiple values for admin review

**Critical for Federation**: When multiple nodes provide conflicting information about the same event, provenance enables explainable resolution and audit trails.

### Knowledge Graph Integration

Responsible for **entity reconciliation and enrichment** using external knowledge graphs.

**Supported Authorities:**
- **Artsdata**: Arts and culture events (primary for arts)
- **Wikidata**: Universal knowledge graph (fallback, non-arts events)
- **MusicBrainz**: Music artists and recordings
- **ISNI**: International Standard Name Identifier (creators)
- **OpenStreetMap**: Venues and places

**Domain-Based Routing:**
- Arts/Culture/Music: Artsdata → Wikidata (fallback)
- Sports/Community/Education: Wikidata (primary)
- Places: Multi-graph parallel reconciliation

**Confidence Thresholds:**
- ≥95%: Auto-accept and add to `entity_identifiers`
- 80-94%: Flag for manual review
- <80%: Reject, cache negative result

**Cache Strategy**: Results stored in `reconciliation_cache` with TTL (30 days positive, 7 days negative) to avoid repeated API calls.

### Authentication & RBAC Layer

Manages user accounts, API keys, and role-based access control.

**Roles:**
- **Public**: Read-only access to published events
- **Agent**: Write access for scrapers and integrations
- **Admin**: Full access including user management

**Authentication Methods:**
- **API Keys**: For agents and long-lived integrations (multiple keys per user, scoped permissions)
- **JWT**: For admin interactive sessions (tokens, configurable expiry)

**Rate Limiting**: Applied per-role to prevent abuse while allowing legitimate high-volume users. See [SECURITY.md](SECURITY.md) for details.

### Background Jobs & Utilities

Use **River** (transactional job queue) instead of best-effort in-process goroutines.

**Why Transactional Jobs?**
- Jobs queued atomically in the same DB transaction that persists the event
- Work is never silently lost on restarts (no invisible failure modes)
- Observable job status, retries, and audit trails
- Workers can run in same binary or separate processes

**Job Types:**
- Entity reconciliation with knowledge graphs
- Vector index updates (USearch sync)
- Bulk ingestion processing
- Periodic re-indexing
- Occurrence expansion for recurring events
- Cleanup and maintenance tasks

**Implementation**: River stores jobs in Postgres tables, so no external queue service needed. This aligns with the "Postgres-centric" architecture philosophy.

### Federation Layer

Components for multi-node federation enabling data sharing between SEL instances.

**Infrastructure:**
- **Node Registry** (`federation_nodes`): Tracks peer nodes with trust levels
- **Change Feed** (`event_changes`): Ordered stream of changes via cursor-based pagination
- **Bulk Export**: Dataset downloads in JSON-LD and N-Triples formats
- **Sync Protocol**: Accept/send events to/from peer nodes while preserving origin

**Change Feed**: Per-node monotonic sequence numbers for ordering. Cursor-based pagination prevents offset pagination pitfalls on large datasets.

**Sync Protocol (MVP):**
1. Discovery via `/.well-known/sel-profile`
2. Auth via per-peer API key
3. Pull changes from `GET /api/v1/feeds/changes?since={cursor}&limit={n}`
4. Apply in sequence order; cursor advances only after successful batch
5. Exponential backoff on 5xx/timeouts

### Agent Tooling

The system is designed to be inspectable via CLI interfaces and well-documented APIs. There is no separate MCP server in the current implementation; agent workflows interact through HTTP endpoints, CLI commands, and documented artifacts.

---

## API Design

The SEL exposes a **RESTful HTTP API** with clear, versioned endpoints for both data submission and retrieval.

### Core Principles

- **Resource-Oriented URLs**: `/api/v1/events/{id}`, not `/api/v1/getEvent`
- **Standard HTTP Verbs**: GET (retrieval), POST (creation), PUT (update), DELETE (removal)
- **Appropriate Status Codes**: 201 Created, 400 Bad Request, 401 Unauthorized, 410 Gone
- **Content Negotiation**: Support `application/ld+json` for APIs, HTML on public pages
- **Cursor-Based Pagination**: Avoid offset pagination pitfalls on large datasets

### Response Envelopes

**List Response:**
```json
{
  "items": [
    { "@id": "https://toronto.togather.foundation/events/01J...", "@type": "Event", "name": "Jazz Night" }
  ],
  "next_cursor": "seq_1048602"
}
```

**Change Feed Response:**
```json
{
  "cursor": "seq_1048576",
  "changes": [
    {
      "action": "update",
      "uri": "https://toronto.togather.foundation/events/01J...",
      "changed_at": "2025-07-10T12:05:00Z",
      "changed_fields": ["/name"],
      "snapshot": { "@id": "...", "@type": "Event" }
    }
  ],
  "next_cursor": "seq_1048602"
}
```

**Error Response (RFC 7807):**
```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Invalid request",
  "status": 400,
  "detail": "Missing required field: startDate",
  "instance": "/api/v1/events"
}
```

### Key Endpoints

#### Event Submission
**`POST /api/v1/events`**

Allows authenticated agents to submit new events. Payloads follow schema.org/Event structure.

**Headers:**
- `Content-Type: application/ld+json` or `application/json`
- `Idempotency-Key: {uuid}` (optional, for safe retries)
- `Authorization: Bearer {api_key}`

**Response:**
- `201 Created` with `Location` header pointing to new event
- `409 Conflict` if duplicate detected (returns existing event URI)

**Bulk Submission:**
**`POST /api/v1/events:batch`** (planned)

#### Event Query
**`GET /api/v1/events`**

Filtered queries over events. Publicly accessible.

**Query Parameters:**
- `start_after={ISO8601}`: Events starting after this time
- `start_before={ISO8601}`: Events starting before this time
- `location={city}` or `venue_id={id}`: Location filter
- `q={text}`: Full-text search
- `after={cursor}&limit=50`: Pagination (max limit: 200)

**Response**: List envelope with items and next_cursor

#### Event Detail
**`GET /api/v1/events/{id}`**

Returns full event details. Supports content negotiation.

**Accept Headers (API):**
- `application/ld+json`: Canonical JSON-LD
- `application/json`: JSON-LD (alias)

**Response**: Single event object with full schema.org fields and enrichment

#### Semantic Search
**`GET /api/v1/events/search`** (planned)

#### Change Feed
**`GET /api/v1/feeds/changes`**

Ordered change stream for federation and sync.

**Query Parameters:**
- `since={cursor}` or `after={cursor}`: Start from this sequence number
- `limit={n}`: Events per page (default 100, max 1000)

**Response**: Change feed envelope with cursor and changes array

#### Bulk Export

Planned endpoints (not implemented yet):
- **`GET /api/v1/exports/events.jsonld`**: Single JSON-LD graph
- **`GET /api/v1/exports/events.ndjson`**: Newline-delimited JSON-LD

Planned filters: `start_after` and `start_before`.

#### Admin Endpoints

Protected by admin role authentication (`/api/v1/admin/*`):

- **`GET /api/v1/admin/events/pending`**: List events awaiting approval
- **`PUT /api/v1/admin/events/{id}`**: Update or correct event data
- **`DELETE /api/v1/admin/events/{id}`**: Remove or cancel event

#### Health Checks
- **`GET /healthz`**: Liveness probe (always returns 200 if process alive)
- **`GET /readyz`**: Readiness probe (checks DB connection, job queue)

### OpenAPI Specification

Published at `GET /api/v1/openapi.json` for client generation and agent use. The spec is maintained in `specs/` and kept in sync with handlers.

---

## Data Architecture

The SEL database implements a **three-layer hybrid architecture** designed to balance semantic correctness, query performance, and federation requirements:

### Three-Layer Architecture

1. **Document Truth Layer (JSONB)**: Preserves original payloads with full provenance
2. **Relational Core Layer (Postgres)**: Enables fast queries, time-range filtering, geospatial operations
3. **Semantic Export Layer (JSON-LD)**: Generated on-demand, cached, serves federation and knowledge graphs

### Key Design Principles

- **Schema.org Aligned**: Explicit semantic mappings to schema.org vocabulary
- **Federated by Design**: Origin tracking and URI preservation built-in
- **Provenance as First-Class**: Field-level attribution and conflict resolution
- **Temporal Correctness**: Timezone handling, lifecycle states, bitemporal tracking
- **License Clarity**: Per-source validation (CC0 requirement for sharing)
- **SHACL Validation**: Against Artsdata shapes in CI/CD

### Core Entity Pattern: Events

**Identity vs. Temporal Instances**

Events use a two-table pattern to separate identity from temporal data:

**`events` Table** (Canonical Identity):
- Unique URI and ULID
- Lifecycle states: draft, published, postponed, rescheduled, cancelled, completed
- Federation metadata: origin_node_id, federation_uri
- Deduplication fingerprints
- Quality indicators and confidence scores

**`event_occurrences` Table** (Temporal Instances):
- Multiple occurrences per event (recurring events, rescheduled performances)
- Stores: UTC timestamp + IANA timezone + computed local time
- Occurrence-specific overrides (venue, status, tickets)
- Enables "events on Friday evening" queries without runtime timezone math

**Why Split?**
- Clean handling of postponements (update occurrence, preserve event identity)
- Natural modeling of recurring events (one event series, many instances)
- Per-occurrence venue changes (e.g., touring performances)
- URI stability when dates change (event @id stays constant)

**Serialization Rule:**
- `event_series` renders as `EventSeries` with `eventSchedule`
- `event_occurrences` render as `Event` with `superEvent` pointing to series
- Occurrence location overrides series location

### Related Entities

**Places** (`places`):
- schema.org/Place mapping
- Structured addresses (PostalAddress)
- Geocoding with PostGIS (GeoCoordinates)
- Support for both physical venues and VirtualLocation

**Organizations** (`organizations`):
- schema.org/Organization mapping
- Event organizers, promoters, venues as organizations
- Reconciled external IDs (sameAs to Artsdata, Wikidata, ISNI)
- alternateName support for known aliases

**Persons** (`persons`):
- schema.org/Person mapping
- Performers and artists
- Birth dates, biography, external IDs

**Event Performers** (`event_performers`):
- Many-to-many junction table
- Role support (e.g., "headliner", "opening act")
- Performance order

### Provenance and Trust

**Sources Registry** (`sources`):
- Trust levels (1-10 scale): 10 = official, 5 = community, 1 = unverified
- License information (CC0, CC-BY, proprietary)
- Rate limiting configuration
- Contact information for source maintainers

**Field-Level Provenance** (`field_provenance`):
- Tracks which source provided each critical field value
- Confidence scores per field (0.0 - 1.0)
- Temporal tracking with supersession (new values don't delete old provenance)
- Enables explainable conflict resolution

**Conflict Resolution Strategy:**
```sql
ORDER BY trust_level DESC, confidence DESC, observed_at DESC
```

View `field_conflicts` identifies fields with multiple conflicting values for admin review.

### Federation Infrastructure

**Federation Nodes Registry** (`federation_nodes`):
- Tracks peer SEL nodes (node_domain, display_name, trust_level)
- Sync configuration (sync_enabled, last_sync_cursor)
- Geographic scope definitions
- API endpoint and authentication

**Event Changes Outbox** (`event_changes`):
- Captures all create/update/delete actions
- Per-node monotonic sequence numbers for ordering
- Change-feed cursor for pagination
- Enables change feed API and webhooks

**Sync State Tracking** (`federation_sync_state`):
- Last successful sync timestamp and cursor
- Error tracking and retry state
- Per-peer sync configuration

### Reconciliation and External Identifiers

**Knowledge Graph Authorities Registry** (`knowledge_graph_authorities`):
- Configures supported knowledge graphs (Artsdata, Wikidata, MusicBrainz, etc.)
- Domain applicability (arts vs. sports vs. universal)
- Priority ordering for reconciliation routing
- Trust levels (1-10) for conflict resolution
- API endpoints (reconciliation, SPARQL, dereferencing)
- Rate limiting configuration

**Entity Identifiers** (`entity_identifiers`):
- Normalized external IDs from registered authorities
- Maps to `sameAs` semantics in JSON-LD output
- Supports **multiple identifiers per entity** (e.g., both Artsdata AND Wikidata)
- Confidence scores and reconciliation method tracking
- Canonical flag for primary identifier selection

**Reconciliation Cache** (`reconciliation_cache`):
- Per-authority caching to avoid repeated API calls
- Keyed by (entity_type, authority_code, lookup_key)
- TTL: 30 days (positive matches), 7 days (negative results)
- Stores candidates and confidence scores
- Hit count tracking for cache effectiveness

### Temporal and Lifecycle Management

**Event History** (`event_history`):
- Bitemporal tracking for audit and rollback
- Field-level change tracking (JSON patches)
- Approval workflow support
- Triggered automatically on updates

**Event Aliases** (`event_aliases`):
- Preserves URI stability when events merge
- HTTP 301/410 redirects for deleted/merged events
- Tombstone support with `supersededBy` links

**Timezone Handling:**
- Stores UTC + IANA timezone identifier
- Computed local date/time for calendar queries
- Handles DST transitions correctly
- Example: "Every Friday at 7 PM Eastern" expands correctly across DST boundaries

### Search and Discovery

**Vector Embeddings** (`event_embeddings`):
- pgvector storage (384-dimensional)
- Model version tracking (for reindexing when models upgrade)
- Source text preservation (for reindexing)
- Cosine similarity search

**Full-Text Search:**
- GIN indexes on name + description (Postgres built-in)
- pg_trgm for fuzzy matching ("Jazz Fest" matches "Jazz Festival")
- Ranked results using `ts_rank`

**Geospatial Queries:**
- PostGIS support for spatial operations
- Bounding box queries (events in this map viewport)
- Radius queries (events within 5km of this point)
- Distance-based sorting

**Duplicate Detection** (`duplicate_candidates`):
- Layered strategy: exact hash, fuzzy matching, vector similarity
- Match features with similarity scores
- Review workflow for ambiguous cases
- Maintains `event_aliases` for redirects after merges

### Access Control

**Users Table** (`users`):
- Role-based access: admin, agent
- Multiple API keys per user
- Rate limiting tiers

**API Keys** (`api_keys`):
- Scoped permissions (read-only, write events, admin)
- Usage tracking (request count, last used timestamp)
- Expiration support
- Key rotation without user downtime

### Operational Tables

**Idempotency Keys** (`idempotency_keys`):
- Prevents duplicate operations from retries
- 24-hour TTL (auto-cleanup)
- Request/response caching

**Webhook System**:
- `webhook_subscriptions`: Event type filters, target URLs, auth config
- `webhook_deliveries`: Delivery log with status and retry tracking
- Retry with exponential backoff
- Health tracking and automatic subscription suspension on repeated failures

**Job Queue** (River tables):
- `river_job`: Job queue with status, attempts, errors
- `river_leader`: Distributed leadership election for workers
- `river_migration`: Schema versioning for River itself

---

## Integration Strategy

### Knowledge Graph Reconciliation

SEL integrates with multiple knowledge graphs to enrich event data with authoritative identifiers.

#### Domain-Based Routing

Different event types use different primary knowledge graphs:

**Arts/Culture/Music Events:**
1. Artsdata (primary for Canadian arts)
2. MusicBrainz (for music-specific entities)
3. Wikidata (fallback for general entities)

**Sports/Community/Education Events:**
1. Wikidata (universal knowledge graph)
2. Domain-specific graphs as appropriate

**Places (All Event Types):**
1. Artsdata (for arts venues)
2. OpenStreetMap (for general places)
3. Wikidata (for notable locations)
4. **Strategy**: Parallel reconciliation with multiple graphs

#### Reconciliation Pipeline

1. **Extract Event Domain**: Determine event type from schema.org eventType or additionalType
2. **Query Authorities Registry**: Get applicable graphs ordered by priority
3. **Check Cache**: Per-authority cache lookup to avoid API calls
4. **API Reconciliation**: Call reconciliation APIs in sequence until high-confidence match found
5. **Fallback Strategy**: Use Wikidata if no domain-specific match
6. **Store Identifiers**: Add to `entity_identifiers` (supports multiple graphs per entity)

**Example: Music Event Reconciliation**

```
Event domain: "music"
→ Query authorities: [MusicBrainz (priority 15), Artsdata (20), Wikidata (30)]
→ MusicBrainz: confidence 0.97 → Accept, add to entity_identifiers
→ Also reconcile with Artsdata: confidence 0.92 → Accept, add as secondary
→ Result: Event has sameAs = [musicbrainz_uri, artsdata_uri]
```

#### Implementation Details

**HTTP Client**: Use `go-retryablehttp` or `resty` with:
- Exponential backoff for 5xx errors and timeouts
- Circuit breaker pattern to avoid hammering failing services
- Request timeouts (5s reconciliation, 10s enrichment)

**Async Processing**: Reconciliation jobs enqueued via River in same transaction as event creation. Guarantees work is never lost.

**Cache Strategy**:
- Normalize keys before lookup (lowercase, trim, collapse whitespace)
- TTL: 30 days (positive), 7 days (negative)
- Invalidate on entity update or authority configuration change

**Confidence Handling**:
- ≥95%: Auto-accept, add to `entity_identifiers`
- 80-94%: Store candidates, flag for manual review in admin UI
- <80%: Cache negative result, don't create identifier

### Artsdata Integration

Artsdata.ca is a pan-Canadian knowledge graph for the arts. Integration provides:

**Entity Reconciliation**: Link events to Artsdata's persistent IDs (K-numbers like `K11-211`)

**Enrichment**: Fetch authoritative data from Artsdata:
- Canonical venue names and addresses
- Geocoding when missing
- Links to Wikidata, ISNI, MusicBrainz

**Contribution**: SEL can contribute back to Artsdata via their Databus API (future feature)

**Validation**: Use Artsdata SHACL shapes in CI/CD to ensure output compatibility

**License Compliance**: Artsdata requires CC0 for contributed data. SEL enforces license validation per source.

### External Authority Validation

All external identifiers MUST be validated using regex patterns during ingestion:

| Authority | URI Pattern | Example |
|-----------|-------------|---------|
| Artsdata | `^http://kg\.artsdata\.ca/resource/K\d+-\d+$` | `http://kg.artsdata.ca/resource/K11-211` |
| Wikidata | `^http://www\.wikidata\.org/entity/Q\d+$` | `http://www.wikidata.org/entity/Q636342` |
| ISNI | `^https://isni\.org/isni/\d{16}$` | `https://isni.org/isni/0000000121032683` |
| MusicBrainz | `^https://musicbrainz\.org/\w+/[0-9a-f-]{36}$` | `https://musicbrainz.org/artist/...` |
| OpenStreetMap | `^https://www\.openstreetmap\.org/(node\|way\|relation)/\d+$` | `https://www.openstreetmap.org/node/123456` |

Invalid URIs are rejected at ingestion time with clear error messages.

---

## Quality Assurance

### Validation and Normalization

All incoming events undergo multi-stage validation:

#### Stage 1: Schema Validation

**Required Fields:**
- `name`: Event title (string, 1-500 chars)
- `startDate`: Start date/time (ISO8601 with timezone)
- `location`: Place or VirtualLocation

**Optional But Encouraged:**
- `description`: Event description (promotes discoverability)
- `organizer`: Organization or Person
- `url`: Public event page
- `offers`: Ticketing information

**Technology**: `go-playground/validator` for struct tags, custom validators for complex rules

#### Stage 2: Semantic Validation

- **Temporal Logic**: endDate ≥ startDate, doorTime < startDate
- **Timezone Consistency**: All times include IANA timezone or UTC offset
- **URL Validation**: All URLs are valid, reachable (optional check), use HTTPS where possible
- **License Validation**: Source license is declared and valid (CC0, CC-BY, etc.)

#### Stage 3: Normalization

**Date/Time:**
- Convert to UTC for storage
- Preserve original timezone in `timezone` field
- Compute local date/time in `local_start_date` for calendar queries

**Text:**
- Trim whitespace
- Strip HTML tags (events should be plain text or markdown)
- Decode HTML entities
- Normalize unicode (NFC normalization)

**Location:**
- Standardize addresses (title case, consistent formatting)
- Geocode if coordinates missing (via OpenStreetMap Nominatim or similar)
- Normalize country codes to ISO 3166-1 alpha-2

**URIs:**
- Validate format
- Normalize (lowercase scheme/host, remove default ports, sort query params)

#### Stage 4: Enrichment

**Place Enrichment:**
- Look up venue in local database (reuse existing place entities)
- Geocode addresses to coordinates
- Reconcile with Artsdata/OpenStreetMap for place IDs

**Organization Enrichment:**
- Match organizers to known organizations
- Reconcile with Artsdata/Wikidata for persistent IDs

**Categorization:**
- Infer event type from description/title (ML-based classification, future feature)
- Map source categories to controlled vocabulary

### Deduplication Strategy

Duplicate events can occur when multiple sources submit the same event. SEL implements a **layered deduplication strategy**:

#### Layer 1: Exact Matching

**Dedup Hash**: Deterministic fingerprint from:
- Normalized name (lowercase, trim, collapse whitespace)
- Normalized start time (rounded to nearest 5 minutes)
- Canonical place @id (or virtual_url for online events)
- Occurrence index (for recurring events)

**Implementation**: Compute SHA-256 hash of concatenated values. Store in `dedup_hash` column with btree index.

**Conflict Resolution**: When exact match found:
1. Preserve existing event @id (URI stability)
2. Merge provenance (track all sources)
3. Update fields only if new data has higher confidence
4. Log merge action in `event_history`

#### Layer 2: Strong Fuzzy Matching

**Criteria**:
- Same resolved `venue_id` (or close geocoded location within 500m)
- Start time within ±15 minutes
- Name similarity > 0.8 using pg_trgm

**Implementation**:
```sql
SELECT id FROM events
WHERE venue_id = $1
  AND start_time BETWEEN $2 - INTERVAL '15 minutes' AND $2 + INTERVAL '15 minutes'
  AND similarity(name, $3) > 0.8
LIMIT 1
```

**Handling**: Flag as `duplicate_candidate` for review if confidence not 100%

#### Layer 3: Weak Semantic Matching

**Criteria**:
- Vector similarity > 0.85 (cosine distance on embeddings)
- Geospatial proximity (within 5km)
- Wider time window (same day)

**Purpose**: Catch duplicates with significant description differences but same underlying event

**Handling**: Always flag for manual review (high false positive rate)

#### Duplicate Candidates Table

Store suspected duplicates in `duplicate_candidates`:
- `event_id_1`, `event_id_2`: Candidate pair
- `similarity_score`: Confidence (0.0 - 1.0)
- `match_features`: JSON with match reasons (time_diff, name_similarity, geo_distance)
- `review_status`: pending, confirmed_duplicate, false_positive

**Admin UI**: Provide interface to review candidates, merge events, or mark false positives

#### Event Aliases

When events are merged, create alias entries:
- `event_id`: Canonical (surviving) event
- `alias_uri`: Old (merged) event URI
- `redirect_type`: 301 (merged) or 410 (deleted)

**URI Stability**: Requests to old URI return 301 redirect to canonical URI. Preserves links from external systems.

### SHACL Validation

**CI/CD Integration**: Run SHACL validation in CI pipeline to ensure JSON-LD output conforms to Artsdata shapes.

**Technology**: Use Artsdata's published shapes from https://github.com/culturecreates/artsdata-data-model

**Validation Points**:
- After JSON-LD serialization in tests
- Before bulk export
- In admin UI when reviewing events

**Failure Handling**: Validation failures don't block ingestion but do:
1. Log warning with detailed shape violation report
2. Flag event for manual review
3. Report in admin dashboard

---

## Access Control and Authentication

### Authentication Methods

#### API Keys (For Agents and Integrations)

**Storage**: Hashed with bcrypt (cost 10) in `api_keys` table  
**Transmission**: `Authorization: Bearer <key>` header

**Key Features:**
- Multiple keys per user (for key rotation without downtime)
- Scoped permissions (read-only, write events, admin actions)
- Optional expiration dates
- Usage tracking (request count, last used timestamp)
- Rate limiting tied to key tier

**Generation**: 32-byte random value, base64-encoded (e.g., `sel_live_abcdef123456...`)

**Validation Flow**:
1. Extract key from Authorization header
2. Hash with bcrypt
3. Lookup in `api_keys` table
4. Check expiration, revocation status
5. Load associated user and permissions
6. Apply rate limit for key's tier

#### JWT (For Admin Sessions)

**Issuer**: SEL backend  
**Signing**: HMAC-SHA256 or RS256 (configurable)  
**Lifetime**: 24 hours (configurable via `JWT_EXPIRY_HOURS`)

**Claims**:
```json
{
  "sub": "admin",           // Username
  "email": "info@togather.foundation",
  "role": "admin",
  "iat": 1640000000,
  "exp": 1640003600,
  "iss": "https://toronto.togather.foundation"
}
```

**Validation Flow**:
1. Extract JWT from Authorization header or Cookie
2. Verify signature
3. Check expiration
4. Load user from `sub` claim
5. Apply role-based access control

### Role-Based Access Control (RBAC)

| Role | Permissions | Use Case |
|------|-------------|----------|
| **Public** | Read published events | Anonymous web users |
| **Agent** | Read + Write events | Scrapers, integrations |
| **Admin** | Agent + User management + Config | System administration |

**Implementation**: Middleware checks role before handler execution (JWT admin role required).

**Endpoint Protection Examples**:
- `GET /api/v1/events`: Public
- `POST /api/v1/events`: Agent
- `PUT /api/v1/admin/events/{id}`: Admin

### Rate Limiting

**Strategy**: Token bucket per API key or JWT subject

**Tiers**:
| Tier | Requests/Minute | Burst | Use Case |
|------|----------------|-------|----------|
| **Public** | 60 | 10 | Anonymous reads |
| **Agent** | 300 | 50 | Community scrapers |
| **Admin** | Unlimited | N/A | Internal tools |

**Implementation**: Redis or Postgres-based token bucket. Middleware rejects requests exceeding limit with:
- **429 Too Many Requests** status
- `Retry-After` header with seconds until reset
- Error response with rate limit details

**Headers** (included in all responses):
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 847
X-RateLimit-Reset: 1640003600
```

### Security Best Practices

**Secrets Management:**
- Environment variables for config (never commit secrets)
- Support for external secret managers (future: AWS Secrets Manager, Vault)

**HTTPS Only:**
- Enforce HTTPS in production (HSTS headers)
- Redirect HTTP to HTTPS

**CORS Configuration:**
- Configurable allowed origins
- Credentials support for cookie-based auth

**Input Sanitization:**
- All inputs validated via go-playground/validator
- SQL injection prevention via parameterized queries (SQLc)
- XSS prevention via HTML escaping in templates

For comprehensive security details, see [SECURITY.md](SECURITY.md).

---

## Deployment Architecture

### Container Strategy

**Docker**: Primary deployment method for consistency across environments

**Dockerfile Best Practices:**
- Multi-stage build (build stage + runtime stage)
- Minimal runtime image (distroless or alpine)
- Non-root user
- Health check support

**Compose Configuration**:
```yaml
services:
  server:
    image: togather-sel:latest
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      - DATABASE_URL=postgres://...
      - LOG_LEVEL=info
      - LOG_FORMAT=json
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
      interval: 10s
      timeout: 3s
      retries: 3

  postgres:
    image: postgres:16-alpine
    environment:
      - POSTGRES_DB=togather_sel
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
```

### Configuration Management

**Environment Variables**: Primary configuration method (12-factor app principles)

**Required Config:**
```bash
DATABASE_URL=postgres://user:pass@localhost/dbname
LOG_LEVEL=info
LOG_FORMAT=json
HTTP_PORT=8080
```

**Optional Config:**
```bash
# Authentication
JWT_SECRET=...
API_KEY_SALT=...

# External Services
ARTSDATA_API_KEY=...
OPENSTREETMAP_NOMINATIM_URL=https://nominatim.openstreetmap.org

# Feature Flags
ENABLE_FEDERATION=true
ENABLE_VECTOR_SEARCH=true

# Performance
RIVER_WORKER_COUNT=10
USEARCH_ENABLED=false
```

**Config Validation**: Fail fast on startup if required config missing

### Health Checks

**Liveness Probe** (`GET /healthz`):
- Always returns 200 OK if process is alive
- No external dependencies checked
- Fast (<10ms)

**Readiness Probe** (`GET /readyz`):
- Checks database connectivity
- Checks job queue connectivity
- Returns 503 Service Unavailable if any check fails
- Used by load balancers to route traffic

### Database Migrations

**Tool**: golang-migrate or goose

**Migration Files**: Versioned SQL files in `migrations/` directory

**Execution**:
- Automatically on startup in dev/staging
- Manually in production (for control and rollback capability)

**Rollback Support**: Every migration has up and down scripts

**CI/CD Integration**: Migrations tested in integration tests with testcontainers

### Observability

**Structured Logging**:
- zerolog for structured JSON logs
- Request correlation IDs (X-Request-ID header)
- Standard field names (see [DEVELOPMENT.md](DEVELOPMENT.md))

**Metrics** (Future):
- Prometheus /metrics endpoint
- Key metrics: request rate, latency, error rate, job queue depth

**Tracing** (Future):
- OpenTelemetry support
- Distributed tracing across federation

**Error Tracking** (Future):
- Sentry or similar for error aggregation and alerting

---

## Technology Stack

### Core Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| **Language** | Go 1.22+ | Type safety, performance, excellent stdlib, easy deployment |
| **HTTP Framework** | net/http | Explicit handlers and middleware |
| **Database** | PostgreSQL 16+ | Robust, ACID, excellent JSON/GIS/vector support |
| **SQL Generation** | SQLc | Type-safe SQL without ORM complexity |
| **Job Queue** | River | Transactional queue built on Postgres, no external dependency |
| **Logging** | zerolog | Structured logging, fast, zero-allocation |
| **Validation** | go-playground/validator | Struct tag validation, extensive rules |
| **UUID/ULID** | oklog/ulid/v2 | Sortable UUIDs with timestamp prefix |
| **JSON-LD** | piprate/json-gold | JSON-LD processing, framing, compaction |
| **JWT** | golang-jwt/jwt/v5 | Standard JWT implementation (HS256) |

### Database Extensions

| Extension | Purpose | Use Cases |
|-----------|---------|-----------|
| **pgvector** | Vector embeddings | Semantic search, similarity queries |
| **PostGIS** | Geospatial | Location queries, distance calculations |
| **pg_trgm** | Trigram matching | Fuzzy text search, deduplication |
| **uuid-ossp** | UUID generation | Fallback UUID generation |
| **pgcrypto** | Cryptographic functions | Password hashing, token generation |

### Optional Acceleration

| Component | Technology | When to Use |
|-----------|-----------|-------------|
| **Vector Search** | USearch | Production with high semantic search load |
| **Embedding Model** | Ollama sidecar | Self-hosted embedding generation |
| **Rate Limiting** | Redis | High-volume environments (MVP uses Postgres) |
| **Cache** | Redis | High-traffic environments (MVP uses Postgres) |

---

## Design Patterns

### Why SQLc Over GORM

SEL uses **SQLc** instead of GORM or other ORMs:

**Advantages**:
1. **Explicit Schema**: SQL is first-class, easy for agents and humans to inspect
2. **Type Safety**: Generated Go code is fully typed, compile-time SQL validation
3. **Performance**: No query builder overhead, just prepared statements
4. **Predictable**: No magic, no N+1 queries, no hidden lazy loading
5. **Agent-Friendly**: LLM agents can read SQL and generated code easily

**Disadvantages**:
- More verbose (write SQL by hand)
- No automatic migrations (use golang-migrate or goose separately)

**When to Consider GORM**: Never for this project. The explicit SQL approach aligns with the "Simplicity Over Cleverness" principle.

### Repository Pattern

**Structure**: One repository per domain entity (EventRepository, PlaceRepository, etc.)

**Interface**:
```go
type EventRepository interface {
    Create(ctx context.Context, event *Event) error
    GetByID(ctx context.Context, id string) (*Event, error)
    List(ctx context.Context, filters EventFilters) ([]*Event, error)
    Update(ctx context.Context, event *Event) error
    Delete(ctx context.Context, id string) error
}
```

**Implementation**: Concrete struct with SQLc queries injected

**Benefits**:
- Testable (mock repository in tests)
- Swappable (could replace Postgres with another store)
- Clean separation of data access from business logic

### Service Layer Pattern

**Structure**: Services orchestrate business logic, call repositories and external integrations

**Example: Event Service**
```go
type EventService struct {
    repo          EventRepository
    placeRepo     PlaceRepository
    reconciler    Reconciler
    jobQueue      JobQueue
}

func (s *EventService) CreateEvent(ctx context.Context, input CreateEventInput) (*Event, error) {
    // 1. Validate input
    // 2. Normalize data
    // 3. Check for duplicates
    // 4. Create event (transaction)
    // 5. Enqueue reconciliation job (same transaction)
    // 6. Return event
}
```

**Benefits**:
- Business logic isolated from HTTP handlers
- Reusable across API handlers and CLI tooling
- Testable without HTTP layer

### Transactional Outbox Pattern

SEL uses **River** to implement the transactional outbox pattern for background jobs.

**Problem**: If you create an event and then enqueue a job in separate transactions, the job can be lost if the second transaction fails.

**Solution**: Enqueue jobs in the same transaction as the entity creation.

**Implementation**:
```go
tx, err := db.Begin()
defer tx.Rollback()

// Create event
eventID, err := queries.CreateEvent(ctx, tx, event)

// Enqueue reconciliation job (same transaction)
_, err = riverClient.InsertTx(ctx, tx, ReconcileEventArgs{EventID: eventID}, nil)

tx.Commit()
```

**Benefits**:
- Jobs are never lost (ACID guarantees)
- Observability (job status visible in database)
- Retry logic built-in (River handles retries)

### Content Negotiation Pattern

**Implementation**: Middleware inspects `Accept` header and sets content type

**Supported Formats**:
- `application/ld+json`: Canonical JSON-LD
- `application/json`: JSON-LD (alias)
- `text/html`: Human-readable HTML (public pages)
- `text/turtle`: RDF Turtle (public pages)

**Example**:
```go
func HandleGetEvent(w http.ResponseWriter, r *http.Request) {
    event := fetchEvent(...)
    
    switch contentType := r.Header.Get("Accept") {
    case "text/html":
        renderHTML(w, event)
    case "application/ld+json", "application/json":
        renderJSONLD(w, event)
    default:
        renderJSONLD(w, event) // Default to JSON-LD
    }
}
```

### Provenance Tracking Pattern

**Design**: Every data modification records source, confidence, and timestamp

**Implementation**:
```go
func (s *EventService) UpdateField(ctx context.Context, eventID, field, value string, sourceID int, confidence float64) {
    tx, _ := db.Begin()
    defer tx.Rollback()
    
    // Update event field
    queries.UpdateEventField(ctx, tx, eventID, field, value)
    
    // Record provenance
    queries.CreateFieldProvenance(ctx, tx, FieldProvenance{
        EventID:    eventID,
        FieldName:  field,
        Value:      value,
        SourceID:   sourceID,
        Confidence: confidence,
        ObservedAt: time.Now(),
    })
    
    tx.Commit()
}
```

**Conflict Resolution**: Query `field_provenance` ordered by `trust_level DESC, confidence DESC, observed_at DESC` to get "best" value.

### Cursor-Based Pagination Pattern

**Problem**: Offset pagination (`OFFSET 1000 LIMIT 50`) is slow on large datasets

**Solution**: Use cursor-based pagination with monotonic sequence numbers

**Implementation**:
```sql
SELECT id, name, created_at, sequence_number
FROM events
WHERE sequence_number > $1  -- cursor
ORDER BY sequence_number ASC
LIMIT $2;
```

**Response**:
```json
{
  "items": [...],
  "next_cursor": "seq_1048602"
}
```

**Benefits**:
- Constant time complexity (uses index)
- Safe for real-time data (no skipped/duplicate items during pagination)
- Standard for change feeds and federation

---

## Next Steps

For implementation details and workflows:
- **Database Schema**: See [DATABASE.md](DATABASE.md) for complete schema, migrations, and query patterns
- **Testing Strategy**: See [TESTING.md](TESTING.md) for TDD workflow, test types, and coverage requirements
- **API Integration**: See [../integration/API_GUIDE.md](../integration/API_GUIDE.md) for endpoint usage and examples
- **Security**: See [SECURITY.md](SECURITY.md) for authentication, authorization, and security best practices

---

**Document Version**: 0.2  
**Last Updated**: 2026-01-27  
**Source**: Extracted from togather_SEL_server_architecture_design_v1.md (128KB)  
**Maintenance**: Update when architectural decisions change, not for minor implementation details
