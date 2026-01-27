# SEL API Contract v1.0-DRAFT

**Status:** Proposed for Community Review  
**Version:** 1.0-DRAFT  
**Last Updated:** 2026-01-27  
**Authors:** SEL Architecture Working Group (Ryan Kelln, Gemini 3 Pro, Claude Opus 4.5, OpenAI ChatGPT 5.2)

**Note:** Split from togather_SEL_Interoperability_Profile_v0.1.md for clarity

---

## Executive Summary

This document defines the **HTTP API contract** for Shared Events Library (SEL) nodes. It specifies:
- Public read API endpoints and response formats
- Export formats and bulk data access
- Change feed semantics for synchronization
- Reconciliation API contracts for knowledge graph integration

For core data models, URI schemes, and provenance rules, see [CORE_PROFILE_v0.1.md](./CORE_PROFILE_v0.1.md).

For federation sync protocols, see [FEDERATION_v1.md](./FEDERATION_v1.md).

---

## Table of Contents

1. [Public Read API](#1-public-read-api)
2. [OpenAPI Specification](#2-openapi-specification)
3. [Export Formats](#3-export-formats)
4. [Bulk Dataset Export](#4-bulk-dataset-export)
5. [Change Feed](#5-change-feed)
6. [Reconciliation Contracts](#6-reconciliation-contracts)

---

## 1. Public Read API

SEL nodes SHOULD expose a simple, public read API for open-data access:

- `GET /api/v1/events` (filtered list)
- `GET /api/v1/events/{id}` (single event)
- `GET /api/v1/events/search` (optional semantic search)

Responses MUST support content negotiation for JSON-LD (`Accept: application/ld+json`).
Write/ingestion endpoints are **implementation-specific** and out of scope for the interoperability profile.

### 1.1 Response Envelope (List)

```json
{
  "items": [
    { "@id": "https://toronto.togather.foundation/events/01J...", "@type": "Event", "name": "Jazz Night" }
  ],
  "next_cursor": "seq_1048602"
}
```

**Pagination Defaults:**
- Default `limit`: 50
- Max `limit`: 200

### 1.2 Error Envelope (RFC 7807)

```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Invalid request",
  "status": 400,
  "detail": "Missing required field: startDate",
  "instance": "/api/v1/events"
}
```

### 1.3 Query Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `limit` | Integer | Max items per page (default 50, max 200) | `?limit=100` |
| `cursor` | String | Opaque pagination cursor | `?cursor=seq_1048602` |
| `start_from` | Date | Filter events starting from date | `?start_from=2025-07-01` |
| `start_to` | Date | Filter events starting before date | `?start_to=2025-07-31` |
| `location` | String | Filter by place name or region | `?location=Toronto` |
| `organizer` | URI | Filter by organization URI | `?organizer=https://...` |

---

## 2. OpenAPI Specification

Implementations SHOULD publish an OpenAPI 3.1 document for these endpoints at:

**Endpoint:** `GET /api/v1/openapi.json`

**Minimum Requirements:**
- Document all public read endpoints
- Include request/response schemas using JSON Schema
- Provide example requests and responses
- Document error responses with RFC 7807 format

**Example Structure:**
```yaml
openapi: 3.1.0
info:
  title: Toronto SEL Node API
  version: 1.0.0
  description: Public API for Shared Events Library - Toronto Node

servers:
  - url: https://toronto.togather.foundation/api/v1

paths:
  /events:
    get:
      summary: List events
      parameters:
        - name: limit
          in: query
          schema:
            type: integer
            default: 50
            maximum: 200
      responses:
        '200':
          description: Successful response
          content:
            application/ld+json:
              schema:
                $ref: '#/components/schemas/EventList'
```

---

## 3. Export Formats

SEL nodes MUST support the following content types via `Accept` header negotiation:

| Format | Content-Type | Use Case |
|--------|--------------|----------|
| JSON-LD | `application/ld+json` | Semantic web consumption |
| JSON | `application/json` | Non-LD convenience view |
| Turtle | `text/turtle` | RDF tooling |
| N-Triples | `application/n-triples` | RDF dumps |
| NDJSON | `application/x-ndjson` | Bulk streaming |

### 3.1 Content Negotiation

**Example Request:**
```bash
curl -H "Accept: application/ld+json" \
  https://toronto.togather.foundation/api/v1/events/01HYX3...
```

**Response:**
```json
{
  "@context": [
    "https://schema.org",
    "https://togather.foundation/contexts/sel/v0.1.jsonld"
  ],
  "@id": "https://toronto.togather.foundation/events/01HYX3...",
  "@type": "Event",
  "name": "Jazz in the Park"
}
```

### 3.2 Format-Specific Rules

**JSON-LD:**
- MUST include `@context` with schema.org and SEL context
- MUST include all required fields per CORE_PROFILE

**Turtle/N-Triples:**
- MUST use valid RDF serialization
- MUST preserve all triples from JSON-LD representation

**NDJSON:**
- One JSON-LD object per line
- No commas between lines
- Use for bulk streaming (see § 4)

---

## 4. Bulk Dataset Export

**Endpoints:**
- `GET /api/v1/exports/events.jsonld` (single JSON-LD graph)
- `GET /api/v1/exports/events.ndjson` (newline-delimited JSON-LD)
- `GET /datasets/events.jsonld.gz` (compressed nightly dump)

### 4.1 Query Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `changed_since` | RFC3339 | Only entities changed after timestamp | `?changed_since=2025-07-01T00:00:00Z` |
| `start_from` | Date | Event start date range (from) | `?start_from=2025-07-01` |
| `start_to` | Date | Event start date range (to) | `?start_to=2025-07-31` |
| `include_deleted` | Boolean | Include tombstones (default false) | `?include_deleted=true` |

### 4.2 NDJSON Structure

Each line is a complete JSON-LD document:

```
{"@context":"...","@id":"https://toronto.togather.foundation/events/01J1...","@type":"Event","name":"Jazz Night",...}
{"@context":"...","@id":"https://toronto.togather.foundation/events/01J2...","@type":"Event","name":"Blues Evening",...}
{"@context":"...","@id":"https://toronto.togather.foundation/events/01J3...","@type":"Event","name":"Rock Concert",...}
```

**Processing Guidelines:**
- Each line can be parsed independently
- Parallel processing friendly
- Suitable for streaming ingestion

### 4.3 Compressed Dumps

**Format:** Gzipped NDJSON  
**Update Frequency:** Nightly  
**Naming Convention:** `events-{YYYY-MM-DD}.jsonld.gz`

**Example:**
```bash
curl https://toronto.togather.foundation/datasets/events-2025-07-15.jsonld.gz | \
  gunzip | \
  while IFS= read -r line; do
    echo "$line" | jq '.name'
  done
```

---

## 5. Change Feed

**Endpoint:** `GET /api/v1/feeds/changes?since={cursor}&limit={n}`

Returns ordered list of change envelopes for synchronization and incremental updates.

### 5.1 Response Structure

```json
{
  "cursor": "c2VxXzEwNDg1NzY",
  "changes": [
    {
      "action": "create",
      "uri": "https://toronto.togather.foundation/events/01HYX3...",
      "changed_at": "2025-07-10T12:00:00Z",
      "snapshot": { "@id": "...", "@type": "Event", ... }
    },
    {
      "action": "update",
      "uri": "https://toronto.togather.foundation/events/01HYX4...",
      "changed_at": "2025-07-10T12:05:00Z",
      "changed_fields": ["/name", "/startDate"],
      "snapshot": { ... }
    },
    {
      "action": "delete",
      "uri": "https://toronto.togather.foundation/events/01HYX5...",
      "changed_at": "2025-07-10T12:10:00Z",
      "tombstone": { "@id": "...", "sel:deletedAt": "..." }
    }
  ],
  "next_cursor": "c2VxXzEwNDg2MDI"
}
```

### 5.2 Cursor Rules

- Cursor MUST be opaque (implementations SHOULD use base64url encoding per RFC 4648 §5)
- Cursor MUST be stable (same logical position = same cursor value)
- Ordering MUST be deterministic using a **per-node monotonic sequence** (append-only bigint)
- Delete MUST be represented even if tombstone-only
- Clients MUST treat cursors as opaque strings (never parse or construct manually)

### 5.3 Change Entry Contract (MVP)

| Field | Required | Description |
|-------|----------|-------------|
| `action` | MUST | One of: `create`, `update`, `delete` |
| `uri` | MUST | Entity URI |
| `changed_at` | MUST | RFC3339 timestamp |
| `snapshot` | MUST (for create/update) | Full entity representation |
| `tombstone` | MUST (for delete) | Minimal tombstone with `sel:deletedAt` |
| `changed_fields` | OPTIONAL | Array of JSON Pointers for updated fields |

### 5.4 Optional Enrichment Fields

Implementations MAY include additional metadata fields:

| Field | Type | Description |
|-------|------|-------------|
| `license_url` | URI | License for this entity |
| `license_status` | String | One of: `cc0`, `cc-by`, `proprietary`, `unknown` |
| `source_timestamp` | RFC3339 | When source published the data |
| `received_timestamp` | RFC3339 | When node received the data |
| `federation_uri` | URI | Originating node for multi-hop federation |
| `sequence_number` | Integer | Explicit sequence (if not in cursor) |

Consumers MUST gracefully ignore unknown fields per JSON-LD extensibility.

### 5.5 Usage Example

**Initial Fetch:**
```bash
curl https://toronto.togather.foundation/api/v1/feeds/changes?limit=100
```

**Response:**
```json
{
  "cursor": "c2VxXzEwMDA",
  "changes": [...],
  "next_cursor": "c2VxXzEwMTA"
}
```

**Continue from last cursor:**
```bash
curl "https://toronto.togather.foundation/api/v1/feeds/changes?since=c2VxXzEwMTA&limit=100"
```

### 5.6 Federation Sync

For federation-specific protocols, see [FEDERATION_v1.md](./FEDERATION_v1.md).

---

## 6. Reconciliation Contracts

SEL nodes MAY provide reconciliation endpoints for matching entities to external knowledge graphs (Artsdata, Wikidata, etc.).

### 6.1 Request Contract

```json
{
  "type": "Place|Organization|Person",
  "name": "The Drake Hotel",
  "url": "https://thedrake.ca",
  "limit": 3,
  "properties": [
    { "pid": "schema:addressLocality", "v": "Toronto" },
    { "pid": "schema:postalCode", "v": "M6J" }
  ]
}
```

**Parameters:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | String | MUST | Entity type: `Place`, `Organization`, `Person`, or `Event` |
| `name` | String | MUST | Primary name to match |
| `url` | URI | SHOULD | Official website |
| `limit` | Integer | OPTIONAL | Max candidates to return (default 3) |
| `properties` | Array | OPTIONAL | Additional properties for matching |

**Property Format:**
- `pid`: Property ID (schema.org term)
- `v`: Value

### 6.2 Response Contract

```json
{
  "candidates": [
    {
      "id": "http://kg.artsdata.ca/resource/K12-999",
      "name": "The Drake Hotel",
      "score": 98.5,
      "match": true,
      "type": "Place",
      "properties": {
        "schema:addressLocality": "Toronto",
        "schema:postalCode": "M6J 1M1"
      }
    }
  ],
  "decision": {
    "status": "auto_high",
    "selected_id": "http://kg.artsdata.ca/resource/K12-999",
    "confidence": 0.985
  }
}
```

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `candidates` | Array | List of potential matches |
| `candidates[].id` | URI | External knowledge graph URI |
| `candidates[].name` | String | Matched entity name |
| `candidates[].score` | Float | Match score (0-100) |
| `candidates[].match` | Boolean | High-confidence match flag |
| `candidates[].type` | String | Entity type |
| `candidates[].properties` | Object | Additional matched properties |
| `decision.status` | String | `auto_high`, `auto_low`, or `reject` |
| `decision.selected_id` | URI | Recommended URI (if auto_high) |
| `decision.confidence` | Float | Confidence score (0-1) |

### 6.3 Confidence Thresholds (MVP Defaults)

| Threshold | Score Range | Action |
|-----------|-------------|--------|
| `auto_high` | >= 95 AND `match=true` | Accept automatically |
| `auto_low` | 80-94 | Store as candidate, require review |
| `reject` | < 80 | No match (cache negative for TTL) |

Thresholds MUST be configurable per entity type.

### 6.4 Minting Rule (Hard Constraint)

> **MUST:** SEL MUST NOT mint an Artsdata ID if reconciliation returns any candidate above `auto_low` unless explicitly overridden by admin with audit trail.

### 6.5 Reconciliation Cache Rules (MVP)

**Cache Key Normalization:**
- `type` lowercased
- `name` trimmed, lowercased, collapse whitespace
- `url` canonicalized (strip query + fragment)
- `addressLocality`, `postalCode` normalized (upper-case, trim)

**TTL Defaults:**
- Positive match: 30 days
- Negative match: 7 days

**Idempotency:** Cache lookups MUST be deterministic for the same normalized key.

### 6.6 Endpoint Location

Implementations SHOULD expose reconciliation at:

**Endpoint:** `POST /api/v1/reconcile/{entity_type}`

**Example:**
```bash
curl -X POST https://toronto.togather.foundation/api/v1/reconcile/places \
  -H "Content-Type: application/json" \
  -d '{
    "type": "Place",
    "name": "The Drake Hotel",
    "properties": [
      {"pid": "schema:addressLocality", "v": "Toronto"}
    ]
  }'
```

---

## 7. Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0-DRAFT | 2026-01-27 | Split from monolithic profile document |
| 1.0-DRAFT | 2025-01-20 | Initial draft |

---

## 8. Related Documents

- **Core Profile:** [CORE_PROFILE_v0.1.md](./CORE_PROFILE_v0.1.md) - URI schemes, data models, validation
- **Federation Protocol:** [FEDERATION_v1.md](./FEDERATION_v1.md) - Sync protocols (§ 4.3-4.4)
- **Knowledge Graph Integration:** [knowledge_graph_integration_strategy.md](./knowledge_graph_integration_strategy.md)
