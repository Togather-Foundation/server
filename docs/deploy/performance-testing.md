# Performance Testing Guide

This guide explains how to use the performance/load testing tools to validate the Togather server's monitoring dashboards and test blue-green deployment performance under load.

## Overview

The performance testing tool generates realistic traffic patterns using test fixtures from the codebase. It supports:

- **Configurable load profiles** (light, medium, heavy, stress, burst, peak)
- **Mix of read/write operations** using realistic event data
- **Targeted testing** of blue/green slots or load-balanced endpoints
- **Detailed metrics** including response times, throughput, errors, and per-endpoint statistics
- **Gradual ramp-up/ramp-down** to simulate realistic traffic patterns

## Quick Start

```bash
# Run a light load test against the blue slot
./deploy/scripts/performance-test.sh --profile light --slot blue

# Run a medium load test against the green slot
./deploy/scripts/performance-test.sh --profile medium --slot green

# Custom test: 30 req/s for 2 minutes
./deploy/scripts/performance-test.sh --rps 30 --duration 2m --slot blue
```

## Prerequisites

1. **Server running**: Ensure the Togather server is running:
   ```bash
   cd deploy/docker
   docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml up -d
   ```

2. **Monitoring stack** (optional but recommended):
   ```bash
   cd deploy/docker
   docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml --profile monitoring up -d
   ```

## Load Profiles

### Predefined Profiles

| Profile | RPS | Duration | Use Case |
|---------|-----|----------|----------|
| `light` | 5 | 1 min | Smoke testing, quick validation |
| `medium` | 20 | 2 min | Typical load, normal operations |
| `heavy` | 50 | 5 min | Peak hours, high traffic |
| `stress` | 100 | 10 min | Stress testing, capacity planning |
| `burst` | 10→100→10 | 5 min | Spike patterns, flash traffic |
| `peak` | 0→40→0 | 10 min | Gradual ramp-up/down simulation |

All profiles include gradual ramp-up and ramp-down periods to simulate realistic traffic patterns.

### Read/Write Ratio

All CLI and wrapper profiles default to **read-only** (`--read-ratio 1`):
GET `/health`, `/api/v1/events`, `/api/v1/places`, and `/api/v1/organizations`.
Protected `/metrics` is excluded from this public workload. `test-remote.sh` explicitly
passes read-only mode and a zero error budget during routine post-deployment checks.

Write traffic requires an explicit ratio below 1 and explicit credentials:

```bash
# Use only with a disposable database or positively verified fixture ownership.
./deploy/scripts/performance-test.sh --profile light --read-ratio 0.8 --api-key "$LOADTEST_API_KEY"
```

Before sustained writes, each credential is checked with a POST containing malformed
JSON (`{`) to the authenticated event-create route. The expected HTTP 400 is an explicit
validation-only expectation: decoding fails before ingest, so this probe creates no
event. HTTP 401/403 or any other status aborts the run before sustained traffic.
This establishes authentication, not permission to ingest every generated fixture.
Actual writes still need the isolation and cleanup precautions in
[Load Testing Operations](../operations/load-testing.md).

`--max-error-rate` sets the permitted fraction of ordinary non-authentication 4xx
failures (default 0). HTTP 401/403, 5xx, transport/body failures, cancellation and empty
runs always fail, even with a larger budget. Redirects are not followed. There is no
implicit exemption for rate limiting or authentication failures. Negative-security
checks belong in explicitly specified tests with their own expected status codes.

Use `--json` with the Go CLI for a machine-readable result containing `passed`,
`failed_expectations`, totals and per-endpoint `status_codes` (0 denotes transport/body
failure). The shell wrapper forwards this option but also prints its own progress;
use the binary directly when stdout must be JSON only. Both the CLI and wrappers exit
nonzero when expectations fail. Keys are omitted from result and debug output.

## Usage Examples

### Basic Usage

