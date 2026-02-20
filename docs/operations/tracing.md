# OpenTelemetry Tracing

The Togather server includes OpenTelemetry tracing instrumentation for distributed request monitoring and debugging. Tracing is **opt-in** and disabled by default to ensure zero performance overhead and no breaking changes to existing deployments.

## Quick Start

### Enable Basic Tracing (Development)

Add to your `.env` file:

```bash
TRACING_ENABLED=true
TRACING_EXPORTER=stdout
```

Restart the server and you'll see human-readable traces in the console output:

```
{
  "Name": "GET /api/v1/events",
  "SpanContext": {
    "TraceID": "4bf92f3577b34da6a3ce929d0e0e4736",
    "SpanID": "00f067aa0ba902b7",
    ...
  },
  "Status": {
    "Code": "Ok"
  },
  "Attributes": [
    {"Key": "http.method", "Value": {"Type": "STRING", "Value": "GET"}},
    {"Key": "http.status_code", "Value": {"Type": "INT64", "Value": 200}},
    {"Key": "request_id", "Value": {"Type": "STRING", "Value": "..."}}
  ]
}
```

### Production Setup (OTLP Collector)

For production monitoring with tools like Jaeger, Zipkin, or cloud observability platforms:

```bash
TRACING_ENABLED=true
TRACING_EXPORTER=otlp
TRACING_OTLP_ENDPOINT=localhost:4317
TRACING_SAMPLE_RATE=0.1  # Trace 10% of requests
```

## Configuration Options

All tracing configuration is done via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `TRACING_ENABLED` | `false` | Enable/disable tracing. When `false`, zero overhead. |
| `TRACING_SERVICE_NAME` | `togather-sel-server` | Service identifier in traces |
| `TRACING_EXPORTER` | `stdout` | Where to send traces: `stdout`, `otlp`, or `none` |
| `TRACING_OTLP_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint (only used with `otlp` exporter) |
| `TRACING_SAMPLE_RATE` | `1.0` | Sample rate (0.0 to 1.0). `1.0` = 100%, `0.1` = 10% |

### Exporter Types

- **`stdout`** (default): Human-readable traces to console
  - Best for: Development, debugging, local testing
  - Performance: Low overhead, no network calls
  
- **`otlp`**: Send traces to OpenTelemetry Collector via OTLP protocol
  - Best for: Production, centralized monitoring, observability platforms
  - Requires: Running OTLP collector (Jaeger, Zipkin, or cloud service)
  
- **`none`**: Generate traces but don't export them
  - Best for: Testing instrumentation without output
  - Performance: Minimal overhead, no I/O

### Sample Rate

The sample rate controls what percentage of requests are traced:

- `1.0` (100%): Trace all requests - use for development or low-traffic services
- `0.1` (10%): Trace 10% of requests - good balance for production
- `0.01` (1%): Trace 1% of requests - use for high-traffic services
- `0.0` (0%): Disable tracing (same as `TRACING_ENABLED=false`)

Sampling decisions are made using trace ID hashing for consistent distributed tracing.

## What Gets Traced

The server automatically creates spans for:

1. **HTTP Requests** - Every incoming HTTP request gets a span with:
   - HTTP method, URL, route pattern
   - Status code, user agent
   - Correlation ID (request_id)
   - Response time

2. **Trace Context Propagation** - Follows W3C Trace Context standard:
   - Extracts `traceparent` and `tracestate` headers from incoming requests
   - Propagates trace context to downstream services
   - Links spans across service boundaries

## Span Attributes

Each HTTP request span includes these attributes:

- `http.method` - HTTP method (GET, POST, etc.)
- `http.url` - Full request URL
- `http.route` - Matched route pattern (e.g., `/api/v1/events/{id}`)
- `http.status_code` - Response status code
- `http.scheme` - http or https
- `http.user_agent` - Client user agent
- `net.host.name` - Server hostname
- `request_id` - Correlation ID from request headers

Spans are marked as errors for 4xx/5xx status codes.

## Integration with OpenTelemetry Ecosystem

### Jaeger (Docker Compose)

```yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "4317:4317"  # OTLP gRPC
      - "16686:16686"  # Jaeger UI
    environment:
      - COLLECTOR_OTLP_ENABLED=true

  togather-server:
    environment:
      - TRACING_ENABLED=true
      - TRACING_EXPORTER=otlp
      - TRACING_OTLP_ENDPOINT=jaeger:4317
