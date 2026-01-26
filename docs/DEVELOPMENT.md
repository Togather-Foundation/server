# Development Guide

This document provides guidelines and best practices for developing the Togather SEL backend server.

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

## Additional Topics

(This document will be expanded with additional development topics as needed)

## References

- [zerolog documentation](https://github.com/rs/zerolog)
- [SEL Architecture § 7 (Observability)](../plan/togather_SEL_server_architecture_design_v1.md)
- [RFC 7807 Problem Details](https://www.rfc-editor.org/rfc/rfc7807.html)
