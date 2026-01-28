# Data Model: Deployment Infrastructure

**Feature**: Deployment Infrastructure  
**Date**: 2026-01-28  
**Phase**: 1 (Design & Contracts)

## Overview

This document defines the core entities, their relationships, and validation rules for the deployment infrastructure. These entities represent the deployment state, configuration, and operational artifacts managed by the deployment system.

---

## Core Entities

### 1. Deployment Configuration

**Purpose**: Represents the complete deployment configuration for a specific environment (development, staging, production).

**Storage**: YAML file (`deploy/config/deployment.yml` + `deploy/config/environments/.env.<environment>`)

**Schema**:

```yaml
# deploy/config/deployment.yml
app:
  name: string              # Application name (e.g., "togather-server")
  repository: string        # Git repository URL
  health_endpoint: string   # Health check path (e.g., "/health")
  health_timeout: duration  # Health check timeout (e.g., "30s")
  port: integer            # Application port (e.g., 8080)

database:
  extensions:              # Required PostgreSQL extensions
    - postgis
    - pgvector
    - pg_trgm
  migrations_path: string  # Path to migration files (e.g., "internal/storage/postgres/migrations")
  snapshot_retention_days: integer  # Days to keep snapshots (e.g., 7)

deployment:
  strategy: string         # Deployment strategy ("blue-green")
  lock_timeout: duration   # Max deployment duration (e.g., "30m")
  rollback_on_failure: boolean  # Auto-rollback if health checks fail

monitoring:                # Phase 2 feature (optional)
  enabled: boolean
  prometheus_port: integer
  grafana_port: integer

providers:
  docker:                  # Docker-specific configuration
    compose_file: string
    network: string
  aws:                     # Phase 2 (optional)
    region: string
    service: string       # "ecs" | "fargate"
  gcp:                     # Phase 2 (optional)
    region: string
    service: string       # "cloud-run"
```

**Environment Variables** (`.env.<environment>` file):

```bash
# Infrastructure
ENVIRONMENT=production
DATABASE_URL=postgresql://user:pass@host:5432/dbname

# Application secrets
JWT_SECRET=<base64-encoded-secret>
ADMIN_API_KEY=<hex-encoded-key>

# Deployment metadata (set by deployment script)
DEPLOYED_VERSION=<git-commit-sha>
DEPLOYED_BY=<operator-email>
DEPLOYED_AT=<iso8601-timestamp>

# Optional: Monitoring (Phase 2)
ENABLE_MONITORING=false
GRAFANA_PASSWORD=<secret>
NTFY_TOPIC=togather-prod-alerts
```

**Validation Rules**:
- `app.name` must be alphanumeric with hyphens (no spaces)
- `app.port` must be 1-65535
- `database.snapshot_retention_days` must be >= 1
- `deployment.strategy` must be one of: `blue-green` (only supported value in MVP)
- `ENVIRONMENT` must be one of: `development`, `staging`, `production`
- `DATABASE_URL` must be valid PostgreSQL connection string

**Relationships**:
- One Deployment Configuration per Environment
- References: Migration files (filesystem), Docker Compose files (filesystem)

---

### 2. Deployment Record

**Purpose**: Tracks the history and current state of deployments for an environment.

**Storage**: JSON file (`/var/lib/togather/deployments/<environment>.json`) + deployment logs (`/var/log/togather/deployments/`)

**Schema**:

```json
{
  "environment": "production",
  "current_deployment": {
    "id": "dep_01JBQR2KXYZ9876543210",
    "version": "abc1234",
    "deployed_at": "2026-01-28T10:30:00Z",
    "deployed_by": "operator@example.com",
    "status": "active",
    "health_status": "healthy",
    "migrations_run": 3,
    "rollback_count": 0
  },
  "previous_deployment": {
    "id": "dep_01JBQR1KXYZ9876543210",
    "version": "xyz5678",
    "deployed_at": "2026-01-27T14:15:00Z",
    "deployed_by": "operator@example.com",
    "status": "rolled_back",
    "health_status": "unhealthy",
    "rollback_count": 0
  },
  "history": [
    {
      "id": "dep_01JBQR0KXYZ9876543210",
      "version": "def9876",
      "deployed_at": "2026-01-26T08:45:00Z",
      "deployed_by": "operator@example.com",
      "status": "superseded",
      "duration_seconds": 245,
      "migrations_run": 1
    }
  ]
}
```