```bash
# Light load test (default)
./deploy/scripts/performance-test.sh

# Specific profile
./deploy/scripts/performance-test.sh --profile heavy

# Target specific deployment slot
./deploy/scripts/performance-test.sh --profile medium --slot blue
./deploy/scripts/performance-test.sh --profile medium --slot green

# Target load-balanced endpoint
./deploy/scripts/performance-test.sh --profile medium --slot lb
```

### Custom Configurations

```bash
# Custom RPS and duration
./deploy/scripts/performance-test.sh --rps 50 --duration 3m --slot blue

# Read-heavy workload
./deploy/scripts/performance-test.sh --rps 40 --duration 5m --read-ratio 0.95 --slot green

# Write-heavy workload (stress test writes)
./deploy/scripts/performance-test.sh --rps 20 --duration 2m --read-ratio 0.3 --slot blue
```

### Custom Target URL

```bash
# Test a remote server
./deploy/scripts/performance-test.sh --url https://api.example.com --profile medium

# Test specific port
TARGET_URL=http://localhost:8082 ./deploy/scripts/performance-test.sh --profile light
```

## Understanding the Results

### Sample Output

The following is historical output from a failing mixed workload that included
protected metrics and writes. Current public read-only profiles omit those
endpoints, and these authentication failures now make the command exit nonzero.

```
═══════════════════════════════════════════════════════════════
                    LOAD TEST RESULTS                           
═══════════════════════════════════════════════════════════════

Duration:        1m20s
Total Requests:  343
Successful:      122 (35.6%)
Failed:          221 (64.4%)
Requests/sec:    4.28

Response Times (ms):
  Average:  1
  p50:      1
  p95:      3
  p99:      6

Errors by Status Code:
  401: 16
  429: 205

Per-Endpoint Statistics:
─────────────────────────────────────────────────────────────
Endpoint             Count  Avg(ms)  p95(ms)      Min      Max
─────────────────────────────────────────────────────────────
health                  46        1        3        0        4
list_events             58        1        4        0       14
create_event            64        0        1        0        2
metrics                 49        1        4        0        5
─────────────────────────────────────────────────────────────
```

### Metrics Explained

- **Duration**: Total test duration including ramp-up/ramp-down
- **Total Requests**: All HTTP requests sent
- **Successful**: Requests matching their expected status (normally 2xx; the
  validation-only authentication preflight explicitly expects 400).
- **Failed**: Requests that do not match their expected status, including transport
  or response-body failures.
- **Requests/sec**: Average throughput
- **Response Times**: Latency percentiles (p50 = median, p95 = 95th percentile, p99 = 99th percentile)
- **Errors by Status Code**: Breakdown of HTTP error responses
- **Per-Endpoint Statistics**: Performance breakdown by endpoint

### Common Error Codes

- **401/403**: Unexpected authentication failure; the run fails
- **429 Too Many Requests**: The zero-budget run fails; reduce load or explicitly set an error budget
- **500 Internal Server Error**: Indicates server issues that need investigation
- **503 Service Unavailable**: Server may be overloaded

## Monitoring with Grafana

While running performance tests, view real-time metrics in Grafana:

1. Open Grafana: http://localhost:3000 (admin/admin)
2. Navigate to: **Dashboards** → **Togather** → **Togather Server Overview**
3. Observe:
   - Request rate increasing during ramp-up
   - Response time percentiles under load
   - Error rates
   - Database connection pool usage
   - Memory and goroutine counts

## Use Cases

### 1. Validate Monitoring Dashboard

```bash
# Generate varied traffic to see all dashboard panels populate
./deploy/scripts/performance-test.sh --profile medium --slot blue
```

Check Grafana to ensure all metrics are being collected and displayed correctly.

### 2. Test Blue-Green Deployment

```bash
# Test blue slot
./deploy/scripts/performance-test.sh --profile heavy --slot blue

# Test green slot
./deploy/scripts/performance-test.sh --profile heavy --slot green

# Compare results and Grafana metrics for both slots
```

### 3. Capacity Planning

