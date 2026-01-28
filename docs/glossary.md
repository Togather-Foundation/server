# SEL Terminology Glossary

**Version:** 0.1.0  
**Last Updated:** 2026-01-26

This glossary defines canonical terms used throughout the SEL backend codebase and documentation to ensure consistent terminology.

---

## Core Concepts

### Change Feed
**Type:** System Feature  
**Definition:** An ordered, monotonic log of all modifications (create/update/delete operations) to events in the system, exposed via `/api/v1/feeds/changes`.

**Key Properties:**
- Ordered by `sequence_number` (BIGSERIAL, monotonic per-node)
- Includes tombstones for deleted events
- Cursor-based pagination for federation sync
- Immutable once written (append-only)

**Usage Example:**
```json
{
  "sequence_number": 1234,
  "event_id": "01HYX3KQW7ERTV9XNBM2P8QJZF",
  "action": "update",
  "changed_at": "2026-01-26T21:30:00Z",
  "changed_fields": ["/name", "/startDate"]
}
```

**Related Terms:** Federation Sync, Tombstone, Sequence Number

---

### Lifecycle State
**Type:** Event Property  
**Definition:** The current state of an event in its publication lifecycle, controlling visibility and behavior.

**Valid States:**
- `draft` — Not published, visible only to admins
- `published` — Publicly visible and active
- `postponed` — Temporarily suspended, may resume
- `rescheduled` — Date/time changed, see new occurrence
- `sold_out` — No tickets available, still visible
- `cancelled` — Event will not occur, visible with status
- `completed` — Past event, archived but visible
- `deleted` — Soft-deleted, returns 410 Gone with tombstone

**State Transitions:**
```
draft → published → {postponed, rescheduled, sold_out, cancelled, completed}
Any state → deleted (admin action only)
```

**Database Column:** `events.lifecycle_state` (TEXT, indexed)

**Related Terms:** Event Status, Tombstone

---

### Field Provenance
**Type:** Data Attribution  
**Definition:** Source attribution for individual fields within an event, tracking which data source provided each piece of information.

**Key Properties:**
- Stored in `field_provenance` table
- Uses JSON Pointer paths (e.g., `/name`, `/startDate`)
- Includes confidence scores (0.0-1.0)
- Tracks observation timestamps
- Supports conflict resolution via trust levels

**Schema:**
```sql
field_provenance (
  event_id UUID,
  field_path TEXT,           -- JSON Pointer
  value_hash TEXT,
  source_id UUID,
  confidence DECIMAL(3,2),
  observed_at TIMESTAMPTZ,
  is_current BOOLEAN
)
```

**Conflict Resolution Priority:**
1. Highest source trust level
2. Highest confidence score
3. Most recent observation

**Related Terms:** Source Attribution, Provenance Tracking, Trust Level

---

### Federation URI
**Type:** Identifier  
**Definition:** The original, canonical URI of an entity from a federated peer node, preserved when ingesting events from other SEL nodes.

**Key Properties:**
- Never re-minted by receiving nodes
- Stored in `events.federation_uri` (TEXT, nullable)
- Paired with `events.origin_node_id` (UUID FK)
- NULL for locally-created events
- Used in `sameAs` linking for multi-graph connections

**URI Format:**
```
https://{origin_node_domain}/events/{ulid}
```

**Example:**
```json
{
  "@id": "https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
  "schema:sameAs": [
    "https://vancouver.sel.events/events/01HYZ9KQWX1234567890ABCDEF"
  ]
}
```

**Related Terms:** Origin Node, ULID, Canonical URI, sameAs

---

## Authentication & Authorization

### API Key
**Type:** Authentication Credential  
**Definition:** Long-lived bearer token for agent authentication, issued to data sources and scrapers.

**Properties:**
- 32-byte random token, bcrypt hashed in database
- Prefix (first 8 chars) used for lookup
- Associated with a `source_id` for attribution
- Role: `agent` (300 req/min rate limit)
- Can expire via `expires_at` timestamp

**Database Table:** `api_keys`

**Related Terms:** Agent, Rate Limiting, Source

---

### Role
**Type:** Authorization Level  
**Definition:** Permission tier for access control, determining rate limits and allowed operations.