**Fields**:
- `id`: ULID (lexicographically sortable, timestamp-embedded)
- `version`: Git commit SHA (short form, 7 chars)
- `status`: Enum - `active`, `deploying`, `failed`, `rolled_back`, `superseded`
- `health_status`: Enum - `healthy`, `degraded`, `unhealthy`, `unknown`
- `migrations_run`: Count of migrations executed in this deployment
- `rollback_count`: Number of times this deployment was rolled back to

**Validation Rules**:
- `id` must be valid ULID format
- `version` must match `[a-f0-9]{7}` (Git SHA format)
- `deployed_at` must be valid ISO 8601 timestamp
- `status` transitions: `deploying` → `active` | `failed` | `rolled_back`
- Only one deployment can have `status: active` at a time

**Relationships**:
- One Deployment Record per Environment
- References: Migration records, Database snapshots, Deployment logs

---

### 3. Migration Record

**Purpose**: Tracks database schema migrations and their execution status.

**Storage**: PostgreSQL table (`schema_migrations`) managed by `golang-migrate` + deployment log

**Schema** (golang-migrate standard):

```sql
CREATE TABLE schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);
```

**Deployment Log Entry** (JSON):

```json
{
  "migration_id": "mig_01JBQR3KXYZ9876543210",
  "deployment_id": "dep_01JBQR2KXYZ9876543210",
  "version": 20260128_001,
  "description": "add_federation_tables",
  "executed_at": "2026-01-28T10:30:15Z",
  "duration_ms": 1234,
  "status": "success",
  "snapshot_taken": true,
  "snapshot_file": "togather_production_20260128_103014_abc1234.sql.gz"
}
```

**Fields**:
- `version`: Integer timestamp (YYYYMMDDHHmmss format)
- `dirty`: Boolean - `true` if migration failed mid-execution (needs manual cleanup)
- `status`: Enum - `success`, `failed`, `skipped`
- `snapshot_taken`: Boolean - whether pre-migration snapshot was created
- `snapshot_file`: Filename of database snapshot (if `snapshot_taken` is true)

**Validation Rules**:
- `version` must be unique and strictly increasing
- If `dirty` is `true`, no new migrations can run until resolved
- Migration files must exist in `migrations_path` before execution
- Snapshot must be created before migration runs (safety requirement)

**Relationships**:
- Many Migrations per Deployment
- One Migration per Deployment Record entry
- One Database Snapshot per Migration (optional)

---

### 4. Database Snapshot

**Purpose**: Point-in-time backup of the database before running migrations, enabling safe rollback.

**Storage**: Filesystem (`/var/backups/togather/`) or S3-compatible storage (Phase 2)

**Schema** (Filesystem metadata):

```json
{
  "snapshot_id": "snap_01JBQR4KXYZ9876543210",
  "database_name": "togather_production",
  "created_at": "2026-01-28T10:30:14Z",
  "created_by": "deploy-script",
  "file_path": "/var/backups/togather/togather_production_20260128_103014_abc1234.sql.gz",
  "file_size_bytes": 15728640,
  "compression": "gzip",
  "git_commit": "abc1234",
  "pre_migration_version": 20260127_001,
  "retention_until": "2026-02-04T10:30:14Z",
  "status": "available"
}
```

**Filename Convention**:
```
{database_name}_{timestamp}_{git_commit}.sql.gz
```

Example: `togather_production_20260128_103014_abc1234.sql.gz`

**Fields**:
- `snapshot_id`: ULID
- `created_at`: ISO 8601 timestamp
- `file_size_bytes`: Size of compressed snapshot file
- `compression`: Enum - `gzip`, `zstd` (Phase 2), `uncompressed`
- `pre_migration_version`: Schema version before migration (from `schema_migrations` table)
- `retention_until`: Auto-delete after this date (7 days from creation)
- `status`: Enum - `available`, `expired`, `deleted`, `restoring`

