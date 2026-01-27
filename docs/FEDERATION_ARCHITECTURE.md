# Federation Architecture - Phase 8 Implementation

**Version:** 1.0  
**Date:** 2026-01-26  
**Status:** Implemented (Phase 8, User Story 6)

---

## Overview

This document describes the federation infrastructure implemented in Phase 8 of the SEL Backend Server project. The implementation enables SEL nodes to discover and synchronize changes with peer nodes while preserving data provenance and origin.

## Change Feed Architecture

The change feed provides an ordered, cursor-based stream of all modifications to events in the system. This enables downstream consumers (federation partners, caching layers, analytics systems) to stay synchronized with minimal overhead.

### Sequence-Based Cursors

Change feed uses monotonically increasing sequence numbers (BIGSERIAL) stored in the `event_changes` table. Cursors are encoded as `seq_<number>` (base64) to provide opaque, stable pagination tokens that survive database restarts and don't reveal internal state.

### Cursor Format Separation

The change feed deliberately uses a different cursor format than event list pagination:

- **Event lists:** `timestamp:ULID` cursors for time-ordered browsing
- **Change feeds:** Sequence numbers for guaranteed ordering and no-skip guarantees during sync

This separation ensures that federation sync operations have strict ordering guarantees that general-purpose event browsing doesn't require.

### Action Filtering

Consumers can filter by action type (`create`, `update`, `delete`) to optimize bandwidth. For example, a read-only mirror might only need `create` and `update` actions.

```http
GET /api/v1/feeds/changes?action=create&limit=100
```

### Tombstone Integration

Delete actions include tombstone metadata (`deletion_reason`, `superseded_by`) to help consumers handle merged or cancelled events correctly. Tombstones return HTTP 410 Gone when dereferenced.

Example tombstone in change feed:

```json
{
  "action": "delete",
  "uri": "https://toronto.togather.foundation/events/01HYX3KQW7...",
  "changed_at": "2025-01-20T15:00:00Z",
  "snapshot": {
    "@context": "https://schema.org",
    "@type": "Event",
    "@id": "https://toronto.togather.foundation/events/01HYX3KQW7...",
    "eventStatus": "https://schema.org/EventCancelled",
    "sel:tombstone": true,
    "sel:deletedAt": "2025-01-20T15:00:00Z",
    "sel:deletionReason": "duplicate_merged",
    "sel:supersededBy": "https://toronto.togather.foundation/events/01HYX4MERGED..."
  }
}
```

### Snapshot Inclusion

Each change includes the full event snapshot at that point in time, avoiding N+1 queries for consumers processing the feed. The snapshot uses standard JSON-LD serialization with `@context`.

## Federation Sync Protocol

The sync protocol allows trusted peer nodes to submit events while preserving their original URIs and origin metadata. This prevents URI collisions and maintains global identifier stability.

### URI Preservation

**CRITICAL RULE:** Federated events NEVER have their `@id` re-minted.

The original `federation_uri` is stored exactly as received and becomes the canonical identifier for that event in the local database. A separate local ULID is generated for internal indexing but does NOT replace the federation URI in outputs.

Example:

```
Node A mints: https://nodeA.example.org/events/01ABC
Node B receives via federation sync
Node B stores:
  - federation_uri: https://nodeA.example.org/events/01ABC  (preserved!)
  - id (local ULID): 01XYZ... (internal only)
  - origin_node_id: nodeA.example.org

Node B's API returns: @id = https://nodeA.example.org/events/01ABC
```

### Origin Tracking

Every federated event stores `origin_node_id` referencing the node that originally minted the URI. This enables:

- Trust-based conflict resolution
- Provenance tracking across the federation
- Proper attribution in outputs
- Routing updates back to authoritative source

### Idempotency by URI

The sync endpoint uses `ON CONFLICT (federation_uri)` upsert semantics:

