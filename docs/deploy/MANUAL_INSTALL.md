# Manual Installation Guide

This guide explains how to manually install the Togather SEL server step-by-step. Use this for:
- **Troubleshooting** automated installation failures
- **Understanding** what `install.sh` does under the hood
- **Custom deployments** that deviate from standard setup
- **Agent context** for debugging installation issues

For automated installation, see [quickstart.md](./quickstart.md).

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Step-by-Step Installation](#step-by-step-installation)
4. [Troubleshooting Each Step](#troubleshooting-each-step)
5. [Recovery Procedures](#recovery-procedures)

## Overview

### What install.sh Does

The automated `install.sh` script orchestrates these tools and commands:

```
install.sh
  ├── Pre-flight checks (Docker, disk, ports)
  ├── Install binary → /usr/local/bin/togather-server
  ├── Copy files → /opt/togather
  ├── Call: togather-server setup --docker --non-interactive --allow-production-secrets
  │   ├── Generates .env with all secrets
  │   ├── Starts Docker PostgreSQL (make docker-db)
  │   ├── Waits for PostgreSQL (pg_isready)
  │   └── Runs migrations (make migrate-up, make migrate-river)
  ├── Create systemd service
  ├── Start service
  ├── Verify health (togather-server healthcheck)
  └── Generate installation-report.txt
```

### Architecture

```
┌─────────────────────────────────────────────────┐
│             Systemd Service                     │
│  /etc/systemd/system/togather.service          │
│  Runs: /usr/local/bin/togather-server serve    │
└────────────────┬────────────────────────────────┘
                 │
                 ├─ Reads: /opt/togather/.env
                 ├─ Uses: /opt/togather/contexts/
                 ├─ Uses: /opt/togather/shapes/
                 └─ Connects to: Docker PostgreSQL (port 5433)
                                    │
                    ┌───────────────┴───────────────┐
                    │  Docker Container             │
                    │  togather-postgres            │
                    │  PostgreSQL 16 + PostGIS      │
                    │  Data: /var/lib/docker/       │
                    └───────────────────────────────┘
```

## Prerequisites

### Required

1. **Operating System**: Ubuntu 22.04+ or Debian 11+
   ```bash
   cat /etc/os-release
   ```

2. **Docker**: Version 20.10+
   ```bash
   docker --version
   # Expected: Docker version 20.10.x or higher
   ```

3. **Docker Compose**: v2 (plugin)
   ```bash
   docker compose version
   # Expected: Docker Compose version v2.x.x
   ```

4. **Disk Space**: 2GB+ available
   ```bash
   df -h /opt
   # Should show 2G+ available
   ```

5. **Ports Available**: 8080 (API), 5433 (PostgreSQL)
   ```bash
   sudo lsof -i :8080
   sudo lsof -i :5433
   # Both should return nothing (ports free)
   ```

### Installation Fixes

**Docker not installed:**
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
newgrp docker  # Apply group membership without logout
```

**Docker Compose v2 not available:**
```bash
# Usually comes with Docker, verify:
docker compose version

# If missing, install Docker Desktop or standalone plugin:
# https://docs.docker.com/compose/install/
```

**Ports in use:**
```bash
# Find what's using port 8080
sudo lsof -i :8080
# Kill or reconfigure conflicting service

# Find what's using port 5433
sudo lsof -i :5433
```

## Step-by-Step Installation

### Step 1: Extract Deployment Package

```bash
# Copy package to server (from your local machine)
scp togather-server-*.tar.gz user@server:~/

# SSH to server
ssh user@server

# Extract package
tar -xzf togather-server-*.tar.gz
cd togather-server-*/

# Verify contents
ls -la
# Should see: server, migrate, install.sh, deploy/, docs/, etc.
```

### Step 2: Install Binary

```bash
# Install server binary to system path
sudo install -m 755 server /usr/local/bin/togather-server

# Verify installation
togather-server version
which togather-server  # Should show /usr/local/bin/togather-server
```

**Troubleshooting:**
- **Permission denied**: Need `sudo`
- **File not found**: Extract tarball first
- **Command not found after install**: PATH doesn't include `/usr/local/bin`
  ```bash
  export PATH="/usr/local/bin:$PATH"
  echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.bashrc
  ```

### Step 3: Install Application Files

```bash
# Create application directory
sudo mkdir -p /opt/togather

# Copy all files
sudo cp -r * /opt/togather/

# Set ownership to your user
sudo chown -R $USER:$USER /opt/togather

# Verify
ls -la /opt/togather
# Should show: server, migrate, deploy/, internal/, contexts/, shapes/, etc.
```

**Troubleshooting:**
- **Permission denied**: Need `sudo` for /opt
- **Disk full**: Check `df -h /opt`, need 2GB+

### Step 4: Configure Environment

This step generates `.env` with all secrets, starts Docker, and runs migrations.

```bash
cd /opt/togather

# Set environment
export ENVIRONMENT=staging  # or 'production'

# Run setup (does steps 4a-4d below)
./server setup --docker --non-interactive --allow-production-secrets --no-backup
```

**What this does:**

#### Step 4a: Generate .env File

If running setup manually fails, generate `.env` by hand:

```bash
cd /opt/togather

# Generate secrets
JWT_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '\n' | head -c 32)
CSRF_KEY=$(head -c 32 /dev/urandom | base64 | tr -d '\n' | head -c 32)
ADMIN_PASSWORD=$(head -c 16 /dev/urandom | base64 | tr -d '\n' | head -c 24)
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '\n' | head -c 32)

# Create .env file
cat > .env <<EOF
# Server Configuration
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_BASE_URL=http://localhost:8080

# Database Configuration
DATABASE_URL=postgresql://togather:${POSTGRES_PASSWORD}@localhost:5433/togather?sslmode=disable
DATABASE_MAX_CONNECTIONS=25
DATABASE_MAX_IDLE_CONNECTIONS=5

# Docker PostgreSQL Configuration
POSTGRES_DB=togather
POSTGRES_USER=togather
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_PORT=5433

# Bootstrap Admin User
ADMIN_USERNAME=admin
ADMIN_PASSWORD=${ADMIN_PASSWORD}
ADMIN_EMAIL=admin@localhost

# JWT Configuration
JWT_SECRET=${JWT_SECRET}
JWT_EXPIRY_HOURS=24

# CSRF Protection
CSRF_KEY=${CSRF_KEY}

# CORS Configuration
# WARNING: In production, replace * with your actual domain(s)
CORS_ALLOWED_ORIGINS=*

# Rate Limiting
RATE_LIMIT_PUBLIC=60
RATE_LIMIT_AGENT=300
RATE_LIMIT_ADMIN=0

# Background Jobs
JOB_RETRY_DEDUPLICATION=1
JOB_RETRY_RECONCILIATION=5
JOB_RETRY_ENRICHMENT=10

# Environment
ENVIRONMENT=${ENVIRONMENT}

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Federation
FEDERATION_NODE_NAME=local-dev
FEDERATION_SYNC_ENABLED=false

# Feature Flags
ENABLE_VECTOR_SEARCH=false
ENABLE_AUTO_RECONCILIATION=false
EOF

chmod 600 .env
echo "✓ Created .env file"

# Display generated credentials
echo ""
echo "Credentials:"
echo "  Admin: admin / ${ADMIN_PASSWORD}"
echo "  PostgreSQL: togather / ${POSTGRES_PASSWORD}"
```

#### Step 4b: Start Docker PostgreSQL

```bash
cd /opt/togather

# Start only the database container
docker compose -f deploy/docker/docker-compose.yml up -d postgres

# Or using make (if available)
make docker-db

# Verify container is running
docker ps | grep togather-postgres
# Should show container running on port 5433
```

#### Step 4c: Wait for PostgreSQL to Be Ready

```bash
# Poll until PostgreSQL accepts connections
for i in {1..30}; do
    if docker exec togather-postgres pg_isready -U togather > /dev/null 2>&1; then
        echo "✓ PostgreSQL is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ PostgreSQL did not start within 30 seconds"
        exit 1
    fi
    echo -n "."
    sleep 1
done
```

**Troubleshooting:**
- **Container not found**: Docker not running or container failed to start
  ```bash
  docker compose -f deploy/docker/docker-compose.yml logs postgres
  ```
- **Permission denied**: User not in docker group
  ```bash
  sudo usermod -aG docker $USER
  newgrp docker
  ```

#### Step 4d: Run Database Migrations

```bash
cd /opt/togather

# Source .env for DATABASE_URL
set -a
source .env
set +a

# Run migrations using bundled migrate tool
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up

# Or using make (if available)
make migrate-up
make migrate-river

# Verify migrations
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Should show: 16 (or current migration count)
```

**Troubleshooting:**
- **migrate: command not found**: Use bundled `./migrate` not system migrate
- **Connection refused**: PostgreSQL not ready (wait longer) or wrong port
- **Permission denied**: Check DATABASE_URL has correct password from .env

### Step 5: Create Systemd Service

```bash
# Create service file
sudo tee /etc/systemd/system/togather.service > /dev/null <<EOF
[Unit]
Description=Togather SEL Server
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=$USER
WorkingDirectory=/opt/togather
EnvironmentFile=/opt/togather/.env
ExecStart=/usr/local/bin/togather-server serve
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
sudo systemctl daemon-reload

# Enable service (start on boot)
sudo systemctl enable togather

echo "✓ Systemd service created and enabled"
```

**Troubleshooting:**
- **Permission denied**: Need `sudo` to create service
- **Unit not found**: Check file was created at `/etc/systemd/system/togather.service`

### Step 6: Start Service

```bash
# Start the service
sudo systemctl start togather

# Check status
sudo systemctl status togather
# Should show: active (running)

# View logs
sudo journalctl -u togather -f
# Press Ctrl+C to stop following logs
```

**Troubleshooting:**
- **Failed to start**: Check logs
  ```bash
  sudo journalctl -u togather -n 50 --no-pager
  ```
- **Environment file not found**: Verify `/opt/togather/.env` exists
- **Binary not found**: Verify `/usr/local/bin/togather-server` exists

### Step 7: Verify Health

```bash
# Wait a moment for server to start
sleep 3

# Check health
togather-server healthcheck

# Or via HTTP
curl http://localhost:8080/health | jq .

# Check API
curl http://localhost:8080/api/v1/events | jq .
```

**Expected health check output:**
```json
{
  "status": "healthy",
  "version": "...",
  "database": "healthy",
  "queue": "healthy"
}
```

**Troubleshooting:**
- **Connection refused**: Service not started or wrong port
- **Database unhealthy**: PostgreSQL container not running
- **Queue unhealthy** (degraded): River job queue optional, server still works

### Step 8: Create Installation Report

```bash
cat > /opt/togather/installation-report.txt <<REPORT
Togather Server Installation Report
Generated: $(date)
═══════════════════════════════════════════════════════════════

Installation Details:
  Environment: $ENVIRONMENT
  Install Dir: /opt/togather
  User: $USER
  OS: $(cat /etc/os-release | grep PRETTY_NAME | cut -d= -f2 | tr -d '"')
  
Credentials (from /opt/togather/.env):
  Admin Username: $(grep ADMIN_USERNAME /opt/togather/.env | cut -d= -f2)
  Admin Password: $(grep ADMIN_PASSWORD /opt/togather/.env | cut -d= -f2)
  Admin Email: $(grep ADMIN_EMAIL /opt/togather/.env | cut -d= -f2)
  
  PostgreSQL User: $(grep POSTGRES_USER /opt/togather/.env | cut -d= -f2)
  PostgreSQL Password: $(grep POSTGRES_PASSWORD /opt/togather/.env | cut -d= -f2)
  PostgreSQL Port: $(grep POSTGRES_PORT /opt/togather/.env | cut -d= -f2)

Service Status:
  Systemd: $(systemctl is-enabled togather 2>/dev/null || echo "N/A")
  Running: $(systemctl is-active togather 2>/dev/null || echo "N/A")

Endpoints:
  API: http://localhost:8080/api/v1
  Health: http://localhost:8080/health

Quick Commands:
  Check health: togather-server healthcheck
  View logs: sudo journalctl -u togather -f
  Restart: sudo systemctl restart togather
  Stop: sudo systemctl stop togather

Documentation:
  /opt/togather/docs/deploy/
  /opt/togather/DEPLOY.md
REPORT

echo "✓ Installation report: /opt/togather/installation-report.txt"
```

## Troubleshooting Each Step

### Docker Permission Issues

**Symptom:** `permission denied while trying to connect to the Docker daemon socket`

**Fix:**
```bash
# Add user to docker group
sudo usermod -aG docker $USER

# Apply group membership (choose one):
# Option 1: Log out and log back in
# Option 2: Start a new shell with the group
newgrp docker

# Option 3: Restart SSH session
exit
ssh user@server

# Verify
docker ps  # Should work without sudo
```

### PostgreSQL Not Starting

**Symptom:** Container exits immediately or health checks fail

**Diagnose:**
```bash
# Check container logs
docker compose -f /opt/togather/deploy/docker/docker-compose.yml logs postgres

# Check if port is in use
sudo lsof -i :5433

# Check disk space
df -h /var/lib/docker
```

**Common causes:**
- **Port conflict**: Another PostgreSQL on 5433
  ```bash
  # Change port in .env
  POSTGRES_PORT=5434
  DATABASE_URL=postgresql://togather:password@localhost:5434/togather?sslmode=disable
  
  # Restart
  docker compose -f /opt/togather/deploy/docker/docker-compose.yml down
  docker compose -f /opt/togather/deploy/docker/docker-compose.yml up -d postgres
  ```

- **Disk full**: `/var/lib/docker` out of space
  ```bash
  # Clean old containers/images
  docker system prune -a
  ```

### Migration Failures

**Symptom:** `migrate: ... error: ...`

**Diagnose:**
```bash
# Check current migration version
cd /opt/togather
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# Check migration history
docker exec togather-postgres psql -U togather -d togather -c "SELECT * FROM schema_migrations;"

# Check database connectivity
docker exec togather-postgres psql -U togather -d togather -c "SELECT version();"
```

**Common causes:**
- **Wrong DATABASE_URL**: Check .env, ensure password matches
- **Migrations already run**: Not an error, check version
- **Dirty migration**: Database in inconsistent state
  ```bash
  # Force version (DANGEROUS, backup first!)
  ./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force <version>
  ```

### Server Won't Start

**Symptom:** `systemctl status togather` shows failed

**Diagnose:**
```bash
# View recent logs
sudo journalctl -u togather -n 100 --no-pager

# Check for common errors
sudo journalctl -u togather | grep -i "error\|fatal\|panic"

# Test running manually
cd /opt/togather
./server serve
# Press Ctrl+C to stop
```

**Common causes:**
- **Missing .env**: Copy `.env.example` to `.env` and configure
- **Wrong database URL**: PostgreSQL not running or wrong credentials
- **Port 8080 in use**: Change `SERVER_PORT` in `.env`
- **Missing migrations**: Run migrations first

### Health Check Degraded

**Symptom:** Health returns `"status": "degraded"`

**Diagnose:**
```bash
# Get detailed health
curl http://localhost:8080/health | jq .

# Check what's degraded
togather-server healthcheck --format json | jq '.checks[] | select(.status != "healthy")'
```

**Common causes:**
- **River queue unhealthy**: Optional, server still works
  - Verify River migrations: `make migrate-river`
- **Database unhealthy**: PostgreSQL not accessible
  - Check container: `docker ps | grep postgres`
- **High latency**: Server under load or slow disk

## Recovery Procedures

### Reset Installation

Complete clean slate:

```bash
# Stop service
sudo systemctl stop togather
sudo systemctl disable togather

# Remove files
sudo rm -rf /opt/togather
sudo rm /usr/local/bin/togather-server
sudo rm /etc/systemd/system/togather.service
sudo systemctl daemon-reload

# Stop and remove Docker containers/volumes
docker compose -f /opt/togather/deploy/docker/docker-compose.yml down -v

# Start fresh with new install.sh
```

### Backup and Restore

**Backup:**
```bash
# Backup configuration
cp /opt/togather/.env /opt/togather/.env.backup.$(date +%Y%m%d)

# Backup database
togather-server snapshot create
# Creates snapshot in /opt/togather/backups/ (if configured)

# Or manual pg_dump
docker exec togather-postgres pg_dump -U togather togather > backup.sql
```

**Restore:**
```bash
# Restore .env
cp /opt/togather/.env.backup.YYYYMMDD /opt/togather/.env

# Restore database
cat backup.sql | docker exec -i togather-postgres psql -U togather togather

# Or use snapshot
togather-server snapshot restore <snapshot-file>
```

### Rollback to Previous Version

```bash
# Stop current version
sudo systemctl stop togather

# Install old binary
sudo install -m 755 old-server /usr/local/bin/togather-server

# Rollback migrations if needed
cd /opt/togather
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down

# Start service
sudo systemctl start togather
```

## Quick Reference

### File Locations

- **Binary**: `/usr/local/bin/togather-server`
- **Application**: `/opt/togather/`
- **Configuration**: `/opt/togather/.env`
- **Service**: `/etc/systemd/system/togather.service`
- **Logs**: `sudo journalctl -u togather`
- **Docker data**: `/var/lib/docker/volumes/`

### Common Commands

```bash
# Service management
sudo systemctl start togather
sudo systemctl stop togather
sudo systemctl restart togather
sudo systemctl status togather
sudo journalctl -u togather -f

# Health checks
togather-server healthcheck
curl http://localhost:8080/health | jq .

# Database
docker exec -it togather-postgres psql -U togather togather
docker compose -f /opt/togather/deploy/docker/docker-compose.yml logs postgres

# Migrations
cd /opt/togather
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up
./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down

# API testing
curl http://localhost:8080/api/v1/events | jq .
```

### Environment Variables Reference

See [.env.example](../../.env.example) for full list. Key variables:

- `DATABASE_URL`: PostgreSQL connection string
- `SERVER_PORT`: API port (default: 8080)
- `CORS_ALLOWED_ORIGINS`: Allowed CORS origins (use domain in prod, not *)
- `ADMIN_USERNAME`, `ADMIN_PASSWORD`: Initial admin credentials
- `JWT_SECRET`: Must be 32+ characters
- `ENVIRONMENT`: development|staging|production

## Next Steps

- **Monitoring**: See [monitoring.md](./monitoring.md)
- **Backups**: See [best-practices.md](./best-practices.md#backups)
- **Troubleshooting**: See [troubleshooting.md](./troubleshooting.md)
- **Rollback**: See [rollback.md](./rollback.md)
- **CI/CD**: See [ci-cd.md](./ci-cd.md)

## Support

- Documentation: `/opt/togather/docs/`
- Issues: https://github.com/Togather-Foundation/server/issues
- Logs: `sudo journalctl -u togather -f`
