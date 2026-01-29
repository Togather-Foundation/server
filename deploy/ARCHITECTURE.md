# Deployment Infrastructure

This directory contains the deployment infrastructure for the Togather server, including Docker configurations, deployment scripts, and state management.

## Overview

The Togather server uses a **blue-green deployment strategy** with Docker Compose orchestration. This enables zero-downtime deployments with automatic rollback capabilities.

## Directory Structure

```
deploy/
├── config/                    # Configuration files
│   ├── deployment.yml         # Base deployment configuration
│   ├── deployment-state.json  # Current deployment state (DO NOT edit manually)
│   ├── deployment-state.schema.json  # JSON schema for state validation
│   └── environments/          # Environment-specific configs
│       ├── .env.development.example
│       ├── .env.staging.example
│       └── .env.production.example
├── docker/                    # Docker configurations
│   ├── Dockerfile             # Multi-stage application image
│   ├── docker-compose.yml     # Base compose configuration
│   ├── docker-compose.blue-green.yml  # Blue-green orchestration
│   └── nginx.conf             # Reverse proxy configuration
├── scripts/                   # Deployment automation scripts
└── docs/                      # Detailed deployment documentation
```

## Quick Start

### Prerequisites

- Docker >= 20.10
- Docker Compose >= 2.0
- golang-migrate (for database migrations)
- jq (for JSON processing)
- psql (PostgreSQL client)

### Initial Setup

1. **Copy environment template:**
   ```bash
   cp deploy/config/environments/.env.production.example deploy/config/environments/.env.production
   ```

2. **Configure secrets:**
   Edit `.env.production` and replace all `CHANGE_ME` placeholders with actual values:
   - `DATABASE_URL`: PostgreSQL connection string
   - `JWT_SECRET`: Base64-encoded secret for JWT tokens
   - `ADMIN_API_KEY`: Hex-encoded admin API key

3. **Deploy:**
   ```bash
   ./deploy/scripts/deploy.sh production
   ```

## Deployment State Management

The deployment system tracks state in `deploy/config/deployment-state.json`. This file is **automatically managed** by deployment scripts and should not be edited manually.

### State File Schema

The state file tracks:

1. **Current Deployment**: Active version information
   - Version (Git commit SHA)
   - Deployment timestamp and operator
   - Active blue/green slot
   - Health status

2. **Previous Deployment**: Last deployment for rollback
   - Same fields as current deployment
   - Used by rollback operations

3. **Deployment History**: Array of recent deployments
   - Configurable retention (typically last N deployments)
   - Includes success/failure status
   - Duration of each deployment

4. **Lock**: Deployment lock information
   - Prevents concurrent deployments
   - 30-minute timeout per specification
   - Auto-releases on script exit

### State File Example

```json
{
  "environment": "production",
  "current_deployment": {
    "id": "dep_01JBQR2KXYZ9876543210",
    "version": "abc1234",
    "git_commit": "abc1234567890def1234567890abcdef12345678",
    "deployed_at": "2026-01-28T10:30:00Z",
    "deployed_by": "operator@example.com",
    "active_slot": "blue",
    "health_status": "healthy",
    "status": "active",
    "migrations_run": 3,
    "rollback_count": 0
  },
  "previous_deployment": {
    "id": "dep_01JBQR1KXYZ9876543210",
    "version": "xyz5678",
    "git_commit": "xyz5678901234abc5678901234defabc56789012",
    "deployed_at": "2026-01-27T14:15:00Z",
    "deployed_by": "operator@example.com",
    "active_slot": "green",
    "health_status": "healthy",
    "status": "superseded"
  },
  "deployment_history": [],
  "lock": {
    "locked": false
  }
}
```

### Schema Validation

The state file conforms to the JSON Schema defined in `deployment-state.schema.json`. Deployment scripts validate the state file against this schema before making changes.

**Key validation rules:**

- `version`: Must match Git SHA format (`[a-f0-9]{7}`)
- `deployed_at`: Must be valid ISO 8601 timestamp
- `health_status`: Must be one of `healthy`, `degraded`, `unhealthy`, `unknown`
- `active_slot`: Must be either `blue` or `green`
- `lock.lock_expires_at`: Must be ≤ 30 minutes from `locked_at`

### Lock Mechanism

The deployment lock prevents concurrent deployments to the same environment:

- **Acquisition**: Deployment script creates lock entry in state file
- **Hold**: Lock held for duration of deployment
- **Release**: Lock automatically released when script exits (even on crash)
- **Timeout**: If lock age exceeds 30 minutes, next deployment can override (stale lock detection)

