# Database Guide

**Version:** 0.1.0  
**Date:** 2026-01-27  
**Status:** Living Document

This document provides practical guidance for working with the Togather SEL database. It covers schema design, migrations workflow, query patterns, and database best practices for contributors.

For architectural context, see [ARCHITECTURE.md](ARCHITECTURE.md). For the complete DDL, see [../../docs/togather_schema_design.md](../../docs/togather_schema_design.md).

---

## Table of Contents

1. [Database Overview](#database-overview)
2. [Schema Design Principles](#schema-design-principles)
3. [Core Tables](#core-tables)
4. [Working with Events](#working-with-events)
5. [Provenance and Trust](#provenance-and-trust)
6. [Federation and Sync](#federation-and-sync)
7. [Search and Discovery](#search-and-discovery)
8. [Migrations Workflow](#migrations-workflow)
9. [Query Patterns](#query-patterns)
10. [Performance Optimization](#performance-optimization)
11. [Testing with Database](#testing-with-database)

---

## Database Overview

### Technology Stack

- **PostgreSQL**: 16+ (for latest JSON and vector features)
- **SQLc**: Type-safe Go code generation from SQL
- **golang-migrate**: Migration management
- **River**: Transactional job queue (built on Postgres)

### Required Extensions

The database uses several Postgres extensions for advanced functionality:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";     -- UUID generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";      -- Cryptographic functions
CREATE EXTENSION IF NOT EXISTS "pg_trgm";       -- Trigram similarity (fuzzy search)
CREATE EXTENSION IF NOT EXISTS "btree_gin";     -- Composite indexes
CREATE EXTENSION IF NOT EXISTS "btree_gist";    -- Composite indexes
CREATE EXTENSION IF NOT EXISTS "vector";        -- pgvector for embeddings
CREATE EXTENSION IF NOT EXISTS "postgis";       -- Geospatial support
```

**Installation**: These are installed automatically via migrations. For local dev, ensure your Postgres installation supports these extensions (standard in Postgres 16+).

### Three-Layer Architecture

The database implements a hybrid architecture:

1. **Document Truth Layer**: JSONB columns preserve original payloads with full provenance
2. **Relational Core Layer**: Normalized tables enable fast queries, filtering, and joins
3. **Semantic Export Layer**: JSON-LD generated on-demand (not stored)

**Why this approach?**
- JSONB preserves source data exactly as received (no information loss)
- Relational structure enables efficient queries without full JSON scans
- On-demand JSON-LD generation ensures correct schema.org output

---

## Schema Design Principles

### 1. Schema.org Aligned

All entities map directly to schema.org types with explicit field mappings. Example:

```go
// events.name → schema:name
// events.description → schema:description
// events.event_status → schema:eventStatus
```

This alignment ensures semantic correctness and enables knowledge graph integration.

### 2. Federated by Design

Every event tracks its origin:
- `origin_node_id`: Which SEL node created this event
- `federation_uri`: Original URI from peer node
- **Rule**: NEVER re-mint URIs for federated events (preserve origin URIs)

### 3. Provenance as First-Class

Every data modification is tracked:
- **Sources Registry**: Trust levels (1-10), license info, contact details
- **Field Provenance**: Which source provided each field value
- **Event History**: Bitemporal tracking for audit and rollback

### 4. Temporal Correctness

Events split into two tables:
- `events`: Identity and metadata (stable across reschedules)
- `event_occurrences`: Temporal instances (handles recurring events, reschedules)

**Why?** Postponing an event shouldn't change its URI. The event identity persists; only the occurrence changes.

### 5. License Clarity

Every event tracks its license:
- `license_url`: License URI (default: CC0)
- `license_status`: Enum ('cc0', 'cc-by', 'proprietary', 'unknown')
- `takedown_requested`: Boolean flag for DMCA/takedown requests

**SEL Requirement**: Only CC0-licensed descriptions can be shared via federation. Proprietary content is retained locally with provenance but not exported.

---

## Core Tables

### Events Table

**Purpose**: Canonical identity and metadata for cultural events

**Key Fields**:
- `id` (UUID): Internal primary key
- `ulid` (TEXT): Time-sortable unique identifier (used in URIs)
- `name`, `description`: Core schema.org properties
- `lifecycle_state`: draft, published, postponed, cancelled, completed, deleted
- `event_status`: Schema.org status URIs (EventScheduled, EventCancelled, etc.)
- `organizer_id`, `primary_venue_id`: Relationships to normalized entities
- `dedup_hash`: Generated column for duplicate detection

**Lifecycle States**:

```
draft → published → completed
             ↓
          postponed → rescheduled
             ↓
          cancelled
             ↓
          deleted (merged/spam)
```

**Critical Constraint**: Event must have EITHER `primary_venue_id` OR `virtual_url` (not both null).

### Event Occurrences Table

**Purpose**: Temporal instances of events (handles reschedules and recurring events)

**Key Fields**:
- `event_id`: Foreign key to events table
- `start_time` (TIMESTAMPTZ): UTC timestamp
- `timezone` (TEXT): IANA timezone identifier (e.g., "America/Toronto")
- `local_start_date` (DATE): Computed field for calendar queries
- `local_start_time` (TIME): Computed field for "events at 7 PM" queries
- `venue_id`: Per-occurrence venue (can override event's primary_venue_id)
- `occurrence_status`: Per-occurrence status (scheduled, cancelled, sold_out)

**Why Split Tables?**

1. **Reschedules**: Update occurrence, keep event identity stable
2. **Recurring Events**: One event, many occurrences
3. **Venue Changes**: Touring events can have different venues per occurrence
4. **URI Stability**: Event URI doesn't change when dates shift

**Example: Concert Postponement**

```sql
-- Original event (identity preserved)
INSERT INTO events (ulid, name, lifecycle_state)
VALUES ('01HYX...', 'Jazz Night', 'published');

-- Original occurrence
INSERT INTO event_occurrences (event_id, start_time)
VALUES ('...', '2025-08-15 19:00:00-04');

-- Postponed: Update occurrence, event identity unchanged
UPDATE event_occurrences SET start_time = '2025-09-01 19:00:00-04';
UPDATE events SET lifecycle_state = 'rescheduled';
```

### Places Table

**Purpose**: Venues and locations (physical and virtual)

**Key Fields**:
- `name`: Venue name
- `address_*`: Structured address (street, city, region, postal_code, country)
- `latitude`, `longitude`: Geocoding (PostGIS support)
- `place_type`: Enum (venue, outdoor_space, virtual, other)

**PostGIS Integration**:

```sql
-- Spatial column for advanced queries
ALTER TABLE places ADD COLUMN location GEOGRAPHY(POINT, 4326);
UPDATE places SET location = ST_SetSRID(ST_MakePoint(longitude, latitude), 4326);

-- Query: Events within 5km of point
SELECT e.* FROM events e
JOIN event_occurrences eo ON e.id = eo.event_id
JOIN places p ON eo.venue_id = p.id
WHERE ST_DWithin(p.location, ST_SetSRID(ST_MakePoint(-79.3832, 43.6532), 4326), 5000);
```

### Organizations Table

**Purpose**: Event organizers, promoters, venues-as-organizations

**Key Fields**:
- `name`, `alternate_names`: Official name and known aliases
- `organization_type`: Enum (venue, promoter, arts_org, community_group, etc.)
- `founding_date`, `dissolution_date`: Lifecycle tracking
- `address_*`: Structured address

**Entity Identifiers**: Organizations link to external knowledge graphs via `entity_identifiers` table (Artsdata, Wikidata, ISNI).

### Persons Table

**Purpose**: Performers, artists, speakers

**Key Fields**:
- `name`, `alternate_names`
- `birth_date`, `death_date`
- `biography`: Short bio
- Links to external IDs (MusicBrainz, ISNI, Wikidata)

### Event Performers Table

**Purpose**: Many-to-many relationship between events and performers

**Key Fields**:
- `event_id`, `person_id`
- `role`: Enum (headliner, opening_act, speaker, host, etc.)
- `performance_order`: Integer for billing order

---

## Working with Events

### Creating Events with SQLc

**Step 1: Define Query in `queries/events.sql`**

```sql
-- name: CreateEvent :one
INSERT INTO events (
  ulid, name, description, lifecycle_state,
  organizer_id, primary_venue_id, license_url, license_status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateEventOccurrence :one
INSERT INTO event_occurrences (
  event_id, start_time, end_time, timezone,
  local_start_date, local_start_time, venue_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;
```

**Step 2: Generate Go Code**

```bash
make sqlc
```

**Step 3: Use Generated Code**

```go
func (r *EventRepository) CreateEvent(ctx context.Context, input CreateEventInput) (*Event, error) {
    // Generate ULID
    eventULID := ulid.Make().String()
    
    // Create event (identity)
    event, err := r.queries.CreateEvent(ctx, CreateEventParams{
        Ulid:           eventULID,
        Name:           input.Name,
        Description:    input.Description,
        LifecycleState: "draft",
        OrganizerID:    input.OrganizerID,
        PrimaryVenueID: input.VenueID,
        LicenseUrl:     "https://creativecommons.org/publicdomain/zero/1.0/",
        LicenseStatus:  "cc0",
    })
    if err != nil {
        return nil, fmt.Errorf("create event: %w", err)
    }
    
    // Create occurrence (temporal instance)
    occurrence, err := r.queries.CreateEventOccurrence(ctx, CreateEventOccurrenceParams{
        EventID:        event.ID,
        StartTime:      input.StartTime,
        EndTime:        input.EndTime,
        Timezone:       input.Timezone,
        LocalStartDate: input.StartTime.In(tz).Format("2006-01-02"),
        LocalStartTime: input.StartTime.In(tz).Format("15:04:05"),
        VenueID:        input.VenueID,
    })
    if err != nil {
        return nil, fmt.Errorf("create occurrence: %w", err)
    }
    
    return event, nil
}
```

### Querying Events

**Query: Upcoming Events in City**

```sql
-- name: ListUpcomingEventsByCity :many
SELECT 
    e.id, e.ulid, e.name, e.description,
    eo.start_time, eo.end_time,
    p.name AS venue_name, p.city
FROM events e
JOIN event_occurrences eo ON e.id = eo.event_id
JOIN places p ON eo.venue_id = p.id
WHERE p.city = $1
  AND eo.start_time > NOW()
  AND e.lifecycle_state = 'published'
ORDER BY eo.start_time ASC
LIMIT $2;
```

**Query: Events by Date Range with Pagination**

```sql
-- name: ListEventsByDateRange :many
SELECT 
    e.id, e.ulid, e.name,
    eo.start_time, eo.sequence_number
FROM events e
JOIN event_occurrences eo ON e.id = eo.event_id
WHERE eo.start_time BETWEEN $1 AND $2
  AND eo.sequence_number > $3  -- cursor-based pagination
  AND e.lifecycle_state = 'published'
ORDER BY eo.sequence_number ASC
LIMIT $4;
```

### Handling Recurring Events

**Create Event Series**

```sql
INSERT INTO event_series (
    name, recurrence_rule, default_start_time,
    default_venue_id, organizer_id
)
VALUES (
    'Weekly Jazz Jam',
    'FREQ=WEEKLY;BYDAY=FR', -- RFC 5545 recurrence rule
    '19:00:00',
    '...venue_id...',
    '...organizer_id...'
);
```

**Expand Occurrences**

```go
// Background job to expand recurring events
func (j *OccurrenceExpansionJob) Work(ctx context.Context, args OccurrenceExpansionArgs) error {
    series, err := j.queries.GetEventSeries(ctx, args.SeriesID)
    // Parse recurrence rule using RFC 5545 library
    // Generate occurrence instances for next 6 months
    // Insert into event_occurrences table
}
```

---

## Provenance and Trust

### Sources Registry

Every data source is registered with:
- `name`: Human-readable name (e.g., "Massey Hall Official API")
- `trust_level`: Integer 1-10 (10 = official, 5 = community, 1 = unverified)
- `license`: License for data from this source (cc0, cc-by, proprietary)
- `contact_email`: Maintainer contact

**Creating a Source**

```sql
INSERT INTO sources (name, trust_level, license, source_type)
VALUES ('Massey Hall API', 9, 'cc-by', 'api');
```

### Field-Level Provenance

Track which source provided each field value:

```sql
CREATE TABLE field_provenance (
  id UUID PRIMARY KEY,
  event_id UUID NOT NULL REFERENCES events(id),
  field_name TEXT NOT NULL,  -- 'name', 'description', 'startDate', etc.
  value JSONB NOT NULL,      -- Field value at time of observation
  source_id UUID NOT NULL REFERENCES sources(id),
  confidence DECIMAL(3,2) CHECK (confidence BETWEEN 0 AND 1),
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Recording Provenance**

```go
func (r *EventRepository) UpdateFieldWithProvenance(
    ctx context.Context,
    eventID uuid.UUID,
    fieldName string,
    value interface{},
    sourceID uuid.UUID,
    confidence float64,
) error {
    tx, err := r.db.Begin(ctx)
    defer tx.Rollback()
    
    // Update event field
    err = r.queries.UpdateEventField(ctx, tx, UpdateEventFieldParams{
        ID:        eventID,
        FieldName: fieldName,
        Value:     value,
    })
    
    // Record provenance
    err = r.queries.CreateFieldProvenance(ctx, tx, FieldProvenanceParams{
        EventID:    eventID,
        FieldName:  fieldName,
        Value:      jsonb(value),
        SourceID:   sourceID,
        Confidence: confidence,
        ObservedAt: time.Now(),
    })
    
    return tx.Commit()
}
```

### Conflict Resolution

**View: Field Conflicts**

```sql
CREATE VIEW field_conflicts AS
SELECT 
    event_id,
    field_name,
    COUNT(DISTINCT value) AS value_count,
    jsonb_agg(
        jsonb_build_object(
            'value', value,
            'source', s.name,
            'trust', s.trust_level,
            'confidence', fp.confidence,
            'observed_at', fp.observed_at
        ) ORDER BY s.trust_level DESC, fp.confidence DESC, fp.observed_at DESC
    ) AS candidates
FROM field_provenance fp
JOIN sources s ON fp.source_id = s.id
GROUP BY event_id, field_name
HAVING COUNT(DISTINCT value) > 1;
```

**Query: Get Best Value for Field**

```sql
-- name: GetBestFieldValue :one
SELECT value, s.name AS source_name
FROM field_provenance fp
JOIN sources s ON fp.source_id = s.id
WHERE fp.event_id = $1 AND fp.field_name = $2
ORDER BY s.trust_level DESC, fp.confidence DESC, fp.observed_at DESC
LIMIT 1;
```

---

## Federation and Sync

### Federation Nodes Registry

Track peer SEL nodes:

```sql
CREATE TABLE federation_nodes (
  id UUID PRIMARY KEY,
  node_domain TEXT NOT NULL UNIQUE,  -- e.g., "toronto.togather.foundation"
  display_name TEXT NOT NULL,
  trust_level INTEGER CHECK (trust_level BETWEEN 1 AND 10),
  sync_enabled BOOLEAN DEFAULT true,
  last_sync_cursor TEXT,
  last_sync_at TIMESTAMPTZ
);
```

### Event Changes Outbox

Capture all event modifications for change feed:

```sql
CREATE TABLE event_changes (
  id BIGSERIAL PRIMARY KEY,
  sequence_number BIGINT NOT NULL UNIQUE,  -- Monotonic per-node
  event_id UUID NOT NULL REFERENCES events(id),
  action TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete')),
  changed_fields TEXT[],  -- Array of JSON paths for updates
  snapshot JSONB,  -- Full event state after change
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_event_changes_seq ON event_changes (sequence_number);
CREATE INDEX idx_event_changes_time ON event_changes (changed_at);
```

**Trigger: Capture Changes**

```sql
CREATE OR REPLACE FUNCTION capture_event_change()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO event_changes (
        sequence_number,
        event_id,
        action,
        changed_fields,
        snapshot
    ) VALUES (
        nextval('event_changes_seq'),
        COALESCE(NEW.id, OLD.id),
        TG_OP,
        -- Compute changed fields for UPDATE
        CASE WHEN TG_OP = 'UPDATE' THEN
            array(SELECT key FROM jsonb_each(to_jsonb(NEW))
                  WHERE to_jsonb(NEW)->key IS DISTINCT FROM to_jsonb(OLD)->key)
        ELSE NULL END,
        to_jsonb(NEW)
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER events_change_trigger
AFTER INSERT OR UPDATE OR DELETE ON events
FOR EACH ROW EXECUTE FUNCTION capture_event_change();
```

### Sync Protocol Implementation

**Query: Get Changes Since Cursor**

```sql
-- name: GetChangesSinceCursor :many
SELECT 
    sequence_number,
    event_id,
    action,
    changed_fields,
    snapshot,
    changed_at
FROM event_changes
WHERE sequence_number > $1
ORDER BY sequence_number ASC
LIMIT $2;
```

**Go Implementation**

```go
func (s *FederationService) SyncFromPeer(ctx context.Context, peerID uuid.UUID) error {
    peer, err := s.queries.GetFederationNode(ctx, peerID)
    cursor := peer.LastSyncCursor
    
    for {
        // Fetch batch of changes
        changes, err := s.fetchChangesFromPeer(ctx, peer.NodeDomain, cursor, 100)
        if len(changes) == 0 {
            break
        }
        
        // Apply changes in transaction
        tx, _ := s.db.Begin(ctx)
        for _, change := range changes {
            err = s.applyChange(ctx, tx, change)
            if err != nil {
                tx.Rollback()
                return err
            }
        }
        
        // Update cursor
        cursor = changes[len(changes)-1].Cursor
        err = s.queries.UpdateFederationNodeCursor(ctx, tx, peerID, cursor)
        tx.Commit()
    }
    
    return nil
}
```

---

## Search and Discovery

### Full-Text Search

**GIN Index on Text Fields**

```sql
CREATE INDEX idx_events_search_vector ON events USING GIN (
  to_tsvector('english', 
    COALESCE(name, '') || ' ' || 
    COALESCE(description, '')
  )
);
```

**Query with Full-Text Search**

```sql
-- name: SearchEvents :many
SELECT 
    e.id, e.ulid, e.name,
    ts_rank(
        to_tsvector('english', e.name || ' ' || COALESCE(e.description, '')),
        websearch_to_tsquery('english', $1)
    ) AS rank
FROM events e
WHERE to_tsvector('english', e.name || ' ' || COALESCE(e.description, ''))
      @@ websearch_to_tsquery('english', $1)
  AND e.lifecycle_state = 'published'
ORDER BY rank DESC, e.updated_at DESC
LIMIT $2;
```

### Fuzzy Matching (pg_trgm)

**Similarity Index**

```sql
CREATE INDEX idx_events_name_trgm ON events USING GIN (name gin_trgm_ops);
```

**Query: Find Similar Event Names**

```sql
-- name: FindSimilarEvents :many
SELECT id, name, similarity(name, $1) AS sim
FROM events
WHERE similarity(name, $1) > 0.3
ORDER BY sim DESC
LIMIT 10;
```

### Vector Search (pgvector)

**Embeddings Table**

```sql
CREATE TABLE event_embeddings (
  event_id UUID PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
  embedding vector(384),  -- 384-dimensional vector (adjust based on model)
  model_version TEXT NOT NULL,
  source_text TEXT NOT NULL,  -- Preserve for reindexing
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_event_embeddings_vector ON event_embeddings
USING ivfflat (embedding vector_cosine_ops)
WITH (lists = 100);
```

**Query: Semantic Search**

```sql
-- name: SemanticSearch :many
SELECT 
    e.id, e.ulid, e.name,
    ee.embedding <=> $1::vector AS distance
FROM events e
JOIN event_embeddings ee ON e.id = ee.event_id
WHERE e.lifecycle_state = 'published'
ORDER BY ee.embedding <=> $1::vector
LIMIT $2;
```

**Go Implementation**

```go
func (r *EventRepository) SemanticSearch(ctx context.Context, queryEmbedding []float32, limit int) ([]*Event, error) {
    // Convert float32 slice to pgvector format
    vectorStr := fmt.Sprintf("[%s]", strings.Join(floatsToStrings(queryEmbedding), ","))
    
    events, err := r.queries.SemanticSearch(ctx, vectorStr, limit)
    return events, err
}
```

---

## Migrations Workflow

### Migration Tool

SEL uses **golang-migrate** for database migrations.

**Installation**:
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### Migration Files

Migrations live in `internal/storage/postgres/migrations/` with numbered pairs:

```
001_initial_schema.up.sql
001_initial_schema.down.sql
002_add_federation.up.sql
002_add_federation.down.sql
```

**Naming Convention**: `{version}_{description}.{up|down}.sql`

### Creating Migrations

**Command**:
```bash
migrate create -ext sql -dir internal/storage/postgres/migrations -seq add_event_series
```

This creates:
```
internal/storage/postgres/migrations/003_add_event_series.up.sql
internal/storage/postgres/migrations/003_add_event_series.down.sql
```

**Write SQL**:

`003_add_event_series.up.sql`:
```sql
BEGIN;

CREATE TABLE event_series (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  recurrence_rule TEXT,
  default_start_time TIME,
  default_venue_id UUID REFERENCES places(id),
  organizer_id UUID REFERENCES organizations(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_event_series_organizer ON event_series(organizer_id);

ALTER TABLE events ADD COLUMN series_id UUID REFERENCES event_series(id);

COMMIT;
```

`003_add_event_series.down.sql`:
```sql
BEGIN;

ALTER TABLE events DROP COLUMN series_id;
DROP TABLE event_series;

COMMIT;
```

### Running Migrations

**Up (Apply)**:
```bash
make migrate-up
# Or manually:
migrate -path migrations -database "postgres://localhost/togather_sel" up
```

**Down (Rollback)**:
```bash
make migrate-down
# Rollback one migration:
migrate -path migrations -database "postgres://localhost/togather_sel" down 1
```

**Status**:
```bash
migrate -path migrations -database "postgres://localhost/togather_sel" version
```

### Migration Best Practices

1. **Transactional**: Wrap in `BEGIN; ... COMMIT;` for atomicity
2. **Idempotent**: Use `IF NOT EXISTS` where possible
3. **Rollback-Friendly**: Always write matching `.down.sql`
4. **Test Rollback**: Test both up and down migrations
5. **Data Migrations**: Use separate migration if backfilling data
6. **Index Creation**: Use `CONCURRENTLY` in production (outside transaction)

**Example: Adding Index Safely**

```sql
-- Don't use CONCURRENTLY in transaction
CREATE INDEX CONCURRENTLY idx_events_new_field ON events(new_field);
```

---

## Query Patterns

### Common Queries with SQLc

**Define in `queries/events.sql`**:

```sql
-- name: GetEventByULID :one
SELECT * FROM events WHERE ulid = $1;

-- name: GetEventWithOccurrences :many
SELECT 
    e.*, eo.start_time, eo.end_time, eo.timezone
FROM events e
JOIN event_occurrences eo ON e.id = eo.event_id
WHERE e.id = $1
ORDER BY eo.start_time;

-- name: ListUpcomingEvents :many
SELECT 
    e.id, e.ulid, e.name,
    eo.start_time, eo.local_start_date
FROM events e
JOIN event_occurrences eo ON e.id = eo.event_id
WHERE eo.start_time > $1
  AND e.lifecycle_state = 'published'
ORDER BY eo.start_time ASC
LIMIT $2;

-- name: GetEventsNeedingReconciliation :many
SELECT e.* FROM events e
WHERE NOT EXISTS (
    SELECT 1 FROM entity_identifiers ei
    WHERE ei.entity_id = e.id AND ei.entity_type = 'event'
)
AND e.lifecycle_state = 'published'
LIMIT $1;
```

**Generate Code**:
```bash
make sqlc
```

**Use in Go**:
```go
// Get single event
event, err := queries.GetEventByULID(ctx, "01HYX...")

// Get event with occurrences
rows, err := queries.GetEventWithOccurrences(ctx, eventID)

// List upcoming events
events, err := queries.ListUpcomingEvents(ctx, time.Now(), 50)
```

### Transaction Patterns

**Pattern 1: Simple Transaction**

```go
func (r *EventRepository) CreateEventWithOccurrence(ctx context.Context, input CreateEventInput) error {
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback() // Rollback if not committed
    
    queries := r.queries.WithTx(tx)
    
    event, err := queries.CreateEvent(ctx, ...)
    if err != nil {
        return err
    }
    
    _, err = queries.CreateEventOccurrence(ctx, ...)
    if err != nil {
        return err
    }
    
    return tx.Commit()
}
```

**Pattern 2: Transaction with Job Queue**

```go
func (r *EventRepository) CreateEventWithReconciliation(ctx context.Context, input CreateEventInput) error {
    tx, err := r.db.Begin(ctx)
    defer tx.Rollback()
    
    // Create event
    event, err := r.queries.WithTx(tx).CreateEvent(ctx, ...)
    
    // Enqueue reconciliation job (same transaction!)
    _, err = r.riverClient.InsertTx(ctx, tx, ReconcileEventArgs{
        EventID: event.ID,
    }, nil)
    
    return tx.Commit()
}
```

---

## Performance Optimization

### Indexing Strategy

**Always Index**:
- Primary keys (automatic)
- Foreign keys (for joins)
- Fields used in WHERE clauses frequently
- Fields used in ORDER BY

**Use GIN Indexes For**:
- Full-text search (`to_tsvector`)
- Array containment queries
- JSONB queries
- Trigram similarity

**Use GIST Indexes For**:
- Range queries (time ranges)
- Geospatial queries (PostGIS)

**Example: Query Optimization**

```sql
-- Slow: Sequential scan
EXPLAIN ANALYZE
SELECT * FROM events WHERE lifecycle_state = 'published';

-- Add index
CREATE INDEX idx_events_lifecycle ON events(lifecycle_state);

-- Fast: Index scan
EXPLAIN ANALYZE
SELECT * FROM events WHERE lifecycle_state = 'published';
```

### Query Performance Checks

**Use EXPLAIN ANALYZE**:

```sql
EXPLAIN (ANALYZE, BUFFERS) 
SELECT e.* FROM events e
JOIN event_occurrences eo ON e.id = eo.event_id
WHERE eo.start_time > NOW()
  AND e.lifecycle_state = 'published'
ORDER BY eo.start_time
LIMIT 50;
```

**Look For**:
- Sequential scans on large tables (bad)
- Index scans (good)
- High "Rows Removed by Filter" (needs better index)
- High execution time

### Connection Pooling

**Configure in Go**:

```go
db, err := sql.Open("postgres", databaseURL)
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

**Rationale**:
- 25 max connections (balance between concurrency and Postgres overhead)
- 5 idle connections (quick reuse without constant opening/closing)
- 5-minute lifetime (prevents stale connections)

### Prepared Statements

**SQLc generates prepared statements automatically**:

```go
// This query is prepared once and reused
event, err := queries.GetEventByULID(ctx, ulid)
```

**Benefit**: Postgres caches query plan, reducing parsing overhead.

---

## Testing with Database

### Integration Tests with Testcontainers

**Setup**:

```go
func setupTestDB(t *testing.T) (*sql.DB, func()) {
    ctx := context.Background()
    
    // Start Postgres container
    req := testcontainers.ContainerRequest{
        Image:        "postgres:16-alpine",
        ExposedPorts: []string{"5432/tcp"},
        Env: map[string]string{
            "POSTGRES_DB":       "test",
            "POSTGRES_USER":     "test",
            "POSTGRES_PASSWORD": "test",
        },
        WaitStrategy: wait.ForLog("database system is ready to accept connections"),
    }
    
    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    require.NoError(t, err)
    
    // Get connection string
    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432")
    connStr := fmt.Sprintf("postgres://test:test@%s:%s/test?sslmode=disable", host, port.Port())
    
    // Connect and run migrations
    db, err := sql.Open("postgres", connStr)
    require.NoError(t, err)
    
    runMigrations(t, db)
    
    // Return cleanup function
    cleanup := func() {
        db.Close()
        container.Terminate(ctx)
    }
    
    return db, cleanup
}
```

**Test Pattern**:

```go
func TestEventRepository_CreateEvent(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()
    
    queries := database.New(db)
    repo := NewEventRepository(queries)
    
    event, err := repo.CreateEvent(context.Background(), CreateEventInput{
        Name:        "Test Event",
        Description: "Test Description",
        VenueID:     testVenueID,
    })
    
    require.NoError(t, err)
    assert.NotEmpty(t, event.ULID)
    assert.Equal(t, "Test Event", event.Name)
}
```

### Test Fixtures

**Create Helper Functions**:

```go
func createTestVenue(t *testing.T, queries *database.Queries) uuid.UUID {
    venue, err := queries.CreatePlace(context.Background(), database.CreatePlaceParams{
        Name:        "Test Venue",
        City:        "Toronto",
        Region:      "ON",
        Country:     "CA",
        PlaceType:   "venue",
    })
    require.NoError(t, err)
    return venue.ID
}

func createTestOrganization(t *testing.T, queries *database.Queries) uuid.UUID {
    org, err := queries.CreateOrganization(context.Background(), database.CreateOrganizationParams{
        Name:             "Test Organization",
        OrganizationType: "promoter",
    })
    require.NoError(t, err)
    return org.ID
}
```

### Table-Driven Tests

```go
func TestEventRepository_QueryFilters(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()
    
    // Setup test data
    setupTestEvents(t, db)
    
    tests := []struct {
        name    string
        filters EventFilters
        want    int
    }{
        {
            name:    "published events only",
            filters: EventFilters{LifecycleState: "published"},
            want:    5,
        },
        {
            name:    "events in Toronto",
            filters: EventFilters{City: "Toronto"},
            want:    3,
        },
        {
            name:    "upcoming events",
            filters: EventFilters{StartAfter: time.Now()},
            want:    4,
        },
    }
    
    repo := NewEventRepository(database.New(db))
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            events, err := repo.ListEvents(context.Background(), tt.filters)
            require.NoError(t, err)
            assert.Len(t, events, tt.want)
        })
    }
}
```

---

## Best Practices Summary

### Schema Design
- Always use `TIMESTAMPTZ` (not `TIMESTAMP`) for time storage
- Use `CHECK` constraints for enums (easier to validate than DB enums)
- Use generated columns for computed values (e.g., `dedup_hash`)
- Always include `created_at` and `updated_at` timestamps

### Queries
- Use SQLc for type-safe SQL (no runtime query building)
- Always use parameterized queries (SQL injection prevention)
- Use transactions for multi-step operations
- Use cursor-based pagination for large result sets

### Indexes
- Index foreign keys (enables efficient joins)
- Index frequently filtered/sorted columns
- Use partial indexes with WHERE clause to reduce index size
- Monitor index usage with `pg_stat_user_indexes`

### Testing
- Use testcontainers for real Postgres in tests
- Test migrations (both up and down)
- Test transactions rollback on errors
- Test constraint violations

### Performance
- Use connection pooling (SetMaxOpenConns, SetMaxIdleConns)
- Use prepared statements (SQLc does this automatically)
- Use EXPLAIN ANALYZE to profile slow queries
- Add indexes based on actual query patterns (not guesses)

---

**Next Steps:**
- [TESTING.md](TESTING.md) - Test-driven development workflow
- [API_GUIDE.md](../integration/API_GUIDE.md) - API endpoints and usage
- [Complete Schema DDL](../../docs/togather_schema_design.md) - Full table definitions

**Document Version**: 0.1.0  
**Last Updated**: 2026-01-27  
**Source**: Consolidated from togather_schema_design.md (52KB, 1769 lines)  
**Maintenance**: Update when schema changes or new query patterns emerge
