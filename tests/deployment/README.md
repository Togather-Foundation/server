# Deployment Validation Tests

Automated tests for validating the complete deployment workflow using testcontainers.

## Overview

This directory contains three types of deployment tests:

### Smoke Tests (Quick Validation)
Fast, lightweight tests that verify basic functionality of a deployed server:
- Health endpoint availability
- Version endpoint response
- Database connectivity
- Migration status
- Security headers

**Run with:** `./smoke_test.sh [BASE_URL]` (default: http://localhost:8080)  
**Duration:** <30 seconds

### Migration Lock Tests (Race Condition Prevention)
Tests the atomic locking mechanism that prevents concurrent migrations from corrupting the database:
- Single process lock acquisition
- Concurrent process blocking
- Lock cleanup on process exit
- Lock cleanup on signals (SIGTERM, SIGINT)
- Stale lock detection and cleanup
- Race condition prevention

**Run with:** `./test_migration_lock.sh`  
**Duration:** ~3-5 seconds

### Integration Tests (Full Validation)
Comprehensive tests using testcontainers that validate the complete deployment pipeline:
1. Docker image build
2. Database provisioning (PostgreSQL + PostGIS)
3. Migration execution
4. Health check validation
5. Performance benchmarking

**Run with:** `go test -v`  
**Duration:** 2-5 minutes

## Prerequisites

- Docker daemon running
- Go 1.24+
- Optional: `migrate` CLI tool (`go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`) - only needed for the RunMigrations subtest and TestMigrationRollback, which will be skipped if not installed
- Optional: `psql` client (for migration rollback tests)

**Note on Migrations:** The deployment tests use two migration systems:
1. **Application migrations** (`internal/storage/postgres/migrations/`) - Run via Go's `postgres.MigrateUp()`
2. **River job queue migrations** - Run via Go's `rivermigrate.Migrate()` API

Both are executed programmatically in the tests, so the `migrate` CLI is optional.

## Running Tests

### Smoke Tests (Deployed Server)
Quick validation of a running deployment:

```bash
# Test local deployment
./smoke_test.sh http://localhost:8080

# Test staging environment
./smoke_test.sh https://staging.togather.dev

# Test production environment
./smoke_test.sh https://api.togather.dev
```

**Use Cases:**
- Post-deployment validation
- CI/CD pipeline verification
- Production health monitoring
- Quick sanity checks

### Migration Lock Tests
Tests the atomic locking mechanism (server-ytc5 fix):

```bash
# Run all migration lock tests
./test_migration_lock.sh
```

**Test Scenarios:**
1. **Single acquisition** - Verify lock can be acquired
2. **Concurrent blocking** - Second process blocked when lock exists
3. **Exit cleanup** - Lock removed on normal process exit
4. **SIGTERM cleanup** - Lock removed when process receives SIGTERM
5. **SIGINT cleanup** - Lock removed when process receives SIGINT (Ctrl+C)
6. **Stale detection** - Stale locks (>30min) can be detected
7. **Race prevention** - Only one process acquires lock when competing

**Use Cases:**
- Verify race condition fix (server-ytc5)
- Regression testing for lock mechanism
- CI/CD validation of deployment safety

### Integration Tests (Full Pipeline)

#### Run all deployment tests
```bash
cd tests/deployment
go test -v
```

### Run specific test
```bash
go test -v -run TestDeploymentFullFlow
go test -v -run TestDeploymentPerformance
go test -v -run TestMigrationRollback
```

### Skip long-running tests
```bash
go test -v -short
```

## Test Coverage

### Smoke Tests
**Duration:** <30 seconds  
**Purpose:** Quick post-deployment validation

Tests:
- Health endpoint returns 200 OK with valid JSON
- Version endpoint returns version metadata
- Database connectivity verified via health check
- Migration status validated
- HTTP endpoint health check passes
- CORS headers present (if applicable)
- Security headers (CSP, X-Frame-Options) present
- Response time acceptable (<1000ms)

**Dependencies:** `curl`, `jq`

### Migration Lock Tests
**Duration:** ~3-5 seconds  
**Purpose:** Verify atomic locking prevents concurrent migration corruption

Tests:
- **test_single_acquisition** - Single process can acquire lock
- **test_concurrent_blocking** - Second process blocked by existing lock
- **test_lock_cleanup_on_exit** - Lock cleaned up on process exit
- **test_lock_cleanup_on_sigterm** - Lock cleaned up after SIGTERM
- **test_lock_cleanup_on_sigint** - Lock cleaned up after SIGINT
- **test_stale_lock_detection** - Stale locks can be detected (>30min)
- **test_race_condition** - Race prevention with concurrent acquisition

**Implementation:**
- Uses atomic `mkdir` for lock acquisition (POSIX guarantee)
- Tests actual `/tmp/togather-deploy-{env}.lock` directory creation
- Validates trap handlers cleanup on EXIT/INT/TERM signals
- Simulates stale lock scenarios with old timestamps

**Dependencies:** `bash`, `mkdir`, `rmdir`, `date`

### TestDeploymentFullFlow
**Duration:** ~2-4 minutes  
**Purpose:** End-to-end deployment validation

Tests:
- Docker image builds successfully
- PostgreSQL container provisions with PostGIS extensions
- Database migrations execute without errors
- Application container starts and passes health checks

### TestDeploymentPerformance
**Duration:** ~3-5 minutes  
**Purpose:** Performance benchmarking against <5min target

Measures:
- Docker build time
- Database provision time
- Migration execution time
- Application startup time
- **Total deployment time** (must be <5 minutes)

Reports timing breakdown and warns if performance target is missed.

### TestMigrationRollback
**Duration:** ~1-2 minutes  
**Purpose:** Verify migrations can be safely rolled back

Tests:
- Migrations apply successfully (`up`)
- Migrations can be rolled back (`down 1`)
- Migrations can be reapplied after rollback

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Deployment Tests

on: [push, pull_request]

jobs:
  deployment-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Install migrate CLI
        run: |
          go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
      
      - name: Run migration lock tests
        run: |
          cd tests/deployment
          ./test_migration_lock.sh
      
      - name: Run integration tests
        run: |
          cd tests/deployment
          go test -v -timeout 10m
```

### Makefile Integration
```makefile
test-deployment:
	cd tests/deployment && go test -v -timeout 10m

test-deployment-quick:
	cd tests/deployment && go test -v -short

test-migration-locks:
	cd tests/deployment && ./test_migration_lock.sh
```

## Troubleshooting

### "Docker image build failed"
- Ensure Docker daemon is running: `docker ps`
- Check Dockerfile exists: `ls deploy/docker/Dockerfile`
- Build manually to see full error: `docker build -f deploy/docker/Dockerfile .`

### "Failed to start PostgreSQL container"
- Check Docker has resources available: `docker system df`
- Verify network connectivity: `docker network ls`
- Check for port conflicts: `docker ps -a`

### "Migration failed"
- The RunMigrations subtest requires `migrate` CLI: `which migrate`
- If `migrate` is not installed, the subtest will be skipped (this is expected)
- Application and River migrations are still run programmatically in the StartApplicationAndValidateHealth test
- Check migrations directory exists: `ls internal/storage/postgres/migrations/`
- To run migrations manually: `migrate -path internal/storage/postgres/migrations -database $DATABASE_URL up`
- For River migrations: `make migrate-river` or use the Go API (`rivermigrate` package)

### "Health check request failed"
- Check application logs: Look for errors in test output
- Verify DATABASE_URL is correct
- Ensure container has network access
- Try accessing health endpoint manually after test: `curl http://localhost:<port>/health`

### Tests timeout
- Increase timeout: `go test -v -timeout 15m`
- Check system resources (CPU, memory, disk)
- Verify Docker pull speeds (first run downloads images)

## Test Output Example

```
=== RUN   TestDeploymentFullFlow
=== RUN   TestDeploymentFullFlow/BuildDockerImage
    deployment_test.go:38: Building Docker image: togather-server-test:deployment-test
    deployment_test.go:56: ✓ Docker image built successfully in 1m23s
=== RUN   TestDeploymentFullFlow/ProvisionDatabase
    deployment_test.go:73: Provisioning PostgreSQL database with PostGIS extensions
    deployment_test.go:88: ✓ PostgreSQL container started in 12s
=== RUN   TestDeploymentFullFlow/RunMigrations
    deployment_test.go:99: Running database migrations
    deployment_test.go:117: ✓ Migrations completed in 2.3s
=== RUN   TestDeploymentFullFlow/StartApplicationAndValidateHealth
    deployment_test.go:135: Starting application container: togather-server-test:deployment-test
    deployment_test.go:165: ✓ Application container started in 8s
    deployment_test.go:171: Health check URL: http://localhost:54321/health
    deployment_test.go:462: Health check response (200): {"status":"ok","checks":[...]}
--- PASS: TestDeploymentFullFlow (1m47s)
PASS
```

## Integration with Deploy Script

The deployment script (`deploy/scripts/deploy.sh`) follows the same workflow as these tests:

1. Build → `build_docker_image()`
2. Provision → handled by docker-compose
3. Migrate → `run_migrations()`
4. Health → `validate_health()`

These tests ensure that each step works correctly in isolation and as a complete flow.

## Related Documentation

- [Deployment Guide](../../docs/deploy/README.md) (if exists)
- [Migration Guide](../../docs/deploy/migrations.md)
- [Rollback Guide](../../docs/deploy/rollback.md)
- [Health Check Script](../../deploy/scripts/health-check.sh)

---

**Last Updated:** 2026-01-30  
**Related Tasks:** 
- T079 - Automated Deployment Validation Tests
- T080 - Deployment Smoke Tests
- server-ndu4 - Migration Lock Race Condition Tests
- server-ytc5 - Atomic Migration Locking Implementation

