# Research: SEL Backend Server

**Date**: 2026-01-23  
**Branch**: `001-sel-backend`  
**Status**: Complete  
**Input**: Technical context requirements from plan.md, existing design docs in `docs/`

## Executive Summary

This document consolidates research findings for the SEL backend implementation. Most architectural decisions are already documented in the existing design documents:
- [togather_SEL_server_architecture_design_v1.md](../../docs/togather_SEL_server_architecture_design_v1.md)
- [togather_schema_design.md](../../docs/togather_schema_design.md)  
- [togather_SEL_Interoperability_Profile_v0.1.md](../../docs/togather_SEL_Interoperability_Profile_v0.1.md)

This research document focuses on implementation-specific decisions not fully covered in those documents.

---

## 1. HTTP Framework Selection

### Decision: Huma

**Rationale**: Huma builds on Chi and provides automatic OpenAPI 3.1 generation from Go structs, reducing spec drift and enabling client code generation. It's lightweight and doesn't impose heavy abstractions.

**Alternatives Considered**:
- **Gin**: Popular but OpenAPI generation is manual or requires separate tools
- **Echo**: Similar to Gin, less mature OpenAPI support
- **Chi alone**: Would require manual OpenAPI maintenance
- **Fiber**: Fast but deviates from net/http patterns

**Best Practices**:
- Use Huma's operation registration for all endpoints
- Define response types as Go structs with proper tags
- Leverage Huma's built-in validation via `go-playground/validator`
- Export OpenAPI at `/api/v1/openapi.json` as required by FR-012

---

## 2. Database Migration Strategy

### Decision: golang-migrate/migrate

**Rationale**: Simple CLI and library support, PostgreSQL-native, integrates with both CI and application startup. Widely adopted in Go community.

**Alternatives Considered**:
- **goose**: Simpler but less flexible
- **Atlas**: More powerful schema management but adds complexity
- **tern**: Less community adoption

**Best Practices**:
- Migrations in `internal/storage/postgres/migrations/`
- Numbered migration files: `000001_initial_schema.up.sql`, `000001_initial_schema.down.sql`
- Run migrations at application startup with version check
- Never modify existing migration files; always create new ones
- Keep migrations backwards compatible where possible

---

## 3. SQLc Configuration

### Decision: SQLc with pgx driver

**Rationale**: SQLc generates type-safe Go code from SQL queries, ensuring compile-time correctness. The pgx driver is the recommended PostgreSQL driver for Go with better performance and feature support than database/sql.

**Configuration** (`sqlc.yaml`):
```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/storage/postgres/queries"
    schema: "internal/storage/postgres/migrations"
    gen:
      go:
        package: "db"
        out: "internal/storage/postgres"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_prepared_queries: false
        emit_interface: true
        emit_exact_table_names: false
        emit_empty_slices: true
```

**Best Practices**:
- One `.sql` file per entity domain (events.sql, places.sql, etc.)
- Use `-- name: QueryName :one/:many/:exec` annotations
- Generate nullable types for optional fields
- Run `sqlc generate` in CI to catch schema drift

---

## 4. ULID Generation

### Decision: oklog/ulid/v2

**Rationale**: Recommended library in the Interoperability Profile. Generates true ULIDs (not UUID re-encodings) with proper Crockford Base32 encoding.

**Implementation**:
```go
import (
    "crypto/rand"
    "time"
    "github.com/oklog/ulid/v2"
)

func NewULID() string {
    entropy := ulid.Monotonic(rand.Reader, 0)
    return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
```

**Best Practices**:
- Use monotonic entropy source for high-frequency generation
- Generate ULIDs in application code, not database
- Store as TEXT in PostgreSQL (26 chars)
- Create unique index on ULID columns

---

## 5. JSON-LD Serialization

### Decision: piprate/json-gold

**Rationale**: Pure Go JSON-LD processor supporting compaction, expansion, framing. Required for Artsdata-compatible output formatting.

**Integration Points**:
- Compact output using SEL context for API responses
- Framing for specific output shapes (event lists, single events)
- Validation of incoming JSON-LD payloads

**Best Practices**:
- Cache parsed context documents
- Use framing for list responses to ensure consistent structure
- Validate `@context` references match expected patterns
- Support both embedded and referenced contexts

---

## 6. Job Queue (River)

### Decision: River with PostgreSQL backend

**Rationale**: Transactional job queue using the same PostgreSQL instance. Jobs are enqueued atomically with business transactions, preventing orphaned work. Built specifically for Go with modern patterns.

**Use Cases**:
- Background reconciliation with Artsdata
- Vector embedding generation (future)
- Link validation for submitted events
- Periodic cleanup and maintenance tasks

**Best Practices**:
- Enqueue jobs in the same transaction as the triggering event
- Define job types with clear retry policies
- Use dead letter queue for failed jobs
- Monitor queue depth and processing latency