**Lock fields when active:**
```json
{
  "lock": {
    "locked": true,
    "lock_id": "lock_01JBQR6KXYZ9876543210",
    "locked_by": "operator@example.com",
    "locked_at": "2026-01-28T10:30:00Z",
    "lock_expires_at": "2026-01-28T11:00:00Z",
    "deployment_id": "dep_01JBQR2KXYZ9876543210",
    "pid": 12345,
    "hostname": "deploy-server.example.com"
  }
}
```

## Configuration Files

### deployment.yml

Base configuration shared across environments. Contains:

- Application settings (name, health check endpoint, port)
- Database configuration (extensions, migration path, snapshot retention)
- Deployment settings (strategy, lock timeout, rollback behavior)
- Provider-specific settings (Docker, AWS, GCP)

### .env Files

Environment-specific secrets and configuration:

- **DO NOT commit** `.env` files to version control
- Use `.env.*.example` as templates
- Required variables:
  - `ENVIRONMENT`: `development`, `staging`, or `production`
  - `DATABASE_URL`: PostgreSQL connection string
  - `JWT_SECRET`: Application secret for JWT tokens
  - `ADMIN_API_KEY`: Admin API authentication key

## Deployment Workflow

1. **Validation**: Check tool versions, Git commit, configuration
2. **Lock Acquisition**: Prevent concurrent deployments
3. **Build**: Create Docker image with version metadata
4. **Database Snapshot**: Backup database before migrations
5. **Migrations**: Apply pending database migrations
6. **Blue-Green Deploy**: Start new version in inactive slot
7. **Health Check**: Validate new deployment health
8. **Traffic Switch**: Update nginx to route to new version
9. **State Update**: Record deployment in state file
10. **Cleanup**: Release lock, log completion

## Health Checks

Deployment health checks validate:

- **HTTP Endpoint**: Server responds to requests
- **Database Connectivity**: PostgreSQL connection pool healthy
- **Database Migrations**: Schema version matches expected
- **Job Queue**: River job queue operational

Health status values:
- `healthy`: All checks pass
- `degraded`: Some checks warn, none fail
- `unhealthy`: One or more checks fail
- `unknown`: Health check not yet performed

## Rollback

To rollback to the previous deployment:

```bash
./deploy/scripts/rollback.sh production
```

Rollback operations:
1. Switch traffic to previous deployment slot
2. Validate health checks
3. Update state file

**Note**: Database migrations are **not automatically rolled back**. Manual intervention required for schema changes.

## Security

- All `.env` files must have `chmod 600` permissions
- Secrets are validated (no `CHANGE_ME` placeholders allowed)
- Deployment logs sanitize secrets (redact passwords, API keys)
- State files stored in secure locations with restricted access

## Troubleshooting

### Deployment Lock Stuck

If a deployment fails and leaves a lock:

```bash
# Check lock status
jq '.lock' deploy/config/deployment-state.json

# Manual lock release (use with caution)
jq '.lock.locked = false' deploy/config/deployment-state.json > tmp.json && mv tmp.json deploy/config/deployment-state.json
```

### Health Check Failures

Check deployment logs:
```bash
tail -f /var/log/togather/deployments/production_*.log
```

View current health status:
```bash
curl http://localhost:8080/health
```

### Migration Failures

Migrations are backed up automatically. See database snapshots:
```bash
ls -lh /var/backups/togather/
```

For detailed troubleshooting, see `deploy/docs/troubleshooting.md` (coming in future phases).

## Development vs Production

- **Development**: Fast iteration, verbose logging, no health check delays
- **Staging**: Production-like environment for testing
- **Production**: Strict validation, health checks, automatic rollback on failure

## References

- [Deployment Specification](../../specs/001-deployment-infrastructure/spec.md)
- [Data Model](../../specs/001-deployment-infrastructure/data-model.md)
- [Deployment Scripts Contract](../../specs/001-deployment-infrastructure/contracts/deployment-scripts.md)
- [Task List](../../specs/001-deployment-infrastructure/tasks.md)

## Support

For issues or questions:

1. Check deployment logs: `/var/log/togather/deployments/`
2. Verify configuration: `./deploy/scripts/deploy.sh --dry-run production`
3. Review specification documents in `specs/001-deployment-infrastructure/`

## Future Enhancements (Phase 2)

- Multi-provider support (AWS ECS/Fargate, GCP Cloud Run)
- Monitoring stack integration (Prometheus, Grafana, Alertmanager)
- Automated deployment validation tests
- Enhanced rollback with database snapshot restore

---

**Last Updated**: 2026-01-28  
**Status**: Phase 2 - Foundational (Task T013 complete)
