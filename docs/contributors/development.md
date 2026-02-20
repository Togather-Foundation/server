# Development Guide

This document provides guidelines and best practices for developing the Togather SEL backend server.

## Building the Server

The SEL server can be built using either Make or Go directly. Both methods produce the same output location.

### Build Commands

```bash
# Recommended: Use Make (includes version metadata from git)
make build

# If you see a robots.txt embed error, generate web files first
make webfiles

# Alternative: Direct Go build (version shows as "dev")
go build ./cmd/server

# Both create: ./server (in project root)
```

### Binary Location

- **Output**: `./server` (consistent for both build methods)
- **Clean**: `make clean` removes `./server` and other build artifacts
- **Run**: `make run` builds and runs the server
- **Dev mode**: `make dev` runs with live reload (if `air` is installed)

### Version Information

The build includes metadata from git:

```bash
./server version
# Output:
# Togather SEL Server
# Version:    v0.1.0-5-g04a7d28-dirty
# Git commit: 04a7d28
# Build date: 2026-01-29T21:00:00Z
# Go version: go1.25.6
# Platform:   linux/amd64
```

### Configuration Loading

The server automatically loads configuration from `.env` files in the project root for development and test environments. In staging/production, config is loaded from explicit environment variables or the optional `ENV_FILE` path.

- **serve command**: Loads `.env` via `config.Load()` when `ENVIRONMENT` is development/test
- **All CLI commands**: Now load `.env` automatically in development/test (e.g., `ingest`, `api-key`, etc.)
- **Priority**: Environment variables > `ENV_FILE` (staging/prod) or `.env` (dev/test) > defaults

**Global CLI Flags** (available for all commands):
```bash
--config <path>     Custom config file (optional, defaults to .env in project root)
--log-level <level> Log level: debug, info, warn, error (default: info)
--log-format <fmt>  Log format: json, console (default: json)
```

Example:
```bash
# .env file contains:
API_KEY=01KG5PE0V9TGJQADQ0QT1KJ2E9secret

# Commands read from .env automatically:
./server ingest events.json          # Uses API_KEY from .env
./server api-key list                # Uses DATABASE_URL from .env
./server serve                       # Uses all config from .env
./server serve --log-level debug --log-format console  # Override log settings
```

**Secret Generation:**
- The `./server setup` command automatically generates cryptographically secure secrets using Go's `crypto/rand` package
- Generated secrets: `JWT_SECRET` (32 bytes), `CSRF_KEY` (32 bytes), admin password (16 bytes)
- All secrets are base64-encoded and written to `.env` file in the project root
- API keys created with `./server api-key create` are also saved to `.env` automatically
- See `cmd/server/cmd/setup.go` for implementation details

## Logging Standards

