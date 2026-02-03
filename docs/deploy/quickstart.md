# Quickstart: Deploying Togather Server

**Target Audience**: Operators deploying Togather server for production, and developers setting up local development  
**Time**: 5 minutes for automated deployment, 30-45 minutes for manual setup, 5-10 minutes for local development  
**Prerequisites**: Linux server with Docker installed, Git (for development)

---

## Table of Contents

1. [One-Command Installation](#one-command-installation) ⭐ **Start here for production**
2. [Deployment Workflows](#deployment-workflows)
3. [Local Development Setup](#local-development-setup)
4. [Production Deployment](#production-deployment)
5. [Prerequisites Check](#prerequisites-check)
6. [Initial Setup](#initial-setup)
7. [First Deployment](#first-deployment)
8. [Verify Deployment](#verify-deployment)
9. [Common Operations](#common-operations)
10. [Troubleshooting](#troubleshooting)

---
---

## One-Command Installation

**⏱️ Time: 5 minutes | Target: Fresh Ubuntu/Debian server**

The automated installer handles everything: binary installation, configuration generation, Docker setup, migrations, service creation, and health verification.

### Quick Deploy

```bash
# On your local machine: Build and copy package
make deploy-package
scp dist/togather-server-*.tar.gz user@server:~/

# On the target server: Extract and install
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh

# ✓ That's it! Server is healthy and running.
```

**What install.sh does:**
1. Pre-flight checks (Docker, disk space, ports)
2. Installs binary to `/usr/local/bin/togather-server`
3. Copies files to `/opt/togather`
4. Generates `.env` with secure secrets
5. Starts Docker PostgreSQL and runs migrations
6. Creates and starts systemd service
7. Verifies health (30s timeout)
8. Generates installation report with credentials

**Outputs:**
- **Credentials**: `/opt/togather/.env` (admin username/password, PostgreSQL credentials)
- **Report**: `/opt/togather/installation-report.txt`
- **Log**: `/var/log/togather-install.log`

**Verify:**
```bash
# Check health
togather-server healthcheck
# Expected: "Status: healthy"

# View credentials
cat /opt/togather/installation-report.txt

# Access API
curl http://localhost:8080/api/v1/events | jq .
```

**Troubleshooting:**
- **Installation failed**: Check `/var/log/togather-install.log`
- **Manual steps needed**: See [MANUAL_INSTALL.md](./MANUAL_INSTALL.md)
- **Common issues**: See [troubleshooting.md](./troubleshooting.md)

**Skip to**: [Verify Deployment](#verify-deployment)

---

## Deployment Workflows

Togather supports multiple deployment approaches:

### 1. One-Command Setup (New Server)

Use `provision.sh --with-app` for brand new servers:

```bash
# NEW: One-command server setup (provisions server + installs app)
./deploy/scripts/provision.sh deploy@server staging --with-app
```

**What happens:**
1. Provisions server (Docker, Caddy, firewall, deploy user)
2. Builds and copies deployment package
3. Runs `install.sh` on server
4. Server is ready with zero manual steps

**Characteristics:**
- Fastest path from zero to running server
- Combines provisioning + installation
- Good for: New deployments, fresh servers, automated setups
- Requires: SSH access to fresh Ubuntu/Debian server

### 2. Manual Step-by-Step Setup

Use `provision.sh` + `install.sh` for manual control:

```bash
# Step 1: Provision server (one time)
./deploy/scripts/provision.sh deploy@server staging

# Step 2: Install application
make deploy-package
scp dist/togather-server-<hash>.tar.gz deploy@server:~/
ssh deploy@server "cd togather-*/ && sudo ./install.sh"
```

**Characteristics:**
- Manual control over each step
- Review configuration before install
- Good for: Custom setups, security reviews, learning the process
- Requires: Same as one-command setup

### 3. Simple Deployment (with brief downtime)

Use `install.sh` for straightforward upgrades:

```bash
# On developer machine
make deploy-package
scp dist/togather-server-<hash>.tar.gz deploy@server:~/

# On server
ssh deploy@server
cd ~
tar -xzf togather-server-<hash>.tar.gz
cd togather-server-<hash>/
sudo ./install.sh  # Auto-detects existing install, preserves data
```

**Characteristics:**
- Brief downtime during service restart (~10-30 seconds)
- Simple, reliable, few moving parts
- **Auto-detects blue-green mode** if Caddy is configured
- Good for: Development, staging, low-traffic updates
- Requires: Deployment package only (no git repo on server)

### 4. Zero-Downtime Deployment (blue-green)

Use `deploy.sh --remote` for production-grade deployments:

```bash
# From developer machine (local git repo)
cd ~/togather/server
git pull origin main

# Deploy to staging
./deploy/scripts/deploy.sh staging --remote deploy@staging.server.com

# Deploy to production
./deploy/scripts/deploy.sh production --remote deploy@prod.server.com

# Deploy specific version
./deploy/scripts/deploy.sh production --remote deploy@prod.server.com --version v1.2.3
```

**Characteristics:**
- Zero downtime during deployment
- Blue-green slot switching (runs two versions temporarily)
- Automatic Caddy traffic routing
- Automatic health checks and validation
- Automatic rollback on failure
- Good for: Production, high-traffic services, critical updates
- Requires: Git repository on server (auto-cloned if missing)

**How it works:**
1. SSH to server and clone/update git repo at `/opt/togather/src/`
2. Checkout target commit
3. Build Docker image for inactive slot (blue or green)
4. Run database migrations
5. Deploy to inactive slot
6. Run health checks
7. Switch Caddy traffic to new slot
8. Update deployment state

**First-time setup:**

`install.sh` must be run first to provision the server:
```bash
# Option A: One-command setup (recommended)
./deploy/scripts/provision.sh deploy@server staging --with-app

# Option B: Manual setup
# 1. Run install.sh first (one time only)
scp dist/togather-server-<hash>.tar.gz deploy@server:~/
ssh deploy@server
sudo ./install.sh

# 2. Then use deploy.sh for all future updates
./deploy/scripts/deploy.sh staging --remote deploy@server
```

---
cd server

# Install development tools (golang-migrate, River CLI, golangci-lint, sqlc)
make install-tools

# Start everything (database + server in Docker)
make docker-up

# ✓ Containers started!
# ✓ Migrations run automatically
# ✓ Server: http://localhost:8080
# ✓ Database: localhost:5433
```

**Verify it's working:**
```bash
curl http://localhost:8080/health | jq '.status'
# Should return: "healthy"
```

**Option 2: Interactive Setup (Recommended for First-Time Setup)**

For a guided setup experience that generates all secrets and configures everything automatically:

```bash
git clone https://github.com/Togather-Foundation/server.git
cd server
make build

# Interactive guided setup - answers questions and configures everything
./server setup

# What it does:
# 1. Detects your environment (Docker vs local PostgreSQL)
# 2. Checks prerequisites
# 3. Generates secure secrets (JWT_SECRET, CSRF_KEY, admin password using crypto/rand)
# 4. Creates .env file in project root with all configuration
# 5. Sets up database and runs migrations
# 6. Creates your first API key and saves it to .env

# Or non-interactive setup for Docker:
./server setup --docker --non-interactive
```

**Global CLI Flags** (available for all `./server` commands):
- `--config <path>` - Custom config file path (optional, defaults to .env in project root)
- `--log-level <level>` - Set log level: debug, info, warn, error (default: info)
- `--log-format <format>` - Set log format: json, console (default: json)

Example: `./server serve --log-level debug --log-format console`

**Option 3: Local PostgreSQL (Manual Setup)**

If you prefer using your local PostgreSQL (port 5432):

```bash
# Install tools
make install-tools

# Check PostgreSQL has required extensions
make db-check

# Create database and install extensions
make db-setup

# Generate .env with auto-generated secrets
make db-init

# Run migrations
make migrate-up
make migrate-river

# Start server
make run           # Or: make dev (with hot reload)
```

### Development Commands

```bash
# Docker commands
make docker-up        # Start database + server (port 5433)
make docker-db        # Start only database
make docker-down      # Stop containers
make docker-logs      # View logs
make docker-rebuild   # Rebuild after code changes
make docker-clean     # Remove everything including volumes

# Database commands
make db-setup         # Create local database
make db-init          # Generate .env for local PostgreSQL
make db-check         # Check required extensions
make migrate-up       # Run app migrations
make migrate-river    # Run River job queue migrations

# Development commands
make test             # Run tests
make test-ci          # Run tests as CI does
make lint             # Run linter
make ci               # Full CI pipeline locally
make run              # Build and run server
make dev              # Run with hot reload (requires air)
```

### Troubleshooting Local Setup

**Port 5432 already in use:**
- Docker PostgreSQL uses port 5433 by default (no conflict!)
- Your system PostgreSQL stays on 5432
- Both can run simultaneously

**Migrations fail:**
```bash
# Ensure tools are installed
make install-tools

# Run migrations manually
DATABASE_URL="postgres://togather:dev_password_change_me@localhost:5433/togather?sslmode=disable" make migrate-up
DATABASE_URL="postgres://togather:dev_password_change_me@localhost:5433/togather?sslmode=disable" make migrate-river
```

**Health check shows unhealthy:**
```bash
# Check which components are failing
curl http://localhost:8080/health | jq '.checks'

# Common fixes:
# - Database not ready: wait 5 seconds and retry
# - Migrations not run: see migrations section above
# - River migrations missing: run make migrate-river
```

---

## Production Deployment

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
# Use CLI for health check
server healthcheck
server healthcheck --format json

# Or direct curl
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

### Upgrade Existing Installation

**⏱️ Time: 3-5 minutes | Target: Server with existing installation**

There are two ways to upgrade an existing Togather installation:

#### Option 1: Using install.sh (Recommended)

The `install.sh` script automatically detects existing installations and offers smart upgrade options:

```bash
# On your local machine: Build latest package
make deploy-package
scp dist/togather-server-*.tar.gz user@server:~/

# On the target server: Extract and run install.sh
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh
```

**What happens during upgrade:**
1. Detects existing installation at `/opt/togather`
2. Creates automatic backup to `/opt/togather/backups/pre-reinstall-YYYYMMDD-HHMMSS.sql.gz`
3. Presents three options:
   - **[1] PRESERVE DATA** - Keep database intact, update files/binary (recommended)
   - **[2] FRESH INSTALL** - Delete all data (requires explicit `DELETE ALL DATA` confirmation)
   - **[3] ABORT** - Cancel installation
4. If you choose option 1:
   - Preserves all database volumes
   - Updates binary to `/usr/local/bin/togather-server`
   - Updates application files
   - Runs migrations (idempotent, safe)
   - Restarts service
   - **No data loss**

**Non-interactive mode** (for automation):
```bash
# Defaults to PRESERVE DATA (option 1) - safest behavior
echo "" | sudo ./install.sh
```

### Update to New Version (Docker Compose Development)

For local development with Docker Compose:

```bash
# Pull latest code
git pull origin main

# Rebuild and restart
make docker-rebuild

# Verify
curl http://localhost:8080/health | jq '.status'
```

### Update to New Version (Production with deploy.sh)

For production deployments using the blue-green deployment script:

```bash
# Pull latest code
git pull origin main

# Deploy new version
./deploy/scripts/deploy.sh production
```

### Rollback to Previous Version

```bash
# Check deployment status first
server deploy status

# Interactive rollback
server deploy rollback production

# Force rollback (skip confirmation - for automation)
server deploy rollback production --force

# Dry run to validate
server deploy rollback production --dry-run
```

### View Deployment History

```bash
# Check current deployment status (CLI)
server deploy status
server deploy status --format json

# Or view state file directly
cat deploy/config/deployment-state.json | jq '.'
```

### Create Manual Database Snapshot

```bash
# Create snapshot before risky operation
server snapshot create --reason "pre-deployment backup"

# List existing snapshots
server snapshot list

# List with JSON output for scripting
server snapshot list --format json
```

### Restore Database from Snapshot

```bash
# List available snapshots
server snapshot list

# Restore specific snapshot (.sql.gz from server snapshot create)
SNAPSHOT_FILE="/var/lib/togather/db-snapshots/togather_production_20260128_103014_abc1234.sql.gz"
gunzip -c "$SNAPSHOT_FILE" | psql "$DATABASE_URL"
```

### Clean Up Old Artifacts

```bash
# Clean up old snapshots (CLI)
server snapshot cleanup --dry-run
server snapshot cleanup --retention-days 7

# Clean up Docker artifacts (bash script - still used)
./deploy/scripts/cleanup.sh --dry-run
./deploy/scripts/cleanup.sh
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
