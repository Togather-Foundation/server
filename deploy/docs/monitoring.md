# Monitoring Guide: Togather Server

**Target Audience**: Operators, DevOps engineers, and site reliability engineers  
**Time to Setup**: 10-15 minutes  
**Prerequisites**: Running Togather server with Docker Compose

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Configuration](#configuration)
4. [Dashboards](#dashboards)
5. [Metrics Reference](#metrics-reference)
6. [Alerting](#alerting)
7. [Troubleshooting](#troubleshooting)
8. [Best Practices](#best-practices)

---

## Overview

The Togather monitoring stack provides real-time observability into server performance, health status, and deployment metrics using industry-standard tools.

### What's Included

- **Prometheus**: Time-series metrics collection and storage
- **Grafana**: Visualization dashboards and alerting (Phase 2)
- **Pre-configured Dashboard**: Togather Server Overview dashboard with blue/green deployment tracking

### Architecture

```
┌─────────────┐     ┌─────────────┐
│ Blue Server │────▶│  Prometheus │
│   :8080     │     │   :9090     │
└─────────────┘     └──────┬──────┘
                           │
┌─────────────┐            │scrapes     ┌─────────────┐
│Green Server │────────────┘            │   Grafana   │
│   :8080     │                         │   :3000     │
└─────────────┘                         └─────────────┘
                                              │
                                              │visualizes
                                              ▼
                                        ┌─────────────┐
                                        │  Dashboard  │
                                        │  (Browser)  │
                                        └─────────────┘
```

### Resource Requirements

| Component  | CPU    | Memory | Disk  | Notes                          |
|------------|--------|--------|-------|--------------------------------|
| Prometheus | 0.5-1  | 1-2GB  | 10GB  | Grows with retention period    |
| Grafana    | 0.25   | 256MB  | 1GB   | Lightweight visualization      |
| **Total**  | 1-1.5  | 2-3GB  | 11GB  | Additional to server resources |

**Recommended for Production**:
- 8GB RAM minimum (4GB server + 3GB monitoring + 1GB overhead)
- 50GB disk space (database + snapshots + Prometheus retention)

---

## Quick Start

### Enable Monitoring

**Development (Local)**:

```bash
# Start server with monitoring stack
docker compose -f deploy/docker/docker-compose.yml \
  -f deploy/docker/docker-compose.blue-green.yml \
  --profile monitoring up -d

# Verify containers are running
docker ps | grep togather

# Expected output:
# togather-db          - PostgreSQL database
# togather-server-blue - Blue deployment slot
# togather-server-green - Green deployment slot
# togather-prometheus  - Metrics collection
# togather-grafana     - Visualization
```

**Production**:

Add to `deploy/config/environments/.env.production`:

```bash
# Enable monitoring stack
ENABLE_MONITORING=true

# Set Grafana admin password (generate: openssl rand -base64 16)
GRAFANA_PASSWORD=change_me_secure_password

# Optional: Active slot tracking
BLUE_ACTIVE_SLOT=true
GREEN_ACTIVE_SLOT=false
```

Then deploy:

```bash
./deploy/scripts/deploy.sh production
```

### Access Dashboards

**Grafana** (Web UI):
- URL: http://localhost:3000
- Username: `admin`
- Password: 
  - Development: `admin` (default)
  - Production: Value from `GRAFANA_PASSWORD` env var

**Prometheus** (Metrics & Targets):
- URL: http://localhost:9090
- No authentication by default
- Check targets: http://localhost:9090/targets

### View Dashboard

1. Log into Grafana at http://localhost:3000
2. Navigate to **Dashboards** → **Togather Server Overview**
3. Select slot filter: `blue`, `green`, or `All` (top left dropdown)
4. Adjust time range (top right)

---

## Configuration

### Environment Variables

| Variable              | Required | Default | Description                                    |
|-----------------------|----------|---------|------------------------------------------------|
| `ENABLE_MONITORING`   | No       | `false` | Enable Prometheus and Grafana containers       |
| `GRAFANA_PASSWORD`    | No       | `admin` | Grafana admin password                         |
| `BLUE_ACTIVE_SLOT`    | No       | `true`  | Mark blue slot as active in metrics            |
| `GREEN_ACTIVE_SLOT`   | No       | `false` | Mark green slot as active in metrics           |
| `PROMETHEUS_RETENTION`| No       | `15d`   | How long Prometheus stores metrics             |

### Prometheus Configuration

Location: `deploy/config/prometheus/prometheus.yml`

**Default Scrape Targets**:
```yaml
scrape_configs:
  - job_name: 'togather-blue'
    scrape_interval: 15s
    static_configs:
      - targets: ['togather-blue:8080']
        labels:
          slot: 'blue'

  - job_name: 'togather-green'
    scrape_interval: 15s
    static_configs:
      - targets: ['togather-green:8080']
        labels:
          slot: 'green'
```

**Adjusting Scrape Interval**:

```yaml
global:
  scrape_interval: 15s     # How often to scrape metrics
  evaluation_interval: 15s # How often to evaluate alert rules
```

- **Lower interval** (5s): More frequent data, higher resource usage
- **Higher interval** (30s-60s): Less frequent data, lower resource usage

**Changing Retention Period**:

Edit `deploy/docker/docker-compose.yml`:

```yaml
prometheus:
  command:
    - '--storage.tsdb.retention.time=30d'  # Keep 30 days instead of 15
```

### Custom Scrape Targets

To monitor additional services:

```yaml
scrape_configs:
  - job_name: 'external-api'
    scrape_interval: 30s
    static_configs:
      - targets: ['api.example.com:9090']
        labels:
          environment: 'production'
          service: 'external-api'
```

### Grafana Configuration

Location: `deploy/config/grafana/grafana.ini`

**Key Settings**:
- Anonymous access: Disabled by default
- Default organization: Togather
- Data source: Prometheus (auto-configured)

**Changing Admin Password** (after first login):

1. Log into Grafana (http://localhost:3000)
2. Navigate to **Configuration** → **Users** → **admin**
3. Click **Change Password**

---

## Dashboards

### Togather Server Overview

Pre-configured dashboard for monitoring blue/green deployment slots.

**Location**: `deploy/config/grafana/dashboards/json/togather-overview.json`

**Panels**:

1. **Server Status & Version**
   - Shows current version and git commit
   - Color-coded by slot (blue/green)
   - Displays active/inactive status

2. **Active Slot Indicator**
   - Shows which slot is currently active
   - Green background = receiving production traffic
   - Uses `togather_app_info{active_slot="true"}` metric

3. **Request Rate**
   - HTTP requests per second
   - Solid lines for each slot
   - Shows traffic distribution

4. **Request Latency**
   - p50, p95, p99 percentiles
   - Solid line (p50), dashed (p95), dotted (p99)
   - Identifies performance degradation

5. **Error Rate**
   - 4xx and 5xx errors per second
   - Tracks client vs. server errors
   - Alert threshold indicators

6. **In-Flight Requests**
   - Concurrent requests being processed
   - Helps identify traffic spikes
   - Normal value: 0-5 (1 is from Prometheus scraper)

7. **Database Connections**
   - Total, In Use, Idle connections
   - Tracks connection pool health
   - Alert on pool exhaustion

8. **Database Query Duration**
   - p50, p95, p99 query latency
   - Identifies slow queries
   - Correlates with request latency

9. **Go Memory Usage**
   - Heap and System memory
   - Tracks memory growth
   - Alert on memory leaks

10. **Goroutines**
    - Active goroutines per slot
    - Identifies goroutine leaks
    - Normal growth with traffic

### Dashboard Variables

- **`$slot`**: Filter by deployment slot
  - Options: `blue`, `green`, `All`
  - Default: `All`

- **Time Range**: Adjustable via top-right controls
  - Last 5m / 15m / 1h / 6h / 24h / 7d
  - Custom range picker

### Creating Custom Dashboards

1. **Via Grafana UI**:
   - Navigate to **+ Create** → **Dashboard**
   - Add panels with PromQL queries
   - Save dashboard
   - Export JSON: **Dashboard Settings** → **JSON Model** → Copy

2. **Save to Repository**:
   ```bash
   # Export dashboard from Grafana API
   curl -u admin:admin http://localhost:3000/api/dashboards/uid/togather-overview \
     | jq '.dashboard' \
     > deploy/config/grafana/dashboards/json/my-custom-dashboard.json
   ```

3. **Auto-load on Startup**:
   - Place JSON in `deploy/config/grafana/dashboards/json/`
   - Restart Grafana container
   - Dashboard appears under **Dashboards**

### Exporting/Importing Dashboards

**Export**:
```bash
# Via API
curl -u admin:$GRAFANA_PASSWORD http://localhost:3000/api/dashboards/uid/DASHBOARD_UID \
  | jq '.dashboard' > dashboard-backup.json

# Via UI: Dashboard Settings → JSON Model → Copy
```

**Import**:
```bash
# Via API
curl -X POST -H "Content-Type: application/json" \
  -d @dashboard-backup.json \
  http://admin:$GRAFANA_PASSWORD@localhost:3000/api/dashboards/db

# Via UI: + Create → Import → Upload JSON file
```

---

## Metrics Reference

### Application Metrics

#### `togather_app_info{version, git_commit, active_slot, slot}`
**Type**: Gauge (always 1)  
**Description**: Application version information  
**Labels**:
- `version`: Semantic version or git commit SHA
- `git_commit`: Full git commit hash
- `active_slot`: `"true"` if receiving production traffic, `"false"` otherwise
- `slot`: Deployment slot (`blue` or `green`)

**Example Query**:
```promql
# Show active slot
togather_app_info{active_slot="true"}

# List all versions
togather_app_info
```

---

### HTTP Metrics

#### `togather_http_requests_total{method, path, status, slot}`
**Type**: Counter  
**Description**: Total number of HTTP requests  
**Labels**:
- `method`: HTTP method (GET, POST, PUT, DELETE)
- `path`: Request path template (e.g., `/api/v1/events`)
- `status`: HTTP status code (200, 404, 500, etc.)
- `slot`: Deployment slot

**Example Queries**:
```promql
# Request rate (requests per second)
rate(togather_http_requests_total[5m])

# Error rate (4xx + 5xx)
rate(togather_http_requests_total{status=~"[45].."}[5m])

# Success rate percentage
100 * rate(togather_http_requests_total{status=~"2.."}[5m]) 
  / rate(togather_http_requests_total[5m])
```

#### `togather_http_request_duration_seconds{method, path, slot}`
**Type**: Histogram  
**Description**: HTTP request latency distribution  
**Buckets**: 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5 seconds

**Example Queries**:
```promql
# p50 latency
histogram_quantile(0.50, 
  rate(togather_http_request_duration_seconds_bucket[5m]))

# p95 latency
histogram_quantile(0.95, 
  rate(togather_http_request_duration_seconds_bucket[5m]))

# p99 latency
histogram_quantile(0.99, 
  rate(togather_http_request_duration_seconds_bucket[5m]))
```

#### `togather_http_requests_in_flight{slot}`
**Type**: Gauge  
**Description**: Number of HTTP requests currently being processed  
**Normal Value**: 0-5 (1 is from Prometheus scraper)

**Example Queries**:
```promql
# Current in-flight requests
togather_http_requests_in_flight

# Max in-flight over time
max_over_time(togather_http_requests_in_flight[5m])
```

#### `togather_http_response_size_bytes{method, path, slot}`
**Type**: Histogram  
**Description**: HTTP response body size distribution

---

### Health Check Metrics

#### `togather_health_status{slot}`
**Type**: Gauge  
**Description**: Overall server health status  
**Values**:
- `0` = Unhealthy (one or more checks failed)
- `1` = Degraded (one or more checks have warnings)
- `2` = Healthy (all checks passed)

**Example Queries**:
```promql
# Current health status
togather_health_status

# Alert if unhealthy
togather_health_status < 2
```

#### `togather_health_check_status{check, slot}`
**Type**: Gauge  
**Description**: Individual health check status  
**Values**:
- `0` = Fail
- `1` = Warn
- `2` = Pass

**Check Names**:
- `database`: PostgreSQL connection
- `migrations`: Schema version verification
- `job_queue`: River job queue operational status
- `jsonld_contexts`: JSON-LD context file availability

**Example Queries**:
```promql
# Show failed checks
togather_health_check_status{status="0"}

# Database health
togather_health_check_status{check="database"}
```

#### `togather_health_check_latency_ms{check, slot}`
**Type**: Gauge  
**Description**: Health check execution latency in milliseconds  
**Note**: Only reported for checks that have measurable latency

**Example Queries**:
```promql
# Database check latency
togather_health_check_latency_ms{check="database"}

# Slowest health check
max(togather_health_check_latency_ms)
```

---

### Database Metrics

#### `togather_db_connections_open{slot}`
**Type**: Gauge  
**Description**: Total number of open database connections (in use + idle)

#### `togather_db_connections_in_use{slot}`
**Type**: Gauge  
**Description**: Number of connections currently executing queries

#### `togather_db_connections_idle{slot}`
**Type**: Gauge  
**Description**: Number of idle connections in the pool

#### `togather_db_connections_max_open{slot}`
**Type**: Gauge  
**Description**: Maximum allowed open connections (from pool config)

**Example Queries**:
```promql
# Connection pool utilization percentage
100 * togather_db_connections_in_use / togather_db_connections_max_open

# Alert on pool exhaustion (>80% in use)
togather_db_connections_in_use / togather_db_connections_max_open > 0.8
```

---

### Go Runtime Metrics

Prometheus automatically collects Go runtime metrics with the `go_` prefix:

#### `go_goroutines{slot}`
**Type**: Gauge  
**Description**: Number of active goroutines

**Example Queries**:
```promql
# Current goroutines
go_goroutines

# Goroutine growth rate
deriv(go_goroutines[5m])
```

#### `go_memstats_heap_alloc_bytes{slot}`
**Type**: Gauge  
**Description**: Bytes of allocated heap objects

#### `go_memstats_sys_bytes{slot}`
**Type**: Gauge  
**Description**: Total bytes of memory obtained from the OS

**Example Queries**:
```promql
# Heap memory in MB
go_memstats_heap_alloc_bytes / 1024 / 1024

# System memory in MB
go_memstats_sys_bytes / 1024 / 1024

# Memory growth rate
deriv(go_memstats_heap_alloc_bytes[5m])
```

---

## Alerting

> **Note**: Alerting is planned for Phase 2. This section provides guidelines for when it's implemented.

### Alert Rules (Planned)

Location: `deploy/config/prometheus/alerts.yml`

**Example Rules**:

```yaml
groups:
  - name: togather-server
    interval: 30s
    rules:
      # High error rate
      - alert: HighErrorRate
        expr: |
          rate(togather_http_requests_total{status=~"5.."}[5m]) > 10
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High 5xx error rate on {{ $labels.slot }}"
          description: "Error rate is {{ $value | humanize }} req/s"

      # High latency
      - alert: HighLatency
        expr: |
          histogram_quantile(0.95,
            rate(togather_http_request_duration_seconds_bucket[5m])) > 1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High p95 latency on {{ $labels.slot }}"
          description: "p95 latency is {{ $value | humanize }}s"

      # Database connection pool exhaustion
      - alert: DatabasePoolExhausted
        expr: |
          togather_db_connections_in_use / togather_db_connections_max_open > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Database connection pool exhausted on {{ $labels.slot }}"
          description: "Pool utilization is {{ $value | humanizePercentage }}"

      # Service unhealthy
      - alert: ServiceUnhealthy
        expr: togather_health_status < 2
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Service unhealthy on {{ $labels.slot }}"
          description: "Health status: {{ $value }} (0=unhealthy, 1=degraded, 2=healthy)"

      # Goroutine leak
      - alert: GoroutineLeak
        expr: |
          deriv(go_goroutines[10m]) > 100
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "Potential goroutine leak on {{ $labels.slot }}"
          description: "Goroutines growing at {{ $value | humanize }} per second"
```

### Notification Channels (Planned)

**ntfy.sh** (Simple, no signup):
```yaml
alertmanager:
  route:
    receiver: 'ntfy'
  receivers:
    - name: 'ntfy'
      webhook_configs:
        - url: 'https://ntfy.sh/togather-prod-alerts'
          send_resolved: true
```

**Slack**:
```yaml
receivers:
  - name: 'slack'
    slack_configs:
      - api_url: '$SLACK_WEBHOOK_URL'
        channel: '#alerts'
```

---

## Troubleshooting

### Prometheus Not Scraping Metrics

**Symptom**: Dashboard shows "No data" or empty graphs

**Solutions**:

1. **Check Prometheus targets**:
   ```bash
   # Open Prometheus UI
   open http://localhost:9090/targets
   
   # Look for targets with status "DOWN"
   ```

2. **Verify server is exposing metrics**:
   ```bash
   # Blue slot
   curl http://localhost:8081/metrics | head -20
   
   # Green slot
   curl http://localhost:8082/metrics | head -20
   
   # Should show Prometheus-format metrics
   ```

3. **Check Prometheus logs**:
   ```bash
   docker logs togather-prometheus
   
   # Look for errors like:
   # - "connection refused"
   # - "context deadline exceeded"
   # - "no such host"
   ```

4. **Verify network connectivity**:
   ```bash
   # From Prometheus container to server
   docker exec togather-prometheus wget -O- http://togather-blue:8080/metrics
   ```

---

### Grafana Shows "Data source connected, but no labels received"

**Symptom**: Grafana can connect to Prometheus, but dashboards are empty

**Solutions**:

1. **Test Prometheus data source**:
   - Navigate to **Configuration** → **Data Sources** → **Prometheus**
   - Click **Test**
   - Should show "Data source is working"

2. **Check if metrics exist in Prometheus**:
   ```bash
   # Query Prometheus directly
   curl -s 'http://localhost:9090/api/v1/query?query=togather_http_requests_total' | jq
   
   # Should return result.length > 0
   ```

3. **Verify scrape interval hasn't passed**:
   - Wait 15-30 seconds after starting containers
   - Prometheus scrapes every 15s by default

4. **Check Grafana logs**:
   ```bash
   docker logs togather-grafana
   ```

---

### Dashboard Variables Not Working

**Symptom**: `$slot` dropdown is empty or shows "No options found"

**Solutions**:

1. **Check variable query**:
   - **Dashboard Settings** → **Variables** → **slot**
   - Query should be: `label_values(togather_app_info, slot)`

2. **Verify metric labels exist**:
   ```bash
   curl -s 'http://localhost:9090/api/v1/query?query=togather_app_info' \
     | jq '.data.result[].metric.slot'
   ```

3. **Refresh dashboard**:
   - Click **🔄** icon next to time range picker

---

### High Memory Usage

**Symptom**: Prometheus container using excessive memory (>4GB)

**Solutions**:

1. **Reduce retention period**:
   ```yaml
   # deploy/docker/docker-compose.yml
   prometheus:
     command:
       - '--storage.tsdb.retention.time=7d'  # Reduce from 15d
   ```

2. **Decrease scrape frequency**:
   ```yaml
   # deploy/config/prometheus/prometheus.yml
   global:
     scrape_interval: 30s  # Increase from 15s
   ```

3. **Check disk usage**:
   ```bash
   docker exec togather-prometheus df -h /prometheus
   ```

---

### "No data points" on Specific Panels

**Symptom**: Some panels work, others show "No data points"

**Solutions**:

1. **Check panel query**:
   - Edit panel → **Query** tab
   - Click **Query Inspector** → **Refresh**
   - Look for PromQL syntax errors

2. **Verify metric exists**:
   ```bash
   # Replace with metric from broken panel
   curl -s 'http://localhost:9090/api/v1/query?query=METRIC_NAME' | jq
   ```

3. **Check time range**:
   - Ensure time range includes data
   - Try "Last 5 minutes" to see most recent data

---

### Health Check Metrics Not Updating

**Symptom**: `togather_health_status` stuck at old value

**Solutions**:

1. **Trigger health check**:
   ```bash
   # Health checks update on every /health request
   curl http://localhost:8081/health
   curl http://localhost:8082/health
   ```

2. **Check health handler logs**:
   ```bash
   docker logs togather-server-blue | grep health
   docker logs togather-server-green | grep health
   ```

3. **Verify metric exists**:
   ```bash
   curl http://localhost:8081/metrics | grep togather_health
   ```

---

## Best Practices

### Monitoring Strategy

1. **Start with Overview Dashboard**
   - Get familiar with normal baseline metrics
   - Identify patterns (daily traffic cycles, etc.)
   - Screenshot baseline for comparison

2. **Set Up Alerting** (Phase 2)
   - Start with critical alerts only (health, errors)
   - Gradually add warning alerts (latency, resources)
   - Avoid alert fatigue (too many non-actionable alerts)

3. **Regular Reviews**
   - Weekly: Review error rates, latency trends
   - Monthly: Check resource growth, plan capacity
   - After incidents: Update dashboards/alerts based on learnings

### Dashboard Maintenance

1. **Keep Dashboards in Git**
   - Export after UI changes:
     ```bash
     curl -u admin:admin http://localhost:3000/api/dashboards/uid/togather-overview \
       | jq '.dashboard' \
       > deploy/config/grafana/dashboards/json/togather-overview.json
     ```

2. **Follow Design Guidelines**
   - See `deploy/docs/grafana-dashboard-guidelines.md`
   - Use consistent colors for blue/green slots
   - Apply line style differentiation (solid/dashed/dotted)

3. **Test Dashboard Changes**
   - Select "All" in `$slot` dropdown
   - Verify both slots are visible and distinguishable
   - Check grayscale/colorblind rendering

### Performance Optimization

1. **Optimize PromQL Queries**
   - Use recording rules for expensive queries
   - Aggregate before querying: `sum(rate(...)) by (slot)`
   - Avoid `rate()` over long time windows in dashboards

2. **Manage Data Retention**
   - Balance retention vs. disk space
   - Archive old data to object storage if needed
   - Clean up unused metrics

3. **Resource Limits**
   ```yaml
   # deploy/docker/docker-compose.yml
   prometheus:
     deploy:
       resources:
         limits:
           cpus: '2'
           memory: 2G
   ```

### Security

1. **Change Default Credentials**
   - Set `GRAFANA_PASSWORD` in production
   - Use strong, randomly generated passwords
   - Store secrets in secrets manager (Phase 2)

2. **Network Isolation**
   - Prometheus/Grafana should NOT be exposed to public internet
   - Use VPN or SSH tunnel to access dashboards
   - Add authentication proxy if external access needed

3. **Access Control** (Phase 2)
   - Create separate Grafana users for operators
   - Use viewer role for read-only access
   - Enable audit logging

---

## Next Steps

### Phase 2 Enhancements

- [ ] **Alerting**: Configure Prometheus Alertmanager with notification channels
- [ ] **Log Aggregation**: Integrate Loki for centralized logging
- [ ] **Distributed Tracing**: Add OpenTelemetry for request tracing
- [ ] **Custom Metrics**: Add business metrics (events created, organizations, etc.)
- [ ] **SLO Dashboards**: Track service level objectives and error budgets

### Learning Resources

- **Prometheus Documentation**: https://prometheus.io/docs/
- **PromQL Guide**: https://prometheus.io/docs/prometheus/latest/querying/basics/
- **Grafana Documentation**: https://grafana.com/docs/
- **Togather Grafana Guidelines**: `deploy/docs/grafana-dashboard-guidelines.md`

---

## Getting Help

- **Dashboard Issues**: Check `deploy/docs/grafana-dashboard-guidelines.md`
- **Deployment Issues**: See `deploy/docs/troubleshooting.md`
- **Metric Questions**: View `/metrics` endpoint directly: `curl http://localhost:8081/metrics`
- **Report Bugs**: https://github.com/Togather-Foundation/server/issues

---

**Version**: 1.0.0  
**Last Updated**: 2026-01-30  
**Maintained By**: Togather DevOps Team
