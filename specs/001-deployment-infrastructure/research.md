# Research: Deployment Infrastructure

**Feature**: Deployment Infrastructure  
**Date**: 2026-01-28  
**Phase**: 0 (Research & Technical Decisions)

## Research Questions

This document resolves all technical unknowns from the specification and Technical Context. Each decision includes rationale and alternatives considered.

---

## 1. Deployment Orchestration Strategy

### Decision: Docker Compose v2 for MVP

**Rationale:**
- **Universal compatibility**: Works on any Linux host, cloud VM, bare metal, or developer laptop with Docker installed
- **Zero learning curve**: Industry-standard tool with extensive documentation and community support
- **Simple maintenance**: Single YAML file defines entire stack (app + database + networking)
- **Blue-green ready**: Native support for multiple service instances with health checks and traffic switching
- **No vendor lock-in**: Can migrate to cloud-native services (ECS, Cloud Run) without changing application code

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Kubernetes** | Massive complexity overhead for single-node architecture; requires cluster management, YAML sprawl, and steep learning curve; violates KISS principle |
| **Docker Swarm** | Limited community adoption, unclear future roadmap, fewer ecosystem tools than Compose |
| **Nomad (HashiCorp)** | Additional dependency for minimal benefit; Compose provides sufficient orchestration for single-node deployments |
| **Systemd units** | Manual blue-green orchestration, no built-in health checks, harder to replicate across environments |
| **Cloud-native only (ECS/Cloud Run)** | Locks community into cloud vendors, no local development/testing parity, breaks "works everywhere" requirement |

**Implementation Notes:**
- Use Docker Compose file version 3.8+ for modern features (healthchecks, secrets, deploy configs)
- Split configuration into multiple compose files for clarity: `base.yml` + `blue-green.yml` + `monitoring.yml`
- Use `docker compose` (v2 CLI) not legacy `docker-compose` (Python tool)

---

## 2. Blue-Green Deployment Implementation

### Decision: Nginx Reverse Proxy with Dynamic Upstream Switching

**Rationale:**
- **Zero downtime**: Traffic switches atomically after new version passes health checks
- **Instant rollback**: Switch back to previous upstream if issues detected
- **Simple architecture**: Single nginx container manages traffic routing, no complex service mesh
- **Health check integration**: Nginx actively probes backends, only routes to healthy instances
- **Observable**: Access logs show which version (blue/green) handled each request

**Implementation Strategy:**

```yaml
# docker-compose.blue-green.yml
services:
  app-blue:
    image: togather-server:${BLUE_VERSION}
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
  
  app-green:
    image: togather-server:${GREEN_VERSION}
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
  
  nginx:
    image: nginx:alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    ports:
      - "80:80"
    depends_on:
      app-blue:
        condition: service_healthy
      app-green:
        condition: service_healthy
```