```bash
# Gradually increase load to find capacity limits
./deploy/scripts/performance-test.sh --profile medium --slot blue
./deploy/scripts/performance-test.sh --profile heavy --slot blue
./deploy/scripts/performance-test.sh --profile stress --slot blue

# Analyze when response times degrade or errors increase
```

### 4. Rate Limiting Validation

```bash
# Generate enough traffic to trigger rate limits
./deploy/scripts/performance-test.sh --rps 100 --duration 1m --slot blue

# Check for 429 errors in results and Grafana
```

### 5. Stress Testing

```bash
# Long-running stress test
./deploy/scripts/performance-test.sh --profile stress --slot blue

# Monitor for memory leaks, connection pool exhaustion, etc.
```

## Advanced Usage

### Run from Go directly

```bash
# Build the binary
go build -o bin/loadtest ./cmd/loadtest

# Run with custom config
./bin/loadtest \
  --url http://localhost:8081 \
  --profile heavy \
  --rps 60 \
  --duration 10m
```

### Continuous Load Testing

```bash
# Run tests in a loop for extended soak testing
for i in {1..10}; do
  echo "Test iteration $i"
  ./deploy/scripts/performance-test.sh --profile medium --slot blue
  sleep 60
done
```

### Comparing Blue vs Green Performance

```bash
# Test both slots and compare
./deploy/scripts/performance-test.sh --profile heavy --slot blue > blue-results.txt
./deploy/scripts/performance-test.sh --profile heavy --slot green > green-results.txt
diff blue-results.txt green-results.txt
```

## Troubleshooting

### Server Not Reachable

```
✗ Error: Cannot reach server at http://localhost:8081
```

**Solution**: Start the server:
```bash
cd deploy/docker
docker compose -f /opt/togather/src/deploy/docker/docker-compose.yml -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml up -d
```

### All Requests Failing

Authentication failures are not a successful workload. Read-only mode needs no write
key. For explicit write mode, check credentials and preflight status before retrying.
Rate-limit responses fail the default zero-error-budget probe; lower the RPS or choose
an explicit error budget for a deliberate capacity test. Investigate all 5xx failures.

### Build Errors

If the load test binary fails to build:

```bash
cd /home/ryankelln/Documents/Projects/Art/togather/server
go mod tidy
go build -o bin/loadtest ./cmd/loadtest
```

## Staging Cleanup

Events ingested during a load test persist in the staging database. Clean them up after
each run to avoid data contamination.

A source-tagged key alone does not isolate the current generator: explicit payload
source URLs override its fallback source. Verify actual attribution before relying on
source-scoped cleanup (see
[Load Testing Operations](../operations/load-testing.md) for one-time setup):

```bash
server cleanup loadtest --env=staging --source-id=<source-uuid> --confirm
```

**Legacy cleanup** — for events ingested before source tagging (matches `example.com`
placeholder URLs injected by the fixture generator):

```bash
server cleanup loadtest --env=staging --legacy --confirm
```

Use `--dry-run` instead of `--confirm` to preview without deleting.

> **Note:** Fixture inputs are subject to the server's normal validation. A valid API
> key does not bypass reserved-domain checks or otherwise guarantee successful ingest.

## Performance Test Data

The load tester uses realistic test fixtures from `tests/fixtures/`:

- **10 Toronto venues** with real addresses and coordinates
- **10 sample organizers** with URLs
- **8 event sources** (Eventbrite, Meetup, Lu.ma, etc.)
- **6 event categories** (music, arts, tech, social, education, games)
- **Varied event types**: virtual, hybrid, in-person, recurring

All generated events use these fixtures to create realistic traffic patterns.

## Tips

1. **Start small**: Begin with `light` profile, then increase load
2. **Monitor first**: Have Grafana open before starting tests
3. **Compare slots**: Test blue and green independently to validate identical performance
4. **Check logs**: Review server logs during/after tests for errors
5. **Read-only first**: Explicitly select writes only after verifying ownership and cleanup
6. **Long-running tests**: Use `stress` or `peak` profiles to find memory leaks or resource exhaustion

---

**Last Updated:** 2026-11-12