The server uses [zerolog](https://github.com/rs/zerolog) for structured logging with the following standards:

### Configuration

Logging is configured via environment variables:

```bash
LOG_LEVEL=info       # debug, info, warn, error
LOG_FORMAT=console   # console (pretty) or json (production)
```

- **Development**: Use `LOG_FORMAT=console` for human-readable output
- **Production**: Use `LOG_FORMAT=json` for machine-parseable logs

### Request Correlation IDs

Every HTTP request automatically receives a correlation ID:

- Extracted from `X-Request-ID` header (if provided by proxy/load balancer)
- Generated as UUID if not present
- Included in all logs for that request
- Returned in `X-Request-ID` response header for client-side tracing

### Logging in Handlers

HTTP handlers should retrieve the logger from context:

```go
import (
    "github.com/Togather-Foundation/server/internal/api/middleware"
    "github.com/rs/zerolog"
)

func MyHandler(w http.ResponseWriter, r *http.Request) {
    logger := middleware.LoggerFromContext(r.Context())
    
    logger.Info().
        Str("event_id", eventID).
        Str("action", "created").
        Msg("event created successfully")
}
```

The logger from context automatically includes the `request_id` field.

### Logging Levels

Use appropriate log levels:

- **Debug**: Detailed diagnostic information (disabled in production)
  ```go
  logger.Debug().Str("query", sql).Msg("executing query")
  ```

- **Info**: General informational messages about normal operations
  ```go
  logger.Info().Str("user_id", userID).Msg("user logged in")
  ```

- **Warn**: Warning messages for recoverable issues or client errors (4xx)
  ```go
  logger.Warn().Err(err).Msg("invalid input received")
  ```

- **Error**: Error messages for server failures (5xx)
  ```go
  logger.Error().Err(err).Msg("database connection failed")
  ```

### Structured Fields

Always use structured fields (key-value pairs) instead of string interpolation:

**Good:**
```go
logger.Info().
    Str("event_id", eventID).
    Str("source_uri", sourceURI).
    Int("occurrences", len(occurrences)).
    Msg("event ingested")
```

**Bad:**
```go
logger.Info().Msgf("event %s ingested from %s with %d occurrences", eventID, sourceURI, len(occurrences))
```

### Standard Field Names

Use consistent field names across the codebase:

| Field Name | Type | Description |
|------------|------|-------------|
| `request_id` | string | HTTP request correlation ID (automatically added) |
| `event_id` | string | Event ULID |
| `place_id` | string | Place ULID |
| `org_id` | string | Organization ULID |
| `user_id` | string | User UUID |
| `api_key_hash` | string | Hashed API key for authentication logs |
| `source_uri` | string | Original source URI for federated entities |
| `method` | string | HTTP method |
| `path` | string | HTTP path |
| `status` | int | HTTP status code |
| `duration` | duration | Request/operation duration |
| `bytes` | int | Response size in bytes |
| `error` | string | Error message (use `.Err(err)` helper) |

### Error Logging

Errors are automatically logged by the problem package:

- **5xx errors**: Logged at `error` level
- **4xx errors**: Logged at `warn` level

```go
// This automatically logs the error with context
problem.Write(w, r, http.StatusInternalServerError, 
    "https://sel.events/problems/server-error", 
    "Server error", err, h.Env)
```

### Background Jobs

Background jobs (River) use `log/slog` for consistency with River's internal logging:

```go
import "log/slog"

func (w MyWorker) Work(ctx context.Context, job *river.Job[MyArgs]) error {
    // River provides a logger in context
    logger := rivercommon.LoggerFromContext(ctx)
    logger.Info("job started", "job_id", job.ID, "event_id", job.Args.EventID)
    return nil
}
```

### PII and Sensitive Data

- **Never log** passwords, API keys, tokens, or secrets
- **Redact PII** in production (emails, full names, addresses)
- Use hashed values for authentication logs: `api_key_hash`, not `api_key`

Example (from `cmd/server/main.go`):

```go
// Log admin creation - redact email in production
if cfg.Environment == "production" {
    logger.Info().Str("username", bootstrap.Username).Msg("bootstrapped admin user")
} else {
    logger.Info().
        Str("email", bootstrap.Email).
        Str("username", bootstrap.Username).
        Msg("bootstrapped admin user")
}
```

### Performance Considerations

- Structured logging has minimal overhead (<1% in most cases)
- Avoid logging in tight loops (e.g., processing large arrays)
- Consider log sampling for high-volume endpoints if needed

### Testing

Set log level to `disabled` in tests to reduce noise:

```go
func TestMyFeature(t *testing.T) {
    logger := zerolog.Nop() // No-op logger for tests
    // ... test code
}
```

## SHACL Validation and Turtle Serialization

The SEL backend validates JSON-LD data against SHACL shapes to ensure conformance with the [SEL Core Profile](../interop/core-profile-v0.1.md).

**⚠️ WARNING: SHACL validation spawns Python processes (~150-200ms overhead per event). Use ONLY in development/CI, NOT in production.**

### Setup

SHACL validation is **disabled by default** for performance reasons.

**Recommended Use Cases:**
- ✅ Development: Catch schema violations early
- ✅ CI/CD: Ensure conformance before deployment  
- ✅ Manual testing: Debug validation issues
- ❌ Production: Use application-level validation instead (see `internal/domain/events/validation.go`)

To enable validation:

1. **Install pyshacl** (choose one method):
   ```bash
   # Recommended: Use uvx (modern Python tool runner)
   make install-pyshacl
   
   # Or manually with uvx:
   uvx pyshacl --help
   
   # Or with pipx:
   pipx install pyshacl
   
   # Or with pip (requires virtual environment):
   pip3 install pyshacl
   ```

2. **Enable validation** via environment variable:
   ```bash
   export SHACL_VALIDATION_ENABLED=true
   ```

3. **Verify setup**:
   ```bash
   make test-contracts  # Run validation tests
   make validate-shapes # Validate shapes against sample data
   ```

### How It Works

**Validation Pipeline:**

1. **JSON-LD → Turtle**: Event data is converted to RDF Turtle format
2. **Shape Loading**: All `.ttl` files in `shapes/` are merged into a single shape graph
3. **pyshacl Validation**: External `pyshacl` tool validates data against shapes
4. **Error Reporting**: Violations are returned as structured errors

**Code Locations:**
- `internal/jsonld/validator.go` - SHACL validator (uvx/pyshacl detection and execution)
- `internal/jsonld/turtle.go` - JSON-LD to Turtle serialization
- `shapes/*.ttl` - SHACL shape definitions (event-v0.1.ttl, place-v0.1.ttl, organization-v0.1.ttl)

### Important Implementation Details

#### Turtle Serialization Requires Datatypes

SHACL shapes require proper RDF datatypes for validation to work. The serializer automatically adds type coercion for common Schema.org properties:

**DateTime properties** (have `T` separator):
```turtle
# Correct (with ^^xsd:dateTime)
schema:startDate "2026-01-01T19:00:00Z"^^xsd:dateTime .
```

Properties: `startDate`, `endDate`, `dateCreated`, `dateModified`, `datePublished`, `uploadDate`, `datePosted`

**Date-only properties** (YYYY-MM-DD format):
```turtle
# Correct (with ^^xsd:date)
schema:birthDate "1990-01-01"^^xsd:date .
```

Properties: `birthDate`, `deathDate`, `foundingDate`, `dissolutionDate`

**Why This Matters:**
- SHACL shapes use `sh:datatype xsd:dateTime` constraints
- Without type coercion, plain string literals won't match the constraint
- Validation will fail silently (pass when it should fail)

#### pyshacl Multi-Shape File Limitation

pyshacl has a quirk with multiple `-s` flags - only the last shape file is properly applied. 

**Solution:** The validator merges all shape files into a single temporary file before validation.

```go
// DON'T: Multiple -s flags (doesn't work correctly)
pyshacl -s event.ttl -s place.ttl -s org.ttl data.ttl

// DO: Merge shapes first (works correctly)
mergedShapes := mergeShapeFiles(event.ttl, place.ttl, org.ttl)
pyshacl -s mergedShapes data.ttl
```

**Implementation:** `validator.go` line 172 (`createMergedShapesFile()` helper)

#### uvx vs Direct pyshacl Execution

The validator supports both execution methods:

1. **uvx** (preferred): Modern Python package runner, works with PEP 668 externally-managed environments
   ```bash
   uvx pyshacl -s shapes.ttl data.ttl
   ```

2. **Direct pyshacl** (fallback): Traditional installed command
   ```bash
   pyshacl -s shapes.ttl data.ttl
   ```

**Detection order:** uvx → pyshacl → disable validation (with warning)

### Testing SHACL Validation

**Unit Tests:**
```bash
go test ./internal/jsonld/... -run Validator
```

**Manual Testing:**
```bash
# Create test event
cat > /tmp/test.json << 'EOF'
{
  "@context": "https://schema.org",
  "@type": "Event",
  "@id": "https://example.org/events/test-123",
  "startDate": "2026-01-01T19:00:00Z"
}
EOF

# Convert to Turtle
go run debug_turtle.go < /tmp/test.json > /tmp/test.ttl

# Validate manually
uvx pyshacl -s shapes/event-v0.1.ttl /tmp/test.ttl
```

**Expected Result:**
```
Validation Report
Conforms: False
Results (1):
Constraint Violation in MinCountConstraintComponent:
  Message: Event must have exactly one name (1-500 characters)
```

### Fail-Open Philosophy

SHACL validation follows a **fail-open** approach for operational resilience:

- If `SHACL_VALIDATION_ENABLED=false` (default), validation is skipped entirely
- If `SHACL_VALIDATION_ENABLED=true` but pyshacl is not installed, validation is disabled with a warning
- If validation encounters errors, the error is logged but the request succeeds
- This ensures the server remains operational even without pyshacl installed

**Production Recommendation:**
- **DO NOT enable SHACL validation in production** - use application-level validation instead
- Application-level validation is fast (<1ms) and doesn't spawn processes
- See `internal/domain/events/validation.go` for production validation logic
- Reserve SHACL validation for development, testing, and CI/CD

**Where SHACL Validation Runs:**
- Federation sync endpoint (`/api/v1/federation/sync`) when `SHACL_VALIDATION_ENABLED=true`
- Validates incoming JSON-LD payloads from federated nodes
- If validation fails, returns 400 Bad Request with error details

**Why Not Production?**
- Development: Catch schema violations early
- CI/CD: Ensure conformance before deployment
- Production: Optional, adds ~150-200ms per validated event

**Why Not Production?**
- Spawns Python process on every validation (~150-200ms overhead)
- Process startup dominates execution time
- Temporary file I/O adds latency
- Not horizontally scalable (no connection pooling for processes)
- Application-level validation is 100-200x faster

### Performance Considerations

**Validation Overhead:**
- ~150-200ms per event (includes process spawn + Python startup)
- Temporary file I/O on each validation
- Shape file merging overhead (cached after first use)

**Optimization Tips:**
- **Production**: Keep validation disabled (default)
- **Development**: Enable for real-time feedback on schema violations
- **CI/CD**: Enable in test pipelines to catch issues before deployment
- **Bulk Imports**: Consider batch validation offline rather than per-event
- **Federation**: Application-level validation runs first (fast), SHACL validation is optional second layer

### Adding New Shapes

To add validation for new entity types:

1. **Create shape file** in `shapes/`:
   ```turtle
   # shapes/person-v0.1.ttl
   @prefix sh: <http://www.w3.org/ns/shacl#> .
   @prefix schema: <https://schema.org/> .
   
   sel:PersonShape
       a sh:NodeShape ;
       sh:targetClass schema:Person ;
       sh:property [
           sh:path schema:name ;
           sh:minCount 1 ;
           sh:datatype xsd:string ;
       ] .
   ```

2. **Shape is auto-loaded** - validator scans `shapes/*.ttl` at startup

3. **Add tests** in `internal/jsonld/validator_test.go`

4. **Update docs** - document required fields in Interoperability Profile


## References

- [zerolog documentation](https://github.com/rs/zerolog)
- [SEL Architecture § 7 (Observability)](./architecture.md)
- [RFC 7807 Problem Details](https://www.rfc-editor.org/rfc/rfc7807.html)
- [pyshacl documentation](https://github.com/RDFLib/pySHACL)
- [SHACL specification (W3C)](https://www.w3.org/TR/shacl/)
- [RDF Turtle specification](https://www.w3.org/TR/turtle/)
