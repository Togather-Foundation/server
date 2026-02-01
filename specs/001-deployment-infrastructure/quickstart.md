# Quickstart: Deploying Togather Server

**Target Audience**: Operators deploying Togather server for the first time  
**Time**: 30-45 minutes for initial setup, 5-10 minutes for subsequent deployments  
**Prerequisites**: Linux server with Docker installed, Git, PostgreSQL access

---

## Table of Contents

1. [Prerequisites Check](#prerequisites-check)
2. [Initial Setup](#initial-setup)
3. [First Deployment](#first-deployment)
4. [Verify Deployment](#verify-deployment)
5. [Common Operations](#common-operations)
6. [Troubleshooting](#troubleshooting)

---

## Prerequisites Check

Before starting, ensure you have:

### Required Software

```bash
# Docker (20.10+)
docker --version
# Should output: Docker version 20.10.x or higher

# Docker Compose v2
docker compose version
# Should output: Docker Compose version 2.x or higher

# Git
git --version
# Should output: git version 2.x or higher

# PostgreSQL client tools
psql --version
# Should output: psql (PostgreSQL) 16.x or higher

# jq (for JSON parsing in scripts)
jq --version
# Should output: jq-1.x or higher
```

### Install Missing Tools

**Ubuntu/Debian**:
```bash
# Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker  # Activate group without logout

# PostgreSQL client
sudo apt install -y postgresql-client jq

# golang-migrate (for migrations)
curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/
```

**macOS**:
```bash
# Docker Desktop from https://www.docker.com/products/docker-desktop/

# PostgreSQL client and tools
brew install postgresql jq golang-migrate
```

### Server Requirements

- **CPU**: 2 cores minimum (4 cores recommended for production)
- **RAM**: 2GB minimum (4GB recommended, 8GB for monitoring stack)
- **Disk**: 20GB minimum (50GB recommended for database + snapshots)
- **Network**: Ports 80/443 for HTTP/HTTPS, 5432 for PostgreSQL

### Access Requirements

- SSH access to deployment server
- PostgreSQL database with admin privileges
- Domain name (optional but recommended for production)
- SSL certificate (optional, can use Cloudflare/reverse proxy)

---

## Initial Setup

### 1. Clone Repository

```bash
# Clone the repository
git clone https://github.com/Togather-Foundation/server.git
cd server

# Verify you're on the main branch
git branch
```

### 2. Configure Environment

Copy the environment template and customize:

```bash
# Copy production environment template
cp deploy/config/environments/.env.production.example deploy/config/environments/.env.production

# Edit configuration (use your preferred editor)
nano deploy/config/environments/.env.production
```

**Required Configuration** (`deploy/config/environments/.env.production`):

```bash
# Environment identifier
ENVIRONMENT=production

# Database connection (replace with your PostgreSQL credentials)
DATABASE_URL=postgresql://togather:CHANGE_ME@localhost:5432/togather_production

# JWT signing key (generate: openssl rand -base64 32)
JWT_SECRET=CHANGE_ME_generate_random_string

# Admin API key (generate: openssl rand -hex 32)
ADMIN_API_KEY=CHANGE_ME_generate_random_string

# Deployment metadata (automatically set by deployment script)
DEPLOYED_VERSION=
DEPLOYED_BY=operator@example.com
DEPLOYED_AT=

# Optional: Monitoring (Phase 2)
ENABLE_MONITORING=false

# Optional: Notifications (Phase 2)
NTFY_TOPIC=togather-prod-alerts
WEBHOOK_URL=
```

**Generate Secrets**:
```bash
# Generate JWT secret
echo "JWT_SECRET=$(openssl rand -base64 32)"

# Generate admin API key
echo "ADMIN_API_KEY=$(openssl rand -hex 32)"
```

### 3. Secure Environment File

```bash
# Set restrictive permissions (readable only by owner)
chmod 600 deploy/config/environments/.env.production

# Verify permissions
ls -la deploy/config/environments/.env.production
# Should show: -rw------- (600)
```

**CRITICAL**: Never commit `.env.production` to version control!

### 4. Create PostgreSQL Database

```bash
# Connect to PostgreSQL as admin
psql -h localhost -U postgres

# Create database and user
CREATE DATABASE togather_production;
CREATE USER togather WITH PASSWORD 'your_secure_password';
GRANT ALL PRIVILEGES ON DATABASE togather_production TO togather;

# Enable required extensions
\c togather_production
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

# Verify extensions
\dx
# Should show: postgis, vector, pg_trgm

\q  # Exit psql
```

### 5. Initialize Deployment Directories

```bash
# Create required directories
sudo mkdir -p /var/lib/togather/deployments
sudo mkdir -p /var/backups/togather
sudo mkdir -p /var/log/togather/deployments
sudo mkdir -p /var/lock

# Set ownership (replace 'youruser' with your username)
sudo chown -R $USER:$USER /var/lib/togather
sudo chown -R $USER:$USER /var/backups/togather
sudo chown -R $USER:$USER /var/log/togather
```

### 6. Validate Configuration

```bash
# Test database connection
psql "$DATABASE_URL" -c "SELECT version();"
# Should output PostgreSQL version information

# Verify Docker is running
docker ps
# Should show running containers (or empty list if none running)

# Check available disk space
df -h /var/backups/togather
# Should show at least 10GB available
```

---

## First Deployment

### 1. Build Deployment Scripts

First, ensure deployment scripts are executable:

```bash
chmod +x deploy/scripts/*.sh
```

### 2. Run Deployment (Dry-Run First)

```bash
# Validate configuration without deploying
./deploy/scripts/deploy.sh --dry-run production
```

**Expected Output**:
```
==> Validating deployment configuration
✓ Environment file found: deploy/config/environments/.env.production
✓ Database connection successful
✓ Required environment variables present
✓ Docker daemon accessible
✓ Disk space sufficient (35GB available)

==> Build validation
✓ Git repository clean (no uncommitted changes)
✓ Current version: abc1234
✓ Makefile targets available

Dry-run complete - configuration valid
```

### 3. Deploy to Production

```bash
# Run actual deployment
./deploy/scripts/deploy.sh production
```

**Expected Flow** (5-10 minutes):

```
==> Deploying togather-server to production
✓ Lock acquired
✓ Configuration validated
✓ Building Docker image (version: abc1234)
  Building... (this may take 3-5 minutes on first run)
✓ Docker image built: togather-server:abc1234

✓ Database snapshot created: togather_production_20260128_103014_abc1234.sql.gz (15.2 MB)
✓ Running 12 database migrations
  Migration 1/12: 20260101_001_initial_schema (152ms)
  Migration 2/12: 20260102_001_add_events_table (89ms)
  ...
  Migration 12/12: 20260128_001_add_federation_tables (234ms)

✓ Deploying blue-green (blue -> green)
✓ Waiting for health checks (attempt 1/30)
✓ Health checks passed (5/5)
  - http_endpoint: pass (2ms)
  - database: pass (8ms)
  - migrations: pass (5ms)
  - job_queue: pass (12ms)

✓ Traffic switched to green
✓ Deployment completed successfully

Deployment ID: dep_01JBQR2KXYZ9876543210
Version: abc1234
Duration: 4m 32s
Logs: /var/log/togather/deployments/dep_01JBQR2KXYZ9876543210.json
```

### 4. Verify Application is Running

```bash
# Check container status
docker ps | grep togather

# Should show:
# CONTAINER ID   IMAGE                        STATUS         PORTS
# a1b2c3d4e5f6   togather-server:abc1234     Up 2 minutes   0.0.0.0:8080->8080/tcp
```

---

## Verify Deployment

### 1. Health Check

```bash
# Manual health check
curl http://localhost:8080/health | jq '.'
```

**Expected Response**:
```json
{
  "status": "healthy",
  "version": "abc1234",
  "checks": {
    "database": {
      "status": "pass",
      "message": "PostgreSQL connection successful",
      "latency": "8ms"
    },
    "migrations": {
      "status": "pass",
      "message": "Schema version matches expected: 20260128_001",
      "latency": "5ms"
    },
    "http_endpoint": {
      "status": "pass",
      "message": "HTTP server responding",
      "latency": "2ms"
    },
    "job_queue": {
      "status": "pass",
      "message": "River job queue operational",
      "latency": "12ms"
    }
  },
  "timestamp": "2026-01-28T10:34:32Z"
}
```

### 2. Version Check

```bash
# Check deployed version
curl http://localhost:8080/version | jq '.'
```

**Expected Response**:
```json
{
  "version": "abc1234",
  "git_commit": "abc1234",
  "build_date": "2026-01-28T10:30:00Z",
  "go_version": "go1.25.6"
}
```

### 3. Test API Endpoint

```bash
# Test a simple API endpoint
curl http://localhost:8080/api/v1/events | jq '.'
```

### 4. Check Logs

```bash
# View application logs
docker logs togather-server-green

# View deployment log
cat /var/log/togather/deployments/dep_01JBQR2KXYZ9876543210.json | jq '.'
```

---

## Common Operations

### Update to New Version

```bash
# Pull latest code
git pull origin main

# Deploy new version
./deploy/scripts/deploy.sh production
```

### Rollback to Previous Version

```bash
# Interactive rollback
./deploy/scripts/rollback.sh production

# Force rollback (skip confirmation - for automation)
./deploy/scripts/rollback.sh --force production
```

### View Deployment History

```bash
# Check current deployment status
cat /var/lib/togather/deployments/production.json | jq '.current_deployment'

# View deployment history
cat /var/lib/togather/deployments/production.json | jq '.history'
```

### Create Manual Database Snapshot

```bash
# Create snapshot before risky operation
server snapshot create --reason "before risky operation"

# Output: Created snapshot: togather_production_20260128_103014.sql.gz
```

### Restore Database from Snapshot

```bash
# List available snapshots
server snapshot list

# Restore specific snapshot
SNAPSHOT_FILE="/var/backups/togather/togather_production_20260128_103014.sql.gz"
gunzip -c $SNAPSHOT_FILE | psql "$DATABASE_URL"
```

### Clean Up Old Artifacts

```bash
# Dry-run to see what would be deleted
./deploy/scripts/cleanup.sh --dry-run

# Run cleanup interactively
./deploy/scripts/cleanup.sh

# Automated cleanup (no prompts)
./deploy/scripts/cleanup.sh --force
```

---

## Troubleshooting

### Deployment Fails: Lock Conflict

**Error**:
```
ERROR: Another deployment is in progress
If deployment is stuck, remove lock: rm /var/lock/togather-deploy-production.lock
```

**Solution**:
```bash
# Check if deployment process is actually running
ps aux | grep deploy.sh

# If no process found, remove stale lock
rm /var/lock/togather-deploy-production.lock

# Retry deployment
./deploy/scripts/deploy.sh production
```

### Deployment Fails: Health Checks Timeout

**Error**:
```
✗ Health check failed after 30 attempts
Last response (HTTP 503): {"status": "unhealthy", ...}
```

**Solutions**:

1. **Check application logs**:
   ```bash
   docker logs togather-server-green
   ```

2. **Check database connection**:
   ```bash
   psql "$DATABASE_URL" -c "SELECT 1;"
   ```

3. **Increase health check timeout**:
   ```bash
   ./deploy/scripts/deploy.sh --health-timeout 60 production
   ```

4. **Manual rollback**:
   ```bash
   ./deploy/scripts/rollback.sh --force production
   ```

### Migration Fails

**Error**:
```
✗ Migration 5/12 failed: error: relation "events" already exists
```

**Solutions**:

1. **Check migration state**:
   ```bash
   migrate -path internal/storage/postgres/migrations \
           -database "$DATABASE_URL" version
   ```

2. **Resolve dirty migration** (if version shows "dirty"):
   ```bash
   # Manually fix database schema, then mark as clean
   migrate -path internal/storage/postgres/migrations \
           -database "$DATABASE_URL" force <version>
   ```

3. **Restore from snapshot** (if unfixable):
   ```bash
   # Find pre-migration snapshot
   ls -lt /var/backups/togather/ | head -3
   
   # Restore
   gunzip -c /var/backups/togather/togather_production_TIMESTAMP_COMMIT.sql.gz \
     | psql "$DATABASE_URL"
   ```

### Out of Disk Space

**Error**:
```
✗ Disk space insufficient (2GB available, 10GB required)
```

**Solutions**:

1. **Clean up old snapshots**:
   ```bash
   ./deploy/scripts/cleanup.sh --snapshots --force
   ```

2. **Remove unused Docker images**:
   ```bash
   docker image prune -a
   ```

3. **Check largest directories**:
   ```bash
   du -sh /var/backups/togather/*
   du -sh /var/lib/docker/volumes/*
   ```

### Docker Container Won't Start

**Error**:
```
✗ Container exited immediately after start
```

**Solutions**:

1. **Check container logs**:
   ```bash
   docker logs togather-server-green
   ```

2. **Check environment variables**:
   ```bash
   docker inspect togather-server-green | jq '.[0].Config.Env'
   ```

3. **Test database connection**:
   ```bash
   psql "$DATABASE_URL" -c "SELECT version();"
   ```

4. **Run container interactively for debugging**:
   ```bash
   docker run -it --rm \
     --env-file deploy/config/environments/.env.production \
     togather-server:abc1234 /bin/sh
   ```

---

## Next Steps

### Production Hardening

- [ ] Set up SSL/TLS certificates (Let's Encrypt or Cloudflare)
- [ ] Configure firewall rules (allow only 80/443, block direct database access)
- [ ] Set up automated backups (daily snapshots to S3/object storage)
- [ ] Enable monitoring (Prometheus + Grafana - Phase 2)
- [ ] Configure log rotation (`logrotate` for deployment logs)

### CI/CD Integration (Phase 2)

- [ ] Add GitHub Actions workflow for automated deployment
- [ ] Set up deployment notifications (ntfy.sh or Slack)
- [ ] Configure branch deployments (ephemeral staging environments)

### Monitoring Setup (Phase 2)

```bash
# Enable monitoring in environment config
echo "ENABLE_MONITORING=true" >> deploy/config/environments/.env.production

# Deploy with monitoring stack
./deploy/scripts/deploy.sh production

# Access Grafana
open http://localhost:3000
# Default credentials: admin / (password from GRAFANA_PASSWORD env var)
```

---

## Getting Help

- **Documentation**: See `specs/001-deployment-infrastructure/` for detailed architecture
- **Logs**: Check `/var/log/togather/deployments/` for deployment history
- **Issues**: Report deployment problems at https://github.com/Togather-Foundation/server/issues
- **Community**: Join the Togather community forum for operator support

---

## Appendix: Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ENVIRONMENT` | Yes | - | Target environment (`development`, `staging`, `production`) |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `JWT_SECRET` | Yes | - | JWT signing key (base64-encoded) |
| `ADMIN_API_KEY` | Yes | - | Admin API authentication key (hex-encoded) |
| `DEPLOYED_VERSION` | Auto | - | Git commit SHA of deployed version (set by deploy script) |
| `DEPLOYED_BY` | Auto | - | Email of operator who deployed (set by deploy script) |
| `DEPLOYED_AT` | Auto | - | ISO 8601 timestamp of deployment (set by deploy script) |
| `ENABLE_MONITORING` | No | `false` | Enable Prometheus/Grafana monitoring stack (Phase 2) |
| `GRAFANA_PASSWORD` | If monitoring | - | Grafana admin password (Phase 2) |
| `NTFY_TOPIC` | No | - | ntfy.sh topic for deployment alerts (Phase 2) |
| `WEBHOOK_URL` | No | - | Generic webhook URL for deployment events (Phase 2) |

---

**Version**: 1.0.0 (MVP)  
**Last Updated**: 2026-01-28  
**Maintained By**: Togather DevOps Team