**Validation Rules**:
- `file_path` must exist and be readable
- `file_size_bytes` must match actual file size
- `retention_until` must be >= `created_at` + 7 days
- Snapshots with `status: expired` are automatically deleted by cleanup script

**Relationships**:
- One Database Snapshot per Migration (1:1)
- Referenced by: Migration Record, Rollback operations

---

### 5. Health Check Result

**Purpose**: Records the outcome of post-deployment health checks to validate deployment success.

**Storage**: In-memory (during deployment) + deployment log (persistent)

**Schema**:

```json
{
  "check_id": "hc_01JBQR5KXYZ9876543210",
  "deployment_id": "dep_01JBQR2KXYZ9876543210",
  "timestamp": "2026-01-28T10:32:00Z",
  "overall_status": "healthy",
  "checks": [
    {
      "name": "http_endpoint",
      "status": "pass",
      "latency_ms": 15,
      "message": "HTTP 200 OK",
      "details": {
        "endpoint": "http://localhost:8080/health",
        "response_code": 200
      }
    },
    {
      "name": "database_connectivity",
      "status": "pass",
      "latency_ms": 8,
      "message": "PostgreSQL connection successful",
      "details": {
        "pool_size": 10,
        "active_connections": 2
      }
    },
    {
      "name": "database_migrations",
      "status": "pass",
      "latency_ms": 5,
      "message": "Schema version matches expected: 20260128_001",
      "details": {
        "expected_version": 20260128_001,
        "actual_version": 20260128_001
      }
    },
    {
      "name": "job_queue",
      "status": "pass",
      "latency_ms": 12,
      "message": "River job queue operational",
      "details": {
        "pending_jobs": 3,
        "failed_jobs": 0
      }
    }
  ]
}
```

**Fields**:
- `overall_status`: Enum - `healthy`, `degraded`, `unhealthy`
- `checks[].status`: Enum - `pass`, `fail`, `warn`
- `checks[].name`: Identifier for the check type
- `checks[].latency_ms`: Time taken to execute the check
- `checks[].details`: Arbitrary JSON object with check-specific data

**Status Determination**:
- `healthy`: All checks have `status: pass`
- `degraded`: At least one check has `status: warn`, no `fail` checks
- `unhealthy`: At least one check has `status: fail`

**Validation Rules**:
- `overall_status` must be consistent with individual check statuses
- `latency_ms` must be >= 0
- `checks` array must contain at least: `http_endpoint`, `database_connectivity`, `database_migrations`

**Relationships**:
- Many Health Check Results per Deployment
- Referenced by: Deployment Record (determines deployment success/failure)

---

### 6. Deployment Lock

**Purpose**: Prevents concurrent deployments to the same environment by multiple operators.

**Storage**: Filesystem (`/var/lock/togather-deploy-<environment>.lock`)

**Schema** (Lock file content):

```json
{
  "lock_id": "lock_01JBQR6KXYZ9876543210",
  "environment": "production",
  "acquired_at": "2026-01-28T10:30:00Z",
  "acquired_by": "operator@example.com",
  "deployment_id": "dep_01JBQR2KXYZ9876543210",
  "pid": 12345,
  "hostname": "deploy-server.example.com",
  "expires_at": "2026-01-28T11:00:00Z"
}
```

**Fields**:
- `lock_id`: ULID
- `acquired_at`: ISO 8601 timestamp when lock was acquired
- `pid`: Process ID of deployment script
- `hostname`: Server where deployment is running
- `expires_at`: Lock timeout (30 minutes from `acquired_at`)

**Lock Lifecycle**:
1. **Acquire**: Deployment script creates lock file (atomic operation via `flock`)
2. **Hold**: Lock held for duration of deployment
3. **Release**: Lock automatically released when script exits (even on crash)
4. **Timeout**: If lock age exceeds 30 minutes, next deployment can override

**Validation Rules**:
- Only one lock can exist per environment at a time
- Lock file must be deleted or expired before new deployment can start
- `expires_at` must be <= `acquired_at` + 30 minutes

**Relationships**:
- One Lock per Environment (at most)
- Referenced by: Deployment Record (deployment in progress)

---

## Entity Relationships Diagram