---

## 7. Authentication Implementation

### Decision: Layered auth with API keys (agents) + JWT (admins)

**Rationale**: Different auth mechanisms suit different use cases. Agents need long-lived, scriptable credentials. Admins need session-based authentication with proper expiration.

**API Keys**:
- 32-byte random tokens, base64 encoded
- Store SHA-256 hash in database (never plain text)
- Lookup via prefix (first 8 chars) for efficiency
- Associate with source/agent and role

**JWT**:
- HMAC-SHA256 signing (HS256) for simplicity
- 15-minute access token expiration
- Refresh tokens for session continuity
- Claims: `sub` (user ID), `role`, `exp`, `iat`

**Best Practices**:
- Rate limit login attempts per IP
- Log all auth events for audit
- Rotate signing keys periodically
- Use constant-time comparison for key validation

---

## 8. Rate Limiting

### Decision: Token bucket algorithm with role-based tiers

**Implementation**: In-memory rate limiter with Redis fallback for multi-instance deployments (future).

**Tiers** (per FR-020):
- Public: 60 req/min
- Agents: 300 req/min  
- Admins: unlimited

**Best Practices**:
- Identify by API key first, then IP
- Return `429 Too Many Requests` with `Retry-After` header
- Exempt health check endpoints
- Log rate limit events for abuse detection

---

## 9. Content Negotiation

### Decision: Middleware-based with explicit format parameter fallback

**Supported Formats**:
| Accept Header | Format | Handler |
|---------------|--------|---------|
| `application/ld+json` | JSON-LD | Default for API |
| `application/json` | JSON-LD | Alias |
| `text/html` | HTML | Admin/public pages |
| `text/turtle` | RDF Turtle | Bulk export |

**Implementation**:
- Parse `Accept` header with quality values
- Support `?format=json` query parameter as override
- Default to `application/json` if unspecified (per edge case spec)

---

## 10. Error Handling

### Decision: RFC 7807 Problem Details

**Structure**:
```json
{
  "type": "https://sel.events/problems/validation-error",
  "title": "Invalid request",
  "status": 400,
  "detail": "Missing required field: startDate",
  "instance": "/api/v1/events"
}
```

**Error Types** (to define):
- `validation-error`: Input validation failures
- `not-found`: Resource doesn't exist
- `conflict`: Optimistic locking failure (409)
- `rate-limited`: Rate limit exceeded
- `unauthorized`: Missing or invalid credentials
- `forbidden`: Insufficient permissions

**Best Practices**:
- Include `instance` for request correlation
- Add `errors` array for multiple validation failures
- Log full error details server-side, return safe subset to client
- Use appropriate HTTP status codes

---

## 11. Testing Strategy

### Unit Tests
- Service layer business logic
- Validation functions
- JSON-LD serialization

### Integration Tests (testcontainers-go)
- Full API endpoint tests with real PostgreSQL
- Database migration verification
- JSON-LD round-trip: submit → store → retrieve → validate

### Contract Tests
- SHACL validation against `shapes/*.ttl`
- OpenAPI spec sync verification
- JSON-LD context conformance

### Performance Tests
- Concurrent submission load test (100 concurrent, SC-002)
- Query latency benchmarks (SC-003)

---

## 12. Deployment Configuration

### Environment Variables
```bash
# Database
DATABASE_URL=postgres://user:pass@host:5432/sel?sslmode=require

# Server
PORT=8080
HOST=0.0.0.0
NODE_DOMAIN=toronto.togather.foundation

# Auth
JWT_SECRET=<32+ byte secret>
JWT_EXPIRY=15m

# Rate Limits
RATE_LIMIT_PUBLIC=60
RATE_LIMIT_AGENT=300
```

### Docker Configuration
- Multi-stage build: builder + minimal runtime image
- Embed migrations in binary
- Health check: `HEALTHCHECK CMD curl -f http://localhost:8080/healthz`
- Non-root user for security

---

## Open Questions Resolved

| Question | Resolution |
|----------|------------|
| Vector search in MVP? | **No** — deferred per spec "Out of Scope" |
| Admin UI framework? | **Go templates + embedded static** — per operational simplicity |
| Multi-node auth? | **Per-peer API keys** — per federation sync spec in architecture doc |
| Artsdata reconciliation in MVP? | **Background enrichment only** — auto-reconciliation out of scope |

---

## References

1. [Huma Documentation](https://huma.rocks/)
2. [SQLc Documentation](https://docs.sqlc.dev/)
3. [River Queue](https://riverqueue.com/)
4. [piprate/json-gold](https://github.com/piprate/json-gold)
5. [oklog/ulid](https://github.com/oklog/ulid)
6. [golang-migrate](https://github.com/golang-migrate/migrate)
7. [testcontainers-go](https://golang.testcontainers.org/)
