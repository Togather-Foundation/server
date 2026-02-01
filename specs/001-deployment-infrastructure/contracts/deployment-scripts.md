# Deployment Scripts CLI Interface

**Version**: 1.0.0  
**Status**: MVP Contract

This document defines the command-line interface contracts for deployment scripts. All scripts follow consistent patterns for arguments, exit codes, and output formats.

---

## Common Conventions

### Exit Codes

All deployment scripts use standard exit codes:

| Code | Meaning | When Used |
|------|---------|-----------|
| `0` | Success | Operation completed successfully |
| `1` | General failure | Unspecified error occurred |
| `2` | Invalid arguments | Missing required argument or invalid value |
| `3` | Lock conflict | Another deployment is in progress |
| `4` | Health check failure | Deployment completed but health checks failed |
| `5` | Migration failure | Database migration failed |
| `10` | Configuration error | Invalid or missing configuration file |

### Output Format

- **stdout**: Operational logs, progress messages (human-readable)
- **stderr**: Error messages, warnings (human-readable)
- **Structured logs**: JSON lines to `/var/log/togather/deployments/<deployment-id>.json`

### Environment Variables

All scripts respect these environment variables:

- `ENVIRONMENT`: Target environment (`development`, `staging`, `production`) - **REQUIRED**
- `CONFIG_DIR`: Path to deployment config directory (default: `./deploy/config`)
- `LOG_LEVEL`: Logging verbosity (`debug`, `info`, `warn`, `error`) (default: `info`)
- `DRY_RUN`: If set to `true`, perform validation only without making changes (default: `false`)

---

## 1. deploy.sh

**Purpose**: Main deployment orchestrator - builds, deploys, and validates a new version.

### Synopsis

```bash
./deploy/scripts/deploy.sh [OPTIONS] <environment>
```

### Arguments

- `<environment>` (required): Target environment (`development`, `staging`, `production`)

### Options

- `--version <git-ref>`: Git commit SHA, tag, or branch to deploy (default: current HEAD)
- `--skip-migrations`: Skip database migrations (dangerous, only for rollback scenarios)
- `--skip-health-check`: Skip post-deployment health validation (not recommended)
- `--force`: Override deployment lock (use when previous deployment crashed)
- `--config <path>`: Path to deployment configuration file (default: `deploy/config/deployment.yml`)
- `--dry-run`: Validate configuration and build artifact without deploying
- `--help`: Display help message

### Examples

```bash
# Deploy current HEAD to production
./deploy/scripts/deploy.sh production

# Deploy specific Git tag to staging
./deploy/scripts/deploy.sh --version v1.2.3 staging

# Dry-run production deployment (validation only)
./deploy/scripts/deploy.sh --dry-run production

# Force deployment (override stale lock)
./deploy/scripts/deploy.sh --force production
```

### Output

**Success** (exit code 0):
```
==> Deploying togather-server to production
✓ Lock acquired
✓ Configuration validated
✓ Building Docker image (version: abc1234)
✓ Database snapshot created: togather_production_20260128_103014_abc1234.sql.gz
✓ Running 3 database migrations
✓ Deploying blue-green (blue -> green)
✓ Health checks passed (5/5)
✓ Traffic switched to green
✓ Deployment completed successfully

Deployment ID: dep_01JBQR2KXYZ9876543210
Version: abc1234
Duration: 4m 32s
```

**Failure** (exit code 4 - health check failure):
```
==> Deploying togather-server to production
✓ Lock acquired
✓ Configuration validated
✓ Building Docker image (version: abc1234)
✓ Database snapshot created
✓ Running 3 database migrations
✓ Deploying blue-green (blue -> green)
✗ Health check failed: database migrations check failed
  Expected version: 20260128_001
  Actual version: 20260127_001

==> Rolling back deployment
✓ Traffic switched back to blue
✓ Green container stopped
✗ Deployment failed (rolled back)

See logs: /var/log/togather/deployments/dep_01JBQR2KXYZ9876543210.json
```

### Structured Log Output

```json
{
  "event": "deployment.completed",
  "deployment_id": "dep_01JBQR2KXYZ9876543210",
  "environment": "production",
  "version": "abc1234",
  "deployed_at": "2026-01-28T10:34:32Z",
  "deployed_by": "operator@example.com",
  "duration_seconds": 272,
  "migrations_run": 3,
  "health_checks": {
    "passed": 5,
    "failed": 0
  },
  "status": "success"
}
```

