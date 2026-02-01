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
   docker compose -f docker-compose.yml -f docker-compose.blue-green.yml up -d
   ```

2. **Monitoring stack** (optional but recommended):
   ```bash
   cd deploy/docker
   docker compose -f docker-compose.yml -f docker-compose.blue-green.yml --profile monitoring up -d
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

By default, profiles use an 80/20 read-to-write ratio (80% reads, 20% writes):

- **Read operations**: GET /health, /events, /places, /organizations, /metrics
- **Write operations**: POST /events (with realistic test fixtures)

You can customize this with `--read-ratio`:
```bash
# 95% reads, 5% writes
./deploy/scripts/performance-test.sh --profile medium --read-ratio 0.95 --slot blue
```

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
- **Successful**: Requests with 2xx status codes
- **Failed**: Requests with non-2xx status codes
- **Requests/sec**: Average throughput
- **Response Times**: Latency percentiles (p50 = median, p95 = 95th percentile, p99 = 99th percentile)
- **Errors by Status Code**: Breakdown of HTTP error responses
- **Per-Endpoint Statistics**: Performance breakdown by endpoint

### Common Error Codes

- **401 Unauthorized**: Normal for endpoints requiring authentication (POST /events)
- **429 Too Many Requests**: Rate limiting is working correctly
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
docker compose -f docker-compose.yml -f docker-compose.blue-green.yml up -d
```

### All Requests Failing

If you see 100% failures with 401 errors on POST requests, this is expected - the write operations require authentication. To focus on read-only testing:

```bash
./deploy/scripts/performance-test.sh --profile medium --read-ratio 1.0 --slot blue
```

### High Error Rates

- **401 errors**: Normal for authenticated endpoints
- **429 errors**: Rate limiting is working (adjust `--rps` lower)
- **500/503 errors**: Server may be overloaded (reduce load or investigate logs)

### Build Errors

If the load test binary fails to build:

```bash
cd /home/ryankelln/Documents/Projects/Art/togather/server
go mod tidy
go build -o bin/loadtest ./cmd/loadtest
```

## Performance Test Data

The load tester uses realistic test fixtures from `tests/testdata/`:

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
5. **Realistic ratios**: Use 80/20 or 90/10 read/write ratios for most tests
6. **Long-running tests**: Use `stress` or `peak` profiles to find memory leaks or resource exhaustion

## Next Steps

- Add authentication to test write endpoints properly
- Integrate with CI/CD for automated performance regression testing
- Create performance benchmarks to track over time
- Add more endpoint coverage (search, filters, pagination)