```
┌─────────────────────┐
│ Deployment Config   │
│ (YAML + .env)       │
└──────┬──────────────┘
       │ configures
       ▼
┌─────────────────────┐       ┌──────────────────┐
│ Deployment Record   │──────▶│ Health Check     │
│ (JSON state file)   │       │ Result           │
└──────┬──────────────┘       └──────────────────┘
       │ contains
       ▼
┌─────────────────────┐       ┌──────────────────┐
│ Migration Record    │──────▶│ Database         │
│ (PostgreSQL +       │       │ Snapshot         │
│  deployment log)    │       │ (filesystem)     │
└─────────────────────┘       └──────────────────┘

┌─────────────────────┐
│ Deployment Lock     │
│ (filesystem)        │
└─────────────────────┘
     (guards all deployments)
```

**Key Relationships**:
- **1:N** - One Deployment Record contains many Migration Records
- **1:1** - One Migration Record has one Database Snapshot
- **1:N** - One Deployment Record has many Health Check Results
- **1:1** - One Environment has one Deployment Lock (when deployment active)
- **1:1** - One Environment has one Deployment Configuration

---

## State Transitions

### Deployment Status State Machine

```
                    ┌──────────┐
                    │  [none]  │
                    └────┬─────┘
                         │ deploy command
                         ▼
                    ┌──────────┐
              ┌────▶│deploying │
              │     └────┬─────┘
              │          │ migrations complete + health checks pass
              │          ▼
              │     ┌──────────┐
              │     │  active  │◀────────┐
              │     └────┬─────┘         │
              │          │                │
              │          │ new deployment │ rollback
              │          │ starts         │
              │          ▼                │
              │     ┌──────────┐         │
              └────▶│superseded│         │
                    └──────────┘         │
                                         │
                    ┌──────────┐         │
                    │  failed  │─────────┘
                    └──────────┘
                         ▲
                         │ health checks fail
                         │
                    ┌──────────┐
                    │deploying │
                    └──────────┘
```

**Valid Transitions**:
- `deploying` → `active` (success)
- `deploying` → `failed` (failure)
- `active` → `superseded` (new deployment)
- `active` → `rolled_back` (manual rollback)
- `failed` → `deploying` (retry)

---

## Validation Rules Summary

### Cross-Entity Validation

1. **Deployment Uniqueness**: Only one deployment with `status: active` per environment
2. **Migration Ordering**: Migration versions must be strictly increasing (no gaps)
3. **Snapshot Existence**: Database snapshot must exist before migration runs
4. **Lock Consistency**: Deployment cannot start if lock exists and is not expired
5. **Version Traceability**: Deployment `version` (Git SHA) must exist in repository history

### Data Integrity

1. **Foreign Key Integrity** (logical):
   - `deployment_id` in Migration Record must reference existing Deployment Record
   - `snapshot_id` in Migration Record must reference existing Database Snapshot
   
2. **Temporal Consistency**:
   - `deployed_at` must be <= `current_time`
   - `retention_until` must be >= `created_at`
   - `expires_at` (lock) must be >= `acquired_at`

3. **File System Consistency**:
   - Snapshot `file_path` must exist and be readable
   - Docker images referenced by `version` must exist in local registry or remote repository

---

## Storage Location Summary

| Entity | Primary Storage | Backup/Archive | Retention |
|--------|----------------|----------------|-----------|
| Deployment Config | `deploy/config/` (git) | N/A (version controlled) | Indefinite |
| Deployment Record | `/var/lib/togather/deployments/` | Deployment logs | 90 days |
| Migration Record | PostgreSQL `schema_migrations` | Deployment logs | Indefinite |
| Database Snapshot | `/var/backups/togather/` | S3 (Phase 2) | 7 days |
| Health Check Result | Deployment logs | None | 90 days |
| Deployment Lock | `/var/lock/` | None | Auto-cleanup |

---

## Next Steps

This data model will be implemented in:
1. **Bash scripts**: Deployment orchestrator reads/writes JSON state files
2. **YAML schemas**: JSON Schema validation for configuration files (Phase 2)
3. **PostgreSQL**: Migration tracking table (already exists via golang-migrate)
4. **Go structs**: Health check response types (application code)

See `contracts/` directory for detailed schemas and validation rules.