**Valid Roles:**
- `public` — Unauthenticated, read-only access (60 req/min)
- `agent` — Authenticated data sources, write access (300 req/min)
- `admin` — Full system access, no rate limits

**Related Terms:** RBAC, API Key, JWT

---

## Data Model

### ULID
**Type:** Identifier Format  
**Definition:** Universally Unique Lexicographically Sortable Identifier, used for all entity primary identifiers.

**Properties:**
- 26 characters (Crockford Base32 encoding)
- Monotonic timestamp prefix (sorts chronologically)
- 80 bits of randomness (collision-resistant)
- URL-safe, case-insensitive

**Example:** `01HYX3KQW7ERTV9XNBM2P8QJZF`

**Library:** `oklog/ulid/v2`

**Related Terms:** Canonical URI, Federation URI

---

### Event Occurrence
**Type:** Entity  
**Definition:** A single temporal instance of an event, representing one specific date/time when the event happens.

**Key Properties:**
- Many-to-one relationship with `events` table
- Includes `start_time`, `end_time`, `timezone`
- Can override event's venue or virtual_url
- Generated columns for local date/time queries
- Supports recurring events and series

**Schema:** `event_occurrences` table

**Related Terms:** Event Series, Recurrence Pattern

---

### Tombstone
**Type:** Deletion Record  
**Definition:** A marker indicating an entity has been deleted, returned as HTTP 410 Gone with metadata about the deletion.

**Properties:**
- Soft delete: `deleted_at` timestamp set on entity
- Returns 410 status with JSON-LD tombstone response
- Included in change feed for federation sync
- Preserves URI for historical references
- Contains deletion timestamp and optional reason

**Example Response:**
```json
{
  "@context": "https://schema.org",
  "@type": "Tombstone",
  "url": "https://toronto.togather.foundation/events/01HYX3KQW7ERTV9XNBM2P8QJZF",
  "dateDeleted": "2026-01-26T21:30:00Z"
}
```

**Related Terms:** Soft Delete, Lifecycle State, Change Feed

---

## Provenance & Sources

### Source
**Type:** Data Provider  
**Definition:** An external system or organization that submits events to the SEL node, tracked for attribution and trust scoring.

**Properties:**
- Registered in `sources` table
- Trust level (1-10, default 5)
- License type (CC0, CC-BY, proprietary, etc.)
- Rate limits and authentication requirements
- Source type: scraper, API, partner, user, federation, manual

**Related Terms:** Field Provenance, API Key, Trust Level

---

### Trust Level
**Type:** Provenance Score  
**Definition:** An integer score (1-10) assigned to a data source, used in conflict resolution when multiple sources provide different values for the same field.

**Scale:**
- **1-3:** Experimental or untested sources
- **4-6:** Standard scrapers and APIs (default: 5)
- **7-8:** Vetted partners and official sources
- **9-10:** Highly trusted authorities (government, primary sources)

**Usage:** Conflict resolution priority: `ORDER BY trust_level DESC, confidence DESC, observed_at DESC`

**Related Terms:** Source, Field Provenance, Confidence

---

### Confidence
**Type:** Data Quality Score  
**Definition:** A decimal value (0.0-1.0) indicating the reliability or certainty of a specific data value, typically from automated extraction or reconciliation.

**Scale:**
- **0.0-0.3:** Low confidence (may need review)
- **0.4-0.6:** Medium confidence (review queue threshold)
- **0.7-0.9:** High confidence (auto-publish)
- **1.0:** Perfect match or manual verification

**Related Terms:** Field Provenance, Trust Level, Review Queue

---

## Federation

### Origin Node
**Type:** Federation Peer  
**Definition:** The SEL node that originally created an event, tracked when receiving federated events from peer nodes.

**Properties:**
- Stored in `events.origin_node_id` (UUID FK to `federation_nodes`)
- NULL for locally-created events
- Paired with `federation_uri` for federated events
- Used to prevent circular federation loops

**Related Terms:** Federation URI, Federation Sync

---

### Sequence Number
**Type:** Ordering Identifier  
**Definition:** A monotonically increasing integer (BIGSERIAL) that orders changes in the change feed, ensuring deterministic federation sync.