**Deployment Flow:**
1. Current traffic goes to `app-blue` (version 1.0.0)
2. Deploy starts: Build `app-green` with version 1.1.0
3. Wait for `app-green` health checks to pass
4. Update nginx config to route traffic to `app-green`
5. Reload nginx (zero downtime: `nginx -s reload`)
6. Monitor `app-green` for 5 minutes
7. If stable: stop `app-blue`, tag `app-green` as current
8. If issues: revert nginx config to `app-blue` (instant rollback)

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Traefik** | More features than needed (auto-discovery, Let's Encrypt), nginx simpler for single-node use case |
| **HAProxy** | Similar capabilities to nginx but less familiar to community, fewer tutorials/examples |
| **Envoy** | Designed for service mesh (overkill), complex configuration, steep learning curve |
| **Application-level switching** | Requires app code changes, harder to debug, mixes deployment concerns with business logic |

---

## 3. Database Migration Strategy

### Decision: golang-migrate CLI with Pre-Migration Snapshots

**Rationale:**
- **Already in project dependencies**: No new tools to learn or install (`go.mod` line 9: `github.com/golang-migrate/migrate/v4`)
- **Proven reliability**: Used by thousands of Go projects, battle-tested against production workloads
- **Forward-only migrations**: Simpler mental model than bi-directional migrations (avoid complex rollback logic)
- **Transaction support**: Each migration runs in a transaction (automatic rollback on failure)
- **CLI + library**: Can run migrations from command line (deployment scripts) or Go code (application startup)

**Safety Mechanism: Automatic Snapshots**

```bash
# Pre-migration snapshot (deploy/scripts/snapshot-db.sh)
DB_NAME="togather_production"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
GIT_COMMIT=$(git rev-parse --short HEAD)
SNAPSHOT_FILE="${DB_NAME}_${TIMESTAMP}_${GIT_COMMIT}.sql.gz"

pg_dump -h $DB_HOST -U $DB_USER -d $DB_NAME \
  --format=plain \
  --no-owner \
  --no-acl \
  | gzip > /backups/$SNAPSHOT_FILE

# Retention: Delete snapshots older than 7 days
find /backups -name "*.sql.gz" -mtime +7 -delete
```

**Migration Best Practices (for developers):**
- **Backward compatible**: Old code must work against new schema during blue-green window
- **Additive changes**: Add columns as nullable, add indexes `CONCURRENTLY` (no table locks)
- **Two-phase breaking changes**: 
  - Phase 1: Add new column, populate from old column, dual-write to both
  - Phase 2: Remove old column after all code uses new column
- **Validation**: Test migrations against production-sized dataset using testcontainers

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Flyway** | Java-based (requires JVM), overkill for Go project, different ecosystem |
| **Liquibase** | XML configuration (verbose), Java dependency, designed for enterprise complexity |
| **Goose** | Similar to golang-migrate but less community adoption, fewer features (no version locking) |
| **Manual SQL scripts** | No version tracking, no state management, error-prone, no transaction handling |
| **ORM migrations (GORM, Ent)** | Project uses SQLc (explicit SQL), mixing migration strategies confuses developers |

---

## 4. Secrets Management (MVP)

### Decision: Environment Files (.env) with Clear Security Guidelines

**Rationale:**
- **Zero infrastructure**: No external secret services required (keeps zero-budget constraint)
- **Familiar format**: `.env` files are standard across languages and frameworks
- **Per-environment isolation**: Separate `.env.production`, `.env.staging` files prevent cross-contamination
- **Docker Compose integration**: Native support via `env_file` directive
- **Local development parity**: Developers use same mechanism as production (no production-only complexity)

**Security Implementation:**

```bash
# .gitignore (already present in most projects)
.env
.env.*
!.env.*.example

# deploy/config/environments/.env.production.example
# Database connection
DATABASE_URL=postgresql://togather:CHANGE_ME@postgres:5432/togather_production

# JWT signing key (generate with: openssl rand -base64 32)
JWT_SECRET=CHANGE_ME

# Admin API key (generate with: openssl rand -hex 32)
ADMIN_API_KEY=CHANGE_ME

# Deployment notifications
NTFY_TOPIC=togather-prod-alerts  # Optional: ntfy.sh topic for alerts
WEBHOOK_URL=                      # Optional: generic webhook for deployment events
```

**Operator Instructions (in quickstart.md):**
1. Copy `.env.production.example` to `.env.production` (never commit `.env.production`)
2. Replace all `CHANGE_ME` values with strong random secrets
3. Store `.env.production` in operator's password manager or encrypted vault
4. Transfer to server using secure channel (scp with SSH key, encrypted USB drive)
5. Set file permissions: `chmod 600 .env.production` (readable only by owner)

**Future-Proof Design (Phase 3):**

```go
// Secrets backend interface (application code)
type SecretsBackend interface {
    GetSecret(key string) (string, error)
    ListSecrets(prefix string) (map[string]string, error)
}

// MVP implementation
type EnvFileBackend struct {
    envFile string
}

// Phase 3 implementations (pluggable)
type AWSSecretsManagerBackend struct { /* ... */ }
type GCPSecretManagerBackend struct { /* ... */ }
type VaultBackend struct { /* ... */ }
```

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **AWS Secrets Manager** | Costs money ($0.40/secret/month), locks into AWS, breaks zero-budget constraint |
| **GCP Secret Manager** | Same issues as AWS, vendor lock-in |
| **HashiCorp Vault** | Requires running/maintaining Vault server, complex setup, overkill for single-node architecture |
| **Kubernetes Secrets** | Not using Kubernetes in MVP |
| **Git-crypt / SOPS** | Encrypted secrets in git adds complexity, key management challenges, accidental commits risk |

---

## 5. Monitoring and Observability (Phase 2)

### Decision: Prometheus + Grafana + Node Exporter Stack

**Rationale:**
- **Industry standard**: Prometheus is the de facto monitoring solution for containerized applications
- **Zero cost**: Open-source, self-hosted, no external dependencies or commercial licenses
- **Native Grafana integration**: Prometheus datasource is first-class citizen in Grafana
- **Efficient storage**: Prometheus uses time-series database optimized for metrics (15-day retention ~2-5GB)
- **Pull-based model**: Application exposes `/metrics` endpoint, Prometheus scrapes it (simple, debuggable)
- **Rich querying**: PromQL enables complex aggregations, rates, percentiles
- **Alert management**: Built-in Alertmanager routes alerts to ntfy.sh, webhooks, email

**Architecture:**

```yaml
# docker-compose.monitoring.yml
services:
  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./monitoring/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml
      - ./monitoring/prometheus/alerts.yml:/etc/prometheus/alerts.yml
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.retention.time=15d'
    ports:
      - "9090:9090"
  
  grafana:
    image: grafana/grafana-oss:latest
    volumes:
      - ./monitoring/grafana/dashboards:/etc/grafana/provisioning/dashboards
      - ./monitoring/grafana/datasources.yml:/etc/grafana/provisioning/datasources
      - grafana-data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD}
      - GF_AUTH_ANONYMOUS_ENABLED=false
    ports:
      - "3000:3000"
  
  node-exporter:
    image: prom/node-exporter:latest
    command:
      - '--path.rootfs=/host'
    volumes:
      - '/:/host:ro,rslave'
    ports:
      - "9100:9100"
  
  alertmanager:
    image: prom/alertmanager:latest
    volumes:
      - ./monitoring/alertmanager/alertmanager.yml:/etc/alertmanager/alertmanager.yml
    ports:
      - "9093:9093"
```

**Metrics to Expose (Application `/metrics` Endpoint):**

```go
// Use prometheus/client_golang (add to go.mod in Phase 2)
import "github.com/prometheus/client_golang/prometheus"

// HTTP metrics (via middleware)
httpRequestsTotal := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total HTTP requests",
    },
    []string{"method", "path", "status"},
)

httpDuration := prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name: "http_request_duration_seconds",
        Help: "HTTP request duration",
        Buckets: prometheus.DefBuckets,
    },
    []string{"method", "path"},
)

// Database metrics
dbConnectionsOpen := prometheus.NewGauge(
    prometheus.GaugeOpts{
        Name: "db_connections_open",
        Help: "Number of open database connections",
    },
)

// Application metrics
appVersion := prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "app_version_info",
        Help: "Application version information",
    },
    []string{"version", "commit", "build_date"},
)
```

**Pre-Built Grafana Dashboards** (JSON configs in repo):
1. **Deployment Dashboard**: Current version, deployment timeline, rollback history, health check status
2. **Application Health**: Request rate, error rate (%), response time (p50/p95/p99), database query performance
3. **Infrastructure**: CPU usage, memory usage, disk usage/I-O, network throughput
4. **Database**: Connection pool utilization, query rate/latency, cache hit ratio, table sizes

**Alert Rules** (Prometheus):
- **HighErrorRate**: `rate(http_requests_total{status=~"5.."}[5m]) > 0.05` (5% error rate for 2 minutes)
- **DatabaseDown**: `up{job="postgres"} == 0` (database unreachable for 1 minute)
- **DiskSpaceLow**: `node_disk_free_bytes / node_disk_total_bytes < 0.1` (90% disk usage for 5 minutes)
- **HighMemoryUsage**: `node_memory_Active_bytes / node_memory_MemTotal_bytes > 0.85` (85% memory for 10 minutes)

**ntfy.sh Integration** (Alertmanager webhook):

```yaml
# monitoring/alertmanager/alertmanager.yml
route:
  receiver: 'ntfy'
  group_by: ['alertname', 'severity']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h

receivers:
  - name: 'ntfy'
    webhook_configs:
      - url: 'https://ntfy.sh/${NTFY_TOPIC}'
        send_resolved: true
```

**Resource Overhead:**
- CPU: ~200-300 millicores (0.2-0.3 cores)
- Memory: ~500-800 MB
- Disk: ~2-5 GB for 15 days of metrics
- Network: Minimal (all localhost communication)

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Datadog / New Relic** | Commercial services ($15-100/month per host), breaks zero-budget constraint |
| **Elastic Stack (ELK)** | Heavy resource usage (Elasticsearch requires 2-4GB RAM), complex setup, overkill for metrics |
| **InfluxDB + Chronograf** | Less community adoption than Prometheus, fewer pre-built integrations, steeper learning curve |
| **Cloud provider monitoring (CloudWatch, Stackdriver)** | Vendor lock-in, costs money, no local development visibility |
| **VictoriaMetrics (MVP)** | More efficient than Prometheus but less familiar to community; defer to Phase 3 for long-term storage |

---

## 6. Deployment Locking (Prevent Concurrent Deploys)

### Decision: Filesystem Lock with Timeout

**Rationale:**
- **Simple and reliable**: No external dependencies (Redis, etcd), works everywhere
- **Atomic operations**: `flock` system call is atomic (race-condition safe)
- **Automatic cleanup**: Lock released when script exits (even if script crashes)
- **Timeout protection**: Lock expires after 30 minutes (prevents stuck locks from aborted deploys)
- **Clear error messages**: Operator immediately knows deployment is in progress

**Implementation:**

```bash
# deploy/scripts/deploy.sh
LOCK_FILE="/var/lock/togather-deploy.lock"
LOCK_TIMEOUT=1800  # 30 minutes

# Try to acquire lock (non-blocking)
exec 200>$LOCK_FILE
if ! flock -n 200; then
    echo "ERROR: Another deployment is in progress"
    echo "If deployment is stuck, remove lock: rm $LOCK_FILE"
    exit 1
fi

# Check lock age (timeout protection)
if [ -f "$LOCK_FILE" ]; then
    LOCK_AGE=$(($(date +%s) - $(stat -c %Y "$LOCK_FILE")))
    if [ $LOCK_AGE -gt $LOCK_TIMEOUT ]; then
        echo "WARNING: Stale lock detected (age: ${LOCK_AGE}s), removing..."
        rm -f $LOCK_FILE
    fi
fi

# Deployment logic here...
# Lock automatically released when script exits
```

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Redis lock (Redlock)** | Requires running Redis, adds infrastructure complexity, overkill for single-node |
| **etcd lock** | Requires etcd cluster, designed for distributed systems (not single-node) |
| **PostgreSQL advisory lock** | Ties deployment to database availability (chicken-and-egg problem if DB is being migrated) |
| **PID file** | Race conditions possible, manual cleanup required if script crashes |

---

## 7. Health Check Implementation

### Decision: Multi-Layer Health Checks (HTTP + Database + Critical Services)

**Rationale:**
- **Comprehensive validation**: Ensures all critical components work, not just HTTP server
- **Fast feedback**: Health check fails immediately if something wrong (no waiting for user traffic to detect issues)
- **Granular diagnostics**: Health endpoint reports which component failed (database? external API?)
- **Kubernetes-ready**: `/health` endpoint is standard for container orchestration (easy migration to K8s later)

**Health Check Endpoint** (`/health`):

```go
// internal/api/health.go
type HealthStatus struct {
    Status     string            `json:"status"`      // "healthy" | "degraded" | "unhealthy"
    Version    string            `json:"version"`     // Git commit SHA
    Checks     map[string]Check  `json:"checks"`
    Timestamp  time.Time         `json:"timestamp"`
}

type Check struct {
    Status  string `json:"status"`   // "pass" | "fail"
    Message string `json:"message"`
    Latency string `json:"latency"`  // e.g., "15ms"
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    
    health := HealthStatus{
        Status:    "healthy",
        Version:   GitCommit,
        Checks:    make(map[string]Check),
        Timestamp: time.Now(),
    }
    
    // Check 1: Database connectivity
    dbCheck := checkDatabase(ctx)
    health.Checks["database"] = dbCheck
    if dbCheck.Status == "fail" {
        health.Status = "unhealthy"
    }
    
    // Check 2: Database migrations (version matches code)
    migrationCheck := checkMigrations(ctx)
    health.Checks["migrations"] = migrationCheck
    if migrationCheck.Status == "fail" {
        health.Status = "unhealthy"
    }
    
    // Check 3: Critical services (job queue, cache if applicable)
    jobQueueCheck := checkJobQueue(ctx)
    health.Checks["job_queue"] = jobQueueCheck
    if jobQueueCheck.Status == "fail" {
        health.Status = "degraded"  // Non-critical, can still serve requests
    }
    
    // Return appropriate status code
    statusCode := http.StatusOK
    if health.Status == "unhealthy" {
        statusCode = http.StatusServiceUnavailable
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(health)
}
```

**Deployment Health Check Script** (`deploy/scripts/health-check.sh`):

```bash
#!/bin/bash
# Wait for application to become healthy after deployment

MAX_ATTEMPTS=30
SLEEP_INTERVAL=10
HEALTH_ENDPOINT="http://localhost:8080/health"

for i in $(seq 1 $MAX_ATTEMPTS); do
    echo "Health check attempt $i/$MAX_ATTEMPTS..."
    
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" $HEALTH_ENDPOINT)
    
    if [ "$RESPONSE" == "200" ]; then
        echo "✓ Application is healthy"
        
        # Validate response body
        HEALTH_JSON=$(curl -s $HEALTH_ENDPOINT)
        STATUS=$(echo $HEALTH_JSON | jq -r '.status')
        VERSION=$(echo $HEALTH_JSON | jq -r '.version')
        
        if [ "$STATUS" == "healthy" ]; then
            echo "✓ All health checks passed (version: $VERSION)"
            exit 0
        else
            echo "✗ Application reports degraded/unhealthy status"
            echo "$HEALTH_JSON" | jq '.'
            exit 1
        fi
    fi
    
    echo "Waiting ${SLEEP_INTERVAL}s before retry..."
    sleep $SLEEP_INTERVAL
done

echo "✗ Health check failed after $MAX_ATTEMPTS attempts"
exit 1
```

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **TCP port check only** | Doesn't validate application is actually working (port could be open but app crashed) |
| **HTTP 200 without body** | No diagnostic information when health check fails (blind debugging) |
| **External monitoring service** | Introduces external dependency, latency, potential false negatives from network issues |
| **Application startup timeout** | No active validation, just hope app works after X seconds |

---

## 8. Rollback Strategy

### Decision: Version Tagging + Docker Image Rollback + Database Snapshot Restore

**Rationale:**
- **Fast rollback**: Switch Docker image tag and restart (< 2 minutes)
- **Data safety**: Database snapshot available if migration needs reverting
- **Clear audit trail**: Git tags + Docker image tags show exact version deployed
- **No code changes**: Rollback is deployment operation, not code operation

**Rollback Script** (`deploy/scripts/rollback.sh`):

```bash
#!/bin/bash
# Rollback to previous deployment version

set -e

CURRENT_VERSION=$(docker inspect togather-server --format '{{.Config.Image}}' | cut -d: -f2)
PREVIOUS_VERSION=$(docker images togather-server --format "{{.Tag}}" | grep -v "$CURRENT_VERSION" | head -1)

if [ -z "$PREVIOUS_VERSION" ]; then
    echo "ERROR: No previous version found"
    echo "Available versions:"
    docker images togather-server --format "{{.Tag}}"
    exit 1
fi

echo "Rolling back from $CURRENT_VERSION to $PREVIOUS_VERSION"
read -p "Continue? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Rollback cancelled"
    exit 0
fi

# Switch Docker image (blue-green instant swap)
export ACTIVE_VERSION=$PREVIOUS_VERSION
docker-compose up -d app

# Wait for health checks
./health-check.sh

if [ $? -eq 0 ]; then
    echo "✓ Rollback successful (now running $PREVIOUS_VERSION)"
    
    # Tag as current
    docker tag togather-server:$PREVIOUS_VERSION togather-server:current
else
    echo "✗ Rollback failed - health checks did not pass"
    echo "Database snapshot available at: /backups/togather_*_${CURRENT_VERSION}.sql.gz"
    exit 1
fi
```

**Database Rollback** (manual operator intervention):

```bash
# If migration needs reverting (rare case)
SNAPSHOT_FILE=$(ls -t /backups/togather_*.sql.gz | head -1)
echo "Restoring database from: $SNAPSHOT_FILE"

gunzip -c $SNAPSHOT_FILE | psql -h localhost -U togather -d togather_production

# Verify migration version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
```

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Git revert commit** | Requires full rebuild and redeploy (slower), creates noisy git history, doesn't handle database rollback |
| **Multiple Docker Compose files** | Complex state management, harder to audit which version is active, file sprawl |
| **Automated database rollback** | Dangerous (data loss risk), migrations should be forward-only, manual confirmation required for safety |
| **Keep N previous versions** | Disk space waste, unclear which version to roll back to, rollback to arbitrary old version risky |

---

## 9. CI/CD Integration (Phase 2)

### Decision: GitHub Actions with Manual Approval for Production

**Rationale:**
- **Already using GitHub**: No additional service/account required
- **Native secrets management**: GitHub Actions secrets for credentials
- **Flexible workflows**: Can trigger on push, PR, schedule, or manual dispatch
- **Environment protection**: Branch protection rules + required approvals for production
- **Clear audit trail**: Every deployment logged in GitHub Actions UI

**Workflow Strategy:**

```yaml
# .github/workflows/deploy.yml
name: Deploy

on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to deploy'
        required: true
        type: choice
        options: [development, staging, production]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - run: make ci
  
  deploy-dev:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: development
    steps:
      - uses: actions/checkout@v4
      - name: Deploy to development
        run: |
          ./deploy/scripts/deploy.sh development
  
  deploy-staging:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - uses: actions/checkout@v4
      - name: Deploy to staging
        run: |
          ./deploy/scripts/deploy.sh staging
  
  deploy-production:
    needs: [test, deploy-staging]
    if: github.event_name == 'workflow_dispatch' && inputs.environment == 'production'
    runs-on: ubuntu-latest
    environment: production  # Requires manual approval
    steps:
      - uses: actions/checkout@v4
      - name: Deploy to production
        run: |
          ./deploy/scripts/deploy.sh production
      - name: Notify deployment
        if: always()
        run: |
          curl -d "Production deployment: ${{ job.status }}" https://ntfy.sh/${NTFY_TOPIC}
```

**Deployment Strategy:**
- **Automatic**: Development and staging deploy on every push to `main` (after tests pass)
- **Manual approval**: Production requires manual trigger + approval from maintainer
- **Fast feedback**: Developers see deployment status in PR checks

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **GitLab CI** | Project hosted on GitHub, no reason to use GitLab |
| **Jenkins** | Requires self-hosting Jenkins server, complex setup/maintenance, legacy technology |
| **CircleCI / Travis CI** | Commercial services (free tier limits), vendor lock-in, GitHub Actions more integrated |
| **Manual deployment only** | Error-prone, slow, no audit trail, manual work scales poorly as project grows |

---

## 10. Version Tagging and Build Metadata

### Decision: Git Commit SHA + Timestamp + Semantic Version

**Rationale:**
- **Traceable**: Every deployed version maps to exact git commit (can reproduce bugs, audit changes)
- **Sortable**: Timestamp enables chronological ordering
- **Meaningful**: Semantic version communicates intent (major/minor/patch changes)
- **Embedded in binary**: Version info available via `/version` endpoint and `--version` flag

**Implementation** (using existing Makefile LDFLAGS):

```makefile
# Makefile (already in project, lines 36-41)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'github.com/Togather-Foundation/server/cmd/server/cmd.Version=$(VERSION)' \
           -X 'github.com/Togather-Foundation/server/cmd/server/cmd.GitCommit=$(GIT_COMMIT)' \
           -X 'github.com/Togather-Foundation/server/cmd/server/cmd.BuildDate=$(BUILD_DATE)'
```

**Docker Image Tagging Strategy:**

```bash
# Build with multiple tags for flexibility
GIT_COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date +%Y%m%d_%H%M%S)
VERSION=$(git describe --tags --always)

docker build \
  --build-arg VERSION=$VERSION \
  --build-arg GIT_COMMIT=$GIT_COMMIT \
  --build-arg BUILD_DATE=$BUILD_DATE \
  -t togather-server:$GIT_COMMIT \
  -t togather-server:$VERSION \
  -t togather-server:latest \
  .
```

**Version Endpoint** (`/version`):

```go
type VersionInfo struct {
    Version   string `json:"version"`    // v1.2.3 or commit SHA
    GitCommit string `json:"git_commit"` // Short SHA
    BuildDate string `json:"build_date"` // RFC3339 timestamp
    GoVersion string `json:"go_version"` // Go compiler version
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
    info := VersionInfo{
        Version:   Version,
        GitCommit: GitCommit,
        BuildDate: BuildDate,
        GoVersion: runtime.Version(),
    }
    json.NewEncoder(w).Encode(info)
}
```

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Semantic version only** | Doesn't map to git commit (can't reproduce exact code state), loses traceability |
| **Timestamp only** | No semantic meaning (is this a bug fix or feature release?), hard to communicate to users |
| **Build number only** | Requires external state tracking (CI build counter), fragile, not reproducible locally |
| **No versioning** | Impossible to debug production issues, can't coordinate deployments, unprofessional |