```sql
INSERT INTO events (federation_uri, name, start_date, ...)
VALUES ($1, $2, $3, ...)
ON CONFLICT (federation_uri) 
DO UPDATE SET 
  name = EXCLUDED.name,
  start_date = EXCLUDED.start_date,
  ...
```

Submitting the same event multiple times (with the same full URI) updates the existing record rather than creating duplicates. This makes sync operations safe to retry.

### Authentication

Federation sync requires API key authentication (`Authorization: Bearer <key>`) with agent-tier rate limiting (300 req/min default).

Each trusted peer node receives a dedicated API key stored in the `federation_nodes` registry:

```sql
-- federation_nodes table
CREATE TABLE federation_nodes (
  id TEXT PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  trust_level INTEGER NOT NULL CHECK (trust_level BETWEEN 1 AND 10),
  api_key_hash TEXT NOT NULL,
  ...
);
```

### JSON-LD Validation

Incoming events are validated as proper JSON-LD with required schema.org properties:

**Required fields:**
- `@type: Event`
- `name` (string)
- `startDate` (ISO8601)
- `location` (Place or VirtualLocation)

The `@id` field is extracted and stored as `federation_uri`. Malformed JSON-LD returns HTTP 400 with RFC 7807 error details.

Example validation error:

```json
{
  "type": "https://sel.events/problems/invalid-jsonld",
  "title": "Invalid JSON-LD",
  "status": 400,
  "detail": "Missing required field: startDate",
  "instance": "/api/v1/federation/sync"
}
```

### Trust-Based Conflict Resolution

When multiple sources provide data about the same event (identified by URI), the system uses trust levels from the `federation_nodes` registry combined with field-level provenance to resolve conflicts.

Resolution algorithm:

1. Check `federation_nodes.trust_level` for each source
2. Query `field_provenance` for confidence scores
3. Apply priority: `trust_level DESC, confidence DESC, observed_at DESC`
4. Higher trust sources override lower trust sources for conflicting fields

## Implementation Patterns

### Import Cycle Avoidance

Repository implementations live in `internal/storage/postgres/*_repository.go` and implement interfaces defined in `internal/domain/federation/`. This keeps domain logic separate from storage while avoiding circular dependencies.

```
internal/domain/federation/
  ├── changefeed.go           (ChangeFeedService + ChangeFeedRepository interface)
  ├── sync.go                 (SyncService + SyncRepository interface)
  └── cursor_test.go          (Unit tests)

internal/storage/postgres/
  ├── changefeed_repository.go  (implements ChangeFeedRepository)
  └── sync_repository.go        (implements SyncRepository)
```

### Service Layer Architecture

`ChangeFeedService` and `SyncService` encapsulate all business logic (cursor encoding, URI extraction, validation) and expose clean interfaces to HTTP handlers. Services are stateless and testable in isolation.

### SQL Query Optimization

**Change feed queries:**
- Index on `(sequence_number, action)` for fast pagination and filtering
- LIMIT + cursor-based WHERE clause avoids OFFSET
- Single query fetches changes + snapshots (no N+1)

**Federation sync queries:**
- Unique index on `federation_uri` for fast conflict detection
- ON CONFLICT clause enables atomic upsert
- Foreign key to `federation_nodes` for trust resolution

### Handler Design

HTTP handlers are thin adapters that:

1. Parse request parameters (query strings, headers, body)
2. Call service methods
3. Serialize responses (JSON-LD, RFC 7807 errors)
4. Apply middleware (auth, rate limiting, logging)

All error handling uses RFC 7807 problem details with environment-aware detail levels (stack traces in dev, sanitized in production).

## API Endpoints

### Change Feed

```http
GET /api/v1/feeds/changes?after=<cursor>&limit=<n>&action=<create|update|delete>
```