**Properties:**
- Unique per node (not globally unique)
- Append-only (never reused or updated)
- Used in change feed cursors: `base64(sequence_number)`
- Survives database compaction/vacuuming

**Database Column:** `event_changes.sequence_number` (BIGSERIAL, indexed)

**Related Terms:** Change Feed, Cursor Pagination

---

## Schema.org Alignment

### Event Status
**Type:** Schema.org Property  
**Definition:** The Schema.org-defined status of an event's occurrence, distinct from SEL's internal lifecycle state.

**Valid Values (Schema.org URIs):**
- `https://schema.org/EventScheduled`
- `https://schema.org/EventPostponed`
- `https://schema.org/EventRescheduled`
- `https://schema.org/EventCancelled`
- `https://schema.org/EventMovedOnline`

**Database Column:** `events.event_status` (TEXT, nullable)

**Related Terms:** Lifecycle State, Attendance Mode

---

### Attendance Mode
**Type:** Schema.org Property  
**Definition:** The mode of attendance for an event (physical, online, or mixed).

**Valid Values (Schema.org URIs):**
- `https://schema.org/OfflineEventAttendanceMode` — Physical only
- `https://schema.org/OnlineEventAttendanceMode` — Virtual only
- `https://schema.org/MixedEventAttendanceMode` — Hybrid

**Database Column:** `events.attendance_mode` (TEXT, nullable)

**Related Terms:** Event Status, Virtual Location

---

## API & Integration

### Cursor Pagination
**Type:** API Pattern  
**Definition:** An opaque token-based pagination method using base64-encoded identifiers for stable, efficient result sets.

**Properties:**
- For events list: `base64(timestamp + ULID)` for time-ordered stability
- For change feed: `base64(sequence_number)` for monotonic ordering
- Response includes `next_cursor` field
- Query parameter: `?after={cursor}&limit={count}`

**Advantages over offset pagination:**
- Stable under concurrent modifications
- Efficient database queries (index-only scans)
- No missing or duplicate results

**Related Terms:** Sequence Number, Change Feed

---

### Idempotency Key
**Type:** Request Deduplication  
**Definition:** A client-provided unique identifier (`Idempotency-Key` header) ensuring duplicate submissions return the same result.

**Properties:**
- Required for POST /api/v1/events
- SHA-256 hash stored in `idempotency_keys` table
- 24-hour TTL with automatic cleanup job
- Returns cached response on duplicate submission
- Prevents double-creation from retries

**Related Terms:** Deduplication, Event Ingestion

---

## Testing & Validation

### Contract Test
**Type:** Test Category  
**Definition:** Tests that validate external contracts and specifications, including JSON-LD formats, OpenAPI spec compliance, and SHACL validation.

**Examples:**
- JSON-LD framing correctness
- SHACL shape validation against `shapes/*.ttl`
- OpenAPI spec sync with implemented routes
- RFC 7807 error format compliance

**Location:** `tests/contracts/*_test.go`

**Related Terms:** Integration Test, SHACL Validation

---

### SHACL Validation
**Type:** Semantic Validation  
**Definition:** Validation of RDF/JSON-LD output against SHACL (Shapes Constraint Language) shapes to ensure semantic correctness.

**Properties:**
- Shape files: `shapes/*.ttl`
- Uses `pyshacl` via subprocess (development/CI only)
- ~150-200ms overhead per validation
- Disabled in production (uses fast app-level validation)
- Validates Schema.org compliance and required fields

**Environment Variable:** `SHACL_VALIDATION_ENABLED=true` (default)

**Related Terms:** Contract Test, JSON-LD, Turtle Serialization

---

## Glossary Maintenance

**Adding New Terms:**
1. Use consistent structure: Type, Definition, Properties/Examples
2. Cross-reference related terms at the end of each entry
3. Include code examples or schema definitions where helpful
4. Maintain alphabetical order within sections

**Review Schedule:** Quarterly or when introducing new core concepts

---

**Questions?**  
File an issue at [togather/server/issues](https://github.com/Togather-Foundation/server/issues) with the "documentation" label.
