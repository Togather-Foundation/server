# Deployment Validation Tests

Automated tests for validating the complete deployment workflow using testcontainers.

## Overview

These tests validate the full deployment pipeline:
1. Docker image build
2. Database provisioning (PostgreSQL + PostGIS)
3. Migration execution
4. Health check validation
5. Performance benchmarking

## Prerequisites

- Docker daemon running
- `migrate` CLI tool installed (`go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`)
- `psql` client (for migration rollback tests)
- Go 1.25+

## Running Tests

### Run all deployment tests
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
      
      - name: Run deployment tests
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
- Verify migrate CLI is installed: `which migrate`
- Check migrations directory: `ls internal/storage/postgres/migrations/`
- Test migrations manually: `migrate -path internal/storage/postgres/migrations -database $DATABASE_URL up`

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

- [Deployment Guide](../../deploy/docs/README.md) (if exists)
- [Migration Guide](../../deploy/docs/migrations.md)
- [Rollback Guide](../../deploy/docs/rollback.md)
- [Health Check Script](../../deploy/scripts/health-check.sh)

---

**Last Updated:** 2026-01-28  
**Related Task:** T079 - Automated Deployment Validation Tests