---

## 11. Logging Strategy

### Decision: Structured JSON Logging with Zerolog (Already in Project)

**Rationale:**
- **Already in go.mod**: `github.com/rs/zerolog v1.33.0` (line 19)
- **Fast and efficient**: Zero-allocation logger, minimal performance overhead
- **Structured by default**: JSON output enables log aggregation and searching
- **Context propagation**: Request IDs, user IDs, trace IDs automatically included
- **Docker-friendly**: Logs to stdout/stderr (Docker captures automatically)

**Deployment-Specific Logging:**

```go
// Deployment event logging
log.Info().
    Str("event", "deployment.started").
    Str("version", newVersion).
    Str("environment", environment).
    Str("deployer", deployerEmail).
    Time("timestamp", time.Now()).
    Msg("Deployment started")

log.Info().
    Str("event", "deployment.completed").
    Str("version", newVersion).
    Dur("duration", deploymentDuration).
    Bool("migrations_run", true).
    Int("migration_count", migrationCount).
    Msg("Deployment completed successfully")

log.Error().
    Err(err).
    Str("event", "deployment.failed").
    Str("version", newVersion).
    Str("phase", "health_check").
    Msg("Deployment failed")
```

**Log Aggregation (Phase 3 - Grafana Loki):**
- Loki indexes only metadata (labels), stores logs as compressed chunks
- Query logs by deployment_id, version, environment
- Correlate logs with metrics and traces in Grafana
- 30-day retention