**Query Parameters:**
- `since` (optional): Resume from cursor (e.g., `seq_1048576`) - per Interop Profile §4.3
- `after` (optional): Legacy alias for `since` cursor parameter (deprecated, use `since`)
- `limit` (optional): Max changes to return (default: 50, max: 200)
- `action` (optional): Filter by action type (`create`, `update`, `delete`)
- `include_snapshot` (optional): Include full event snapshot (default: true)

**Note**: Timestamp-based filtering (e.g., `since=2025-01-20T00:00:00Z`) is not supported in MVP. Use cursor-based pagination only.

**Response (200 OK):**

```json
{
  "cursor": "seq_1048576",
  "changes": [
    {
      "action": "update",
      "uri": "https://toronto.togather.foundation/events/01J...",
      "changed_at": "2025-07-10T12:05:00Z",
      "changed_fields": ["/name", "/description"],
      "snapshot": {
        "@id": "https://toronto.togather.foundation/events/01J...",
        "@type": "Event",
        "name": "Updated Event Name",
        ...
      }
    }
  ],
  "next_cursor": "seq_1048602"
}
```

**Authentication:** Public (rate limited to 60 req/min)

### Federation Sync

```http
POST /api/v1/federation/sync
Authorization: Bearer <api-key>
Content-Type: application/ld+json
```

**Request Body (JSON-LD):**

```json
{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://nodeA.example.org/events/01ABC",
  "name": "Federated Event",
  "startDate": "2025-08-15T19:00:00-04:00",
  "location": {
    "@type": "Place",
    "name": "Example Venue",
    "address": {
      "@type": "PostalAddress",
      "addressLocality": "Toronto",
      "addressRegion": "ON"
    }
  }
}
```

**Response (201 Created - new event):**

```json
{
  "@id": "https://nodeA.example.org/events/01ABC",
  "message": "Event created via federation sync",
  "local_ulid": "01XYZ..."
}
```

**Response (200 OK - updated event):**

```json
{
  "@id": "https://nodeA.example.org/events/01ABC",
  "message": "Event updated via federation sync",
  "local_ulid": "01XYZ..."
}
```

**Authentication:** API key required (agent tier, 300 req/min)

## Testing Strategy

All Phase 8 functionality was implemented following TDD:

### Test Files Created

1. **tests/integration/feeds_changes_test.go** - Change feed pagination, filtering, cursors
2. **tests/integration/feeds_tombstone_test.go** - Delete tombstones in feed
3. **tests/integration/federation_sync_auth_test.go** - API key auth, rate limiting
4. **tests/integration/federation_sync_idempotency_test.go** - Duplicate handling, upserts
5. **tests/integration/federation_sync_uri_preservation_test.go** - URI preservation, origin tracking
6. **internal/domain/federation/cursor_test.go** - Unit tests for cursor encoding/decoding

### Test Coverage

- **Integration tests:** 5 files covering all user-facing behavior
- **Unit tests:** 1 file for cursor logic
- **Total lines:** 3,187 lines of tested code
- **Known bugs:** 0 at completion

### TDD Workflow

1. ✅ **Integration tests written first** covering pagination, filtering, auth, idempotency, URI preservation
2. ✅ **Tests verified failing** before any implementation code
3. ✅ **Implementation added** incrementally until all tests passed
4. ✅ **Contract tests** ensure RFC 7807 errors and JSON-LD compliance
5. ✅ **Code review** for security, performance, maintainability

## Key Design Decisions

### Why Sequence Numbers?

**Problem:** Timestamp-based cursors can skip events during concurrent writes.

**Solution:** Sequence numbers (BIGSERIAL) guarantee no events are missed during sync, which is critical for federation correctness.

**Tradeoff:** Sequence numbers are node-local and don't work across distributed databases. For multi-node federation, each node maintains its own sequence space.

### Why Preserve federation_uri?

**Problem:** Re-minting URIs breaks the semantic web.

**Example:**
- Node A mints `https://nodeA.example.org/events/01ABC`
- Node B re-mints as `https://nodeB.example.org/events/01XYZ`
- Result: Global identifier is lost, sameAs links broken