---

## 2. rollback.sh

**Purpose**: Revert to the previous deployed version.

### Synopsis

```bash
./deploy/scripts/rollback.sh [OPTIONS] <environment>
```

### Arguments

- `<environment>` (required): Target environment

### Options

- `--version <git-ref>`: Specific version to roll back to (default: previous deployment)
- `--force`: Skip confirmation prompt (dangerous)
- `--restore-db`: Also restore database from snapshot (requires manual confirmation)
- `--help`: Display help message

### Examples

```bash
# Rollback to previous version (interactive confirmation)
./deploy/scripts/rollback.sh production

# Rollback to specific version
./deploy/scripts/rollback.sh --version xyz5678 production

# Rollback without confirmation (automation)
./deploy/scripts/rollback.sh --force production
```

### Output

**Success** (exit code 0):
```
==> Rolling back production deployment
Current version: abc1234
Previous version: xyz5678

WARNING: This will switch traffic back to xyz5678
Continue? (y/n) y

✓ Switching traffic (green -> blue)
✓ Health checks passed (5/5)
✓ Rollback completed successfully

Version now: xyz5678
```

**No Previous Version** (exit code 1):
```
==> Rolling back production deployment
✗ No previous version found

Available versions:
  abc1234 (current)

Cannot rollback - this is the only deployment.
```

---

## 3. health-check.sh

**Purpose**: Validate application health after deployment.

### Synopsis

```bash
./deploy/scripts/health-check.sh [OPTIONS] <endpoint>
```

### Arguments

- `<endpoint>` (required): Health check URL (e.g., `http://localhost:8080/health`)

### Options

- `--max-attempts <n>`: Maximum health check attempts (default: 30)
- `--interval <seconds>`: Seconds between attempts (default: 10)
- `--timeout <seconds>`: HTTP request timeout (default: 5)
- `--expected-version <git-sha>`: Expected version in response (optional validation)
- `--json`: Output results as JSON (for automation)
- `--help`: Display help message

### Examples

```bash
# Wait for health checks to pass (default 30 attempts, 10s interval)
./health-check.sh http://localhost:8080/health

# Quick health check (3 attempts, 5s interval)
./health-check.sh --max-attempts 3 --interval 5 http://localhost:8080/health

# Validate specific version deployed
./health-check.sh --expected-version abc1234 http://localhost:8080/health

# JSON output (for CI/CD)
./health-check.sh --json http://localhost:8080/health
```

### Output

**Success** (exit code 0):
```
Health check attempt 1/30...
✓ Application is healthy
✓ All health checks passed (version: abc1234)

Checks:
  database: pass (8ms)
  migrations: pass (5ms)
  http_endpoint: pass (2ms)
  job_queue: pass (12ms)
```

**Failure** (exit code 4):
```
Health check attempt 1/30...
Waiting 10s before retry...
Health check attempt 2/30...
Waiting 10s before retry...
...
Health check attempt 30/30...
✗ Health check failed after 30 attempts

Last response (HTTP 503):
{
  "status": "unhealthy",
  "checks": {
    "database": {"status": "fail", "message": "Connection timeout"}
  }
}
```

### JSON Output (`--json` flag)

```json
{
  "success": true,
  "attempts": 1,
  "duration_seconds": 2,
  "health": {
    "status": "healthy",
    "version": "abc1234",
    "checks": {
      "database": {"status": "pass", "latency": "8ms"},
      "migrations": {"status": "pass", "latency": "5ms"}
    }
  }
}
```

---

## 4. snapshot-db.sh

> **⚠️ DEPRECATED**: This script is deprecated. Use the `server snapshot` CLI command instead.
> 
> - `server snapshot create --reason "pre-deploy backup"`
> - `server snapshot list`
> - `server snapshot cleanup --retention-days 7`
>
> See `server snapshot --help` for full documentation.

**Purpose**: Create database snapshot before migrations (wrapper for backward compatibility).

### Synopsis

```bash
./deploy/scripts/snapshot-db.sh [OPTIONS]
```

**Note**: This script now forwards calls to `server snapshot` CLI. Update your scripts to use the CLI directly.

### Options