**Alternatives Considered:**

| Alternative | Rejected Because |
|-------------|------------------|
| **Plain text logs** | Hard to parse, search, aggregate; unstructured logs don't scale |
| **Logrus** | Slower than zerolog, larger memory footprint, less actively maintained |
| **Zap (uber-go/zap)** | Faster than zerolog but more complex API, diminishing returns for performance difference |
| **Elasticsearch + Filebeat** | Heavy resource usage (Elasticsearch requires 2-4GB RAM), complex setup, Loki more efficient |

---

## Summary of Key Decisions

| Component | MVP Choice | Phase 2+ Enhancement |
|-----------|------------|---------------------|
| **Orchestration** | Docker Compose v2 | Optional: ECS/Fargate (AWS), Cloud Run (GCP) |
| **Blue-Green** | Nginx reverse proxy | Same (works everywhere) |
| **Migrations** | golang-migrate CLI | Same (+ snapshot automation) |
| **Secrets** | Environment files (.env) | Cloud secret managers (AWS/GCP) |
| **Monitoring** | None (MVP) | Prometheus + Grafana + Alertmanager |
| **Locking** | Filesystem lock (flock) | Same (sufficient for single-node) |
| **Health Checks** | HTTP + DB + job queue | Same (+ custom checks) |
| **Rollback** | Docker image swap | Same (+ automated DB snapshot restore) |
| **CI/CD** | None (MVP) | GitHub Actions with manual approval |
| **Versioning** | Git commit + timestamp | Same (+ semantic versioning for releases) |
| **Logging** | Zerolog (structured JSON) | Same (+ Grafana Loki for aggregation) |