**Solution:** Preserve the original URI as `federation_uri` and use it as the canonical `@id` in all outputs.

**Tradeoff:** Requires separate local ULID for internal indexing.

### Why Separate Local ULID?

**Problem:** Internal systems (full-text search, deduplication, admin UI) expect local IDs.

**Solution:** Generate a local ULID alongside `federation_uri` for internal indexing without polluting the semantic layer.

**Tradeoff:** Two identifiers per event (one external, one internal) adds slight complexity.

### Why API Key Auth?

**Problem:** Federation partners are long-lived, trusted relationships (unlike anonymous public consumers).

**Solution:** API keys provide simple, stateless authentication without session management overhead. Keys are scoped per-node in the `federation_nodes` registry.

**Tradeoff:** Key rotation requires manual coordination with federation partners.

### Why Sequence-Based, Not Timestamp-Based Cursors?

**Problem:** Timestamps can collide or be out of order due to clock skew.

**Solution:** Sequence numbers are guaranteed monotonic and ordered, which eliminates sync bugs where events are skipped or processed twice.

**Tradeoff:** Cannot reconstruct cursor from timestamp; must store last sync cursor.

## Future Enhancements (Post-MVP)

### Webhook Delivery

Push changes to subscribers instead of requiring polling.

**Design:**
- `webhook_subscriptions` table with URL + filters
- `webhook_deliveries` table with retry tracking
- Background job processes change feed and POSTs to subscribers
- Exponential backoff on failures

### Partial Sync

Filters by geographic region or event domain to reduce bandwidth.

**Example:**
```http
GET /api/v1/feeds/changes?region=Ontario&domain=music
```

### Conflict UI

Admin interface for reviewing and resolving federation conflicts.

**Features:**
- Show competing values with sources + trust levels
- Allow manual override
- Record resolution reason
- Apply resolution to future conflicts

### Sync Metrics

Dashboards showing sync lag, error rates, bandwidth per peer.

**Metrics:**
- `federation_sync_lag_seconds` (gauge per node)
- `federation_sync_errors_total` (counter per node + error type)
- `federation_sync_bytes_total` (counter per node)
- `federation_change_feed_cursor_position` (gauge)

### Well-Known Discovery

Advertise capabilities via `/.well-known/sel-profile`.

**Example:**
```json
{
  "@context": "https://sel.events/context/v1",
  "id": "https://toronto.togather.foundation",
  "type": "SELNode",
  "changeFeedEndpoint": "https://toronto.togather.foundation/api/v1/feeds/changes",
  "syncEndpoint": "https://toronto.togather.foundation/api/v1/federation/sync",
  "geographicScope": {
    "addressRegion": "ON",
    "addressLocality": "Toronto"
  },
  "supportedFormats": ["application/ld+json", "text/turtle"],
  "version": "1.0"
}
```

## Related Documentation

- [SEL Server Architecture](./togather_SEL_server_architecture_design_v1.md) - Overall system design
- [SEL Interoperability Profile](./togather_SEL_Interoperability_Profile_v0.1.md) - Federation standards
- [Schema Design](./togather_schema_design.md) - Database schema and migrations
- [Security Architecture](./SECURITY.md) - Authentication and authorization

## Implementation Status

**Status:** ✅ **Completed** (2026-01-26)

**Commits:**
- `7217f68` - feat: implement Phase 8 US6 - Change Feed for Federation (T099-T111)

**Files Changed:** 19 files, 3,187 insertions, 21 deletions

**Beads Closed:**
- `server-mia` - US6: Change Feed for Federation
- `server-j3p` - Phase 8: US6 - Change Feed for Federation
- Tasks T099-T111 (all tests + implementation)

**Tests Passing:** ✅ All integration and unit tests pass

**Build Status:** ✅ `go build ./...` succeeds