```

Access Jaeger UI at http://localhost:16686

### Cloud Observability Platforms

Most cloud platforms support OTLP:

**Google Cloud Trace:**
```bash
TRACING_ENABLED=true
TRACING_EXPORTER=otlp
TRACING_OTLP_ENDPOINT=cloudtrace.googleapis.com:443
```

**AWS X-Ray (via OTEL Collector):**
```bash
TRACING_ENABLED=true
TRACING_EXPORTER=otlp
TRACING_OTLP_ENDPOINT=localhost:4317  # Collector forwards to X-Ray
```

**Datadog, New Relic, Honeycomb:**
Each provides OTLP-compatible endpoints - check their documentation.

## Custom Spans (For Future Development)

To add custom spans in application code:

```go
import (
    "github.com/Togather-Foundation/server/internal/telemetry"
    "go.opentelemetry.io/otel/attribute"
)

func (s *Service) ProcessEvent(ctx context.Context, event Event) error {
    tracer := telemetry.GetTracer("github.com/Togather-Foundation/server/internal/domain/events")
    
    ctx, span := tracer.Start(ctx, "ProcessEvent")
    defer span.End()
    
    // Add custom attributes
    span.SetAttributes(
        attribute.String("event.id", event.ID),
        attribute.String("event.type", event.Type),
    )
    
    // Your logic here
    if err := s.validate(ctx, event); err != nil {
        span.RecordError(err)
        return err
    }
    
    return nil
}
```

## Performance Considerations

### When Tracing is Disabled (Default)

- **Zero overhead** - No spans created, no exporters initialized
- **No dependencies** - OpenTelemetry code paths not executed
- Safe for all environments

### When Tracing is Enabled

- **Overhead per request:**
  - Span creation: ~10-50 microseconds
  - Attribute recording: ~1-5 microseconds per attribute
  - Total: typically < 100 microseconds per request

- **Sampling reduces overhead:**
  - `TRACING_SAMPLE_RATE=0.1` means 90% of requests skip span processing
  - Sampled-out requests only pay for sampling decision (~1 microsecond)

- **Exporter impact:**
  - `stdout`: Minimal (single write per span)
  - `otlp`: Batched network sends (default: 512 spans per batch)
  - `none`: Zero exporter overhead

### Recommended Settings

| Environment | Enabled | Exporter | Sample Rate |
|-------------|---------|----------|-------------|
| Development | `true` | `stdout` | `1.0` |
| Staging | `true` | `otlp` | `1.0` |
| Production (low traffic) | `true` | `otlp` | `1.0` |
| Production (high traffic) | `true` | `otlp` | `0.1` |

## Troubleshooting

### No traces appearing

1. Check `TRACING_ENABLED=true` in `.env`
2. Verify exporter configuration:
   - `stdout`: Check console output for trace JSON
   - `otlp`: Ensure collector is running and reachable
3. Check sample rate: `TRACING_SAMPLE_RATE=1.0` to trace all requests
4. Look for startup errors in server logs: `"tracing enabled"` message

### Traces missing attributes

- Ensure middleware order is correct (Tracing before CorrelationID)
- Check that context is properly propagated through request chain

### OTLP connection errors

```
failed to export spans: connection refused
```

- Verify collector is running: `docker ps | grep jaeger`
- Check endpoint: `TRACING_OTLP_ENDPOINT=localhost:4317`
- Try `stdout` exporter to test instrumentation independently

### High memory usage

- Reduce sample rate: `TRACING_SAMPLE_RATE=0.1`
- Check for span leaks (spans not ended)
- Monitor exporter batch settings (default should be fine)

## Security Considerations

- **No sensitive data in spans** - Avoid recording passwords, tokens, or PII
- **OTLP endpoint security** - Use TLS in production (configure via collector)
- **Access control** - Limit who can view traces (configure in observability platform)

## Future Enhancements

Potential tracing improvements (not yet implemented):

- Database query spans (track slow queries)
- Background job spans (River job processing)
- External API call spans (federation sync, etc.)
- Custom business logic spans (event ingestion pipeline)

## References

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/instrumentation/go/)
- [W3C Trace Context Specification](https://www.w3.org/TR/trace-context/)
- [OTLP Specification](https://opentelemetry.io/docs/specs/otlp/)
- [Semantic Conventions for HTTP](https://opentelemetry.io/docs/specs/semconv/http/)

---

**Last Updated:** 2026-02-20