---

## Open Questions for Implementation

### Configuration Management
- **Q**: Should environment variables override deployment.yml values?
- **A**: Yes, follow precedence: CLI args > env vars > .env file > deployment.yml (most specific wins)

### Database Snapshot Storage
- **Q**: Local filesystem or S3-compatible storage for snapshots?
- **A**: Start with local filesystem (`/var/backups/togather`), add S3 option in Phase 2 (use rclone for cloud-agnostic uploads)

### Monitoring Activation
- **Q**: Deploy monitoring stack automatically or opt-in per environment?
- **A**: Opt-in via `ENABLE_MONITORING=true` in .env file (avoids resource waste in dev environments)

### SSL/TLS Certificates
- **Q**: Let's Encrypt automation or manual certificates?
- **A**: Defer to Phase 3; MVP assumes reverse proxy (Cloudflare, nginx on host) handles TLS termination

### Multi-Node Federation (Future)
- **Q**: How does deployment change when project adds federation support?
- **A**: Each node is independently deployed (current architecture supports this); federation is network-level concern, not deployment concern

---

## Next Steps (Phase 1)

1. Generate `data-model.md`: Define deployment entities (Deployment, Environment, Migration, Snapshot)
2. Generate `contracts/`: Bash script interfaces, health check JSON schema, deployment config YAML schema
3. Generate `quickstart.md`: Step-by-step operator guide for first deployment
4. Update agent context: Run `.specify/scripts/bash/update-agent-context.sh` to record technology choices