- `--database <name>`: Database name (default: from `DATABASE_URL`)
- `--output-dir <path>`: Snapshot storage directory (default: `/var/backups/togather`)
- `--compression <type>`: Compression algorithm (`gzip`, `zstd`, `none`) (default: `gzip`)
- `--retention-days <n>`: Days to keep snapshots (default: 7)
- `--help`: Display help message

### Examples

```bash
# Create snapshot with defaults
./snapshot-db.sh

# Create snapshot with custom output directory
./snapshot-db.sh --output-dir /mnt/backups

# Create uncompressed snapshot (faster, larger)
./snapshot-db.sh --compression none
```

### Output

**Success** (exit code 0):
```
==> Creating database snapshot
Database: togather_production
Output: /var/backups/togather/togather_production_20260128_103014_abc1234.sql.gz

✓ Snapshot created (15.2 MB compressed)
✓ Retention: 7 days (expires 2026-02-04)

Snapshot ID: snap_01JBQR4KXYZ9876543210
File: togather_production_20260128_103014_abc1234.sql.gz
```

**Failure** (exit code 5):
```
==> Creating database snapshot
✗ Database connection failed: connection refused

Check DATABASE_URL environment variable:
  postgresql://user:pass@host:5432/dbname
```

---

## 5. cleanup.sh

**Purpose**: Remove old deployments, snapshots, and Docker images.

### Synopsis

```bash
./deploy/scripts/cleanup.sh [OPTIONS]
```

### Options

- `--snapshots`: Clean up expired database snapshots only
- `--images`: Clean up old Docker images only
- `--deployments`: Clean up old deployment logs only
- `--all`: Clean up everything (default if no specific flag provided)
- `--dry-run`: Show what would be deleted without deleting
- `--force`: Skip confirmation prompt
- `--help`: Display help message

### Examples

```bash
# Clean up everything (interactive confirmation)
./cleanup.sh

# Clean up expired snapshots only
./cleanup.sh --snapshots

# Dry-run to see what would be deleted
./cleanup.sh --dry-run

# Clean up without confirmation (automation)
./cleanup.sh --all --force
```

### Output

**Success** (exit code 0):
```
==> Cleanup

Expired snapshots (> 7 days):
  togather_production_20260121_103014_xyz5678.sql.gz (15.2 MB)
  togather_production_20260120_103014_def9876.sql.gz (14.8 MB)

Old Docker images (> 7 days):
  togather-server:xyz5678 (250 MB)
  togather-server:def9876 (248 MB)

Total reclaimed: 528 MB

Continue? (y/n) y

✓ Deleted 2 snapshots
✓ Deleted 2 Docker images
✓ Cleanup completed successfully
```

---

## Error Handling

All scripts follow consistent error handling:

1. **Validation Phase**: Check arguments, configuration, and prerequisites before making changes
2. **Atomic Operations**: Use transactions, lock files, and snapshots to prevent partial state
3. **Clear Error Messages**: Include error type, cause, and remediation steps
4. **Exit Codes**: Use standard exit codes for automation/CI integration
5. **Logging**: Write structured logs for troubleshooting and auditing

### Example Error Message Format

```
✗ <Error Type>: <Short Description>

Details:
  <Key>: <Value>
  <Key>: <Value>

Remediation:
  1. <Step to fix>
  2. <Step to fix>

See logs: <path-to-detailed-logs>
```

---

## Integration with CI/CD

All scripts support automation through:

- Standard exit codes (0 = success, non-zero = failure)
- `--json` output flag for machine parsing
- `--force` flag to skip interactive prompts
- Environment variables for configuration
- Structured log files for audit trails

### Example GitHub Actions Integration

```yaml
- name: Deploy to production
  run: |
    ./deploy/scripts/deploy.sh --force production
  env:
    ENVIRONMENT: production
    DATABASE_URL: ${{ secrets.DATABASE_URL }}
    
- name: Health check
  run: |
    ./deploy/scripts/health-check.sh \
      --max-attempts 10 \
      --expected-version ${{ github.sha }} \
      --json http://production.example.com/health
```

---

## Version Compatibility

| Script Version | Togather Server Version | Breaking Changes |
|----------------|-------------------------|------------------|
| 1.0.0 | v1.0.0+ | Initial release |

Future versions will maintain backward compatibility or provide migration guides.
