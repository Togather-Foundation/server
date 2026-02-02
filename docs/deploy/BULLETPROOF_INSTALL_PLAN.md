# Bulletproof Installation Plan

## Problem Analysis: What Went Wrong

We had to manually:
1. Add `CORS_ALLOWED_ORIGINS` to `.env` (server failed without it)
2. Install `golang-migrate` tool manually (not included in package)
3. Source `.env` file manually to run migrations
4. Fix Docker permission issues (deploy user not in docker group in session)
5. Navigate confusing directory structure (run from `/opt/togather` vs `deploy/docker`)
6. Debug systemd service errors
7. Figure out migration commands from separate docs

**Root Cause**: The `install.sh` script installs files but doesn't actually **setup and start** a working server.

---

## Vision: One-Command Install

```bash
# On fresh Ubuntu/Debian server:
curl -fsSL https://get.togather.net | sudo bash
```

OR

```bash
# From deployment package:
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh
```

**Result**: Server running, healthy, and ready to accept events.

---

## Design Principles

1. **Self-Contained**: Package includes ALL dependencies (no external downloads during install)
2. **Idempotent**: Can run multiple times safely (detect existing state, don't fail)
3. **Atomic**: Either succeeds completely or rolls back cleanly
4. **Observable**: Clear progress indicators, structured logs, error messages with fixes
5. **Resumable**: Can continue from interruption (use state file)
6. **Environment-Aware**: Different behavior for dev/staging/production
7. **Zero-Knowledge**: Assumes user knows nothing about Go, Docker internals, or migration tools

---

## Implementation Plan

### Phase 1: Fix Immediate Issues (This Session)

#### 1.1. Bundle golang-migrate in Deployment Package

**Problem**: Migration tool not included, must be downloaded during install.

**Solution**:
```bash
# In build-deploy-package.sh:
# Download and bundle golang-migrate binary
MIGRATE_VERSION="v4.18.1"
ARCH=$(uname -m | sed "s/x86_64/amd64/;s/aarch64/arm64/")
curl -L "https://github.com/golang-migrate/migrate/releases/download/$MIGRATE_VERSION/migrate.linux-$ARCH.tar.gz" | \
  tar xzv -C "${PACKAGE_DIR}/" migrate
chmod +x "${PACKAGE_DIR}/migrate"
```

**Files**: `deploy/scripts/build-deploy-package.sh`

---

#### 1.2. Smart `install.sh` with Setup Wizard

**Problem**: Install script only copies files, doesn't configure or start services.

**Solution**: Multi-stage install with state tracking

```bash
#!/usr/bin/env bash
# Togather Server Installation (Bulletproof Edition)

set -euo pipefail

# Installation state tracking
STATE_FILE="/var/lib/togather/install-state.json"
INSTALL_LOG="/var/log/togather-install.log"

stages=(
  "check_prerequisites"
  "install_binary"
  "setup_directories"
  "generate_config"
  "start_docker"
  "run_migrations"
  "start_service"
  "verify_health"
)

# Resume from last successful stage
current_stage=$(jq -r '.last_completed_stage // "none"' "$STATE_FILE" 2>/dev/null || echo "none")

for stage in "${stages[@]}"; do
  if [[ "$current_stage" != "none" ]] && stage_completed "$stage"; then
    echo "✓ Skipping $stage (already completed)"
    continue
  fi
  
  echo "→ Running: $stage"
  if "$stage"; then
    mark_stage_complete "$stage"
    echo "✓ Completed: $stage"
  else
    echo "✗ Failed: $stage"
    exit 1
  fi
done

echo ""
echo "═══════════════════════════════════════════"
echo "✓ Installation Complete!"
echo "═══════════════════════════════════════════"
echo ""
echo "Server Status:"
server_healthcheck_output
echo ""
echo "Admin Credentials:"
show_admin_creds
echo ""
echo "Next Steps:"
echo "  • Test API: curl http://localhost:8080/api/v1/events"
echo "  • View logs: journalctl -u togather -f"
echo "  • Manage server: sudo systemctl {start|stop|restart} togather"
```

**Key Features**:
- **Resumable**: Tracks progress in JSON state file
- **Observable**: Shows clear stages with checkmarks
- **Self-Healing**: Detects existing state (Docker running? Migrations done?)
- **Zero-Config**: Generates all secrets automatically
- **Helpful**: Shows credentials, next steps, helpful commands

**Files**: `deploy/scripts/build-deploy-package.sh` (generates install.sh)

---

#### 1.3. Auto-Generate Required Env Vars

**Problem**: `.env` file missing required vars like `CORS_ALLOWED_ORIGINS`.

**Solution**: Generate complete `.env` with all required fields:

```bash
generate_config() {
  echo "→ Generating configuration..."
  
  # Detect environment
  ENV="${ENVIRONMENT:-staging}"
  
  # Generate all secrets
  JWT_SECRET=$(openssl rand -base64 32)
  CSRF_KEY=$(openssl rand -base64 32)
  ADMIN_PASSWORD=$(openssl rand -base64 16)
  POSTGRES_PASSWORD=$(openssl rand -base64 24)
  
  # Write .env with ALL required fields
  cat > /opt/togather/.env <<EOF
# SEL Backend Server - Environment Configuration
# Generated: $(date -Iseconds)

# Environment
ENVIRONMENT=${ENV}

# Server Configuration
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_BASE_URL=http://$(hostname -I | awk '{print $1}'):8080

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

# CORS Configuration (REQUIRED)
CORS_ALLOWED_ORIGINS=*

# Rate Limiting
RATE_LIMIT_PUBLIC=60
RATE_LIMIT_AGENT=300
RATE_LIMIT_ADMIN=0

# Background Jobs
JOB_RETRY_DEDUPLICATION=1
JOB_RETRY_RECONCILIATION=5
JOB_RETRY_ENRICHMENT=10

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Federation
FEDERATION_NODE_NAME=${HOSTNAME}
FEDERATION_SYNC_ENABLED=false

# Feature Flags
ENABLE_VECTOR_SEARCH=false
ENABLE_AUTO_RECONCILIATION=false
EOF

  chmod 600 /opt/togather/.env
  chown $(whoami):$(whoami) /opt/togather/.env
  
  echo "✓ Configuration generated"
  echo "✓ Admin password: ${ADMIN_PASSWORD}"
  echo "  (saved to /opt/togather/.admin-creds.txt)"
  
  # Save admin creds separately
  cat > /opt/togather/.admin-creds.txt <<CREDS
Admin Credentials
=================
Username: admin
Password: ${ADMIN_PASSWORD}
Email: admin@localhost

Database Credentials
====================
User: togather
Password: ${POSTGRES_PASSWORD}
Port: 5433
CREDS
  chmod 600 /opt/togather/.admin-creds.txt
}
```

**Files**: `deploy/scripts/build-deploy-package.sh` (in install.sh generation)

---

#### 1.4. Integrated Migration Runner

**Problem**: Migrations not run automatically, must know migration commands.

**Solution**: Bundle migrate binary and run automatically:

```bash
run_migrations() {
  echo "→ Running database migrations..."
  
  # Wait for PostgreSQL to be ready
  wait_for_postgres
  
  # Load environment
  set -a
  source /opt/togather/.env
  set +a
  
  # Run migrations using bundled migrate binary
  cd /opt/togather
  ./migrate -path internal/storage/postgres/migrations \
            -database "$DATABASE_URL" \
            up
  
  MIGRATION_VERSION=$(./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version 2>&1 | awk '{print $1}')
  
  echo "✓ Migrations complete (version: $MIGRATION_VERSION)"
}

wait_for_postgres() {
  echo "  Waiting for PostgreSQL..."
  for i in {1..30}; do
    if docker exec togather-db pg_isready -U togather >/dev/null 2>&1; then
      echo "  ✓ PostgreSQL ready"
      return 0
    fi
    sleep 1
  done
  echo "  ✗ PostgreSQL not ready after 30s"
  return 1
}
```

**Files**: `deploy/scripts/build-deploy-package.sh` (in install.sh generation)

---

#### 1.5. Fix Docker Permission Handling

**Problem**: Deploy user added to docker group but can't access Docker until logout/login.

**Solution**: Detect and provide clear instructions:

```bash
start_docker() {
  echo "→ Starting Docker services..."
  
  # Check if user can access Docker
  if ! docker ps >/dev/null 2>&1; then
    if groups | grep -q docker; then
      echo "⚠️  Docker permission issue detected"
      echo ""
      echo "You are in the docker group but the session hasn't refreshed."
      echo ""
      echo "Option 1 (Recommended): Re-run with newgrp"
      echo "  sudo -E env PATH=\$PATH newgrp docker <<'DOCKERCMDS'
      echo "  cd $(pwd) && sudo ./install.sh --resume"
      echo "  DOCKERCMDS"
      echo ""
      echo "Option 2: Logout and login again, then:"
      echo "  cd $(pwd) && sudo ./install.sh --resume"
      return 1
    else
      echo "✗ Not in docker group. Adding you now..."
      sudo usermod -aG docker $(whoami)
      echo "⚠️  You must logout and login for docker group to take effect"
      echo "   Then run: cd $(pwd) && sudo ./install.sh --resume"
      return 1
    fi
  fi
  
  # Start containers
  cd /opt/togather
  docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d togather-db
  
  echo "✓ Docker services started"
}
```

**Files**: `deploy/scripts/build-deploy-package.sh` (in install.sh generation)

---

#### 1.6. Health Check Before Success

**Problem**: Installation completes but server is unhealthy.

**Solution**: Verify health before declaring success:

```bash
verify_health() {
  echo "→ Verifying server health..."
  
  # Start service
  sudo systemctl enable --now togather
  
  # Wait for health check
  for i in {1..30}; do
    if /usr/local/bin/togather-server healthcheck >/dev/null 2>&1; then
      echo "✓ Server is healthy"
      return 0
    fi
    sleep 2
  done
  
  echo "✗ Server health check failed"
  echo ""
  echo "Troubleshooting:"
  echo "  • Check logs: journalctl -u togather -n 50"
  echo "  • Check Docker: docker ps"
  echo "  • Manual health check: togather-server healthcheck"
  echo ""
  return 1
}
```

**Files**: `deploy/scripts/build-deploy-package.sh` (in install.sh generation)

---

### Phase 2: Enhanced UX (Next Session)

#### 2.1. Interactive vs Non-Interactive Modes

```bash
# Interactive (asks questions)
sudo ./install.sh

# Non-interactive (uses defaults)
sudo ./install.sh --non-interactive

# Resume from failure
sudo ./install.sh --resume

# Dry-run (check without changing)
sudo ./install.sh --dry-run
```

#### 2.2. Better Error Messages

Replace:
```
Error: config error: CORS_ALLOWED_ORIGINS is required
```

With:
```
✗ Configuration Error: Missing CORS_ALLOWED_ORIGINS

The CORS_ALLOWED_ORIGINS environment variable is required when
ENVIRONMENT=staging or ENVIRONMENT=production.

Fix:
  1. Edit /opt/togather/.env
  2. Add: CORS_ALLOWED_ORIGINS=*  (or your domain)
  3. Restart: sudo systemctl restart togather

Learn more: https://docs.togather.net/security/cors
```

#### 2.3. Pre-Flight Checks

```bash
check_prerequisites() {
  echo "→ Checking prerequisites..."
  
  checks=(
    "check_os:Ubuntu 22.04+ or Debian 11+"
    "check_ram:2GB RAM minimum"
    "check_disk:10GB disk space"
    "check_ports:Ports 8080,5433 available"
    "check_docker:Docker installed"
  )
  
  for check in "${checks[@]}"; do
    func="${check%%:*}"
    desc="${check##*:}"
    if "$func"; then
      echo "  ✓ $desc"
    else
      echo "  ✗ $desc"
      failed=true
    fi
  done
  
  if [[ "$failed" == "true" ]]; then
    echo ""
    echo "Prerequisites not met. See: docs/deploy/quickstart.md"
    return 1
  fi
  
  echo "✓ Prerequisites met"
}
```

#### 2.4. Installation Report

```bash
# At end of install, generate report
cat > /opt/togather/installation-report.txt <<EOF
Togather Server Installation Report
====================================
Date: $(date -Iseconds)
Version: ${VERSION}
Environment: ${ENVIRONMENT}

System Info:
  OS: $(lsb_release -d | cut -f2)
  Kernel: $(uname -r)
  Docker: $(docker --version)
  
Installation Details:
  Directory: /opt/togather
  Binary: /usr/local/bin/togather-server
  Service: togather.service (enabled, running)
  Database: PostgreSQL 16 (Docker, port 5433)
  Migrations: Applied (version $MIGRATION_VERSION)
  
Server Status:
  Health: $(togather-server healthcheck | head -1)
  URL: http://$(hostname -I | awk '{print $1}'):8080
  API: http://$(hostname -I | awk '{print $1}'):8080/api/v1/events
  
Admin Credentials:
  Username: admin
  Password: $ADMIN_PASSWORD
  Email: admin@localhost
  
Next Steps:
  1. Test API: curl http://localhost:8080/api/v1/events
  2. Configure CORS: Edit /opt/togather/.env (CORS_ALLOWED_ORIGINS)
  3. Set up SSL: See docs/deploy/LINODE-DEPLOYMENT.md
  4. Monitor logs: journalctl -u togather -f

Logs:
  Installation: /var/log/togather-install.log
  Service: journalctl -u togather
  
Documentation: /opt/togather/docs/deploy/
Support: https://github.com/Togather-Foundation/server/issues
EOF

echo "Installation report saved: /opt/togather/installation-report.txt"
```

---

### Phase 3: One-Line Remote Install (Future)

```bash
# Install from GitHub release
curl -fsSL https://get.togather.net | sudo bash

# Or with version
curl -fsSL https://get.togather.net | sudo bash -s -- --version v0.2.0

# Custom domain
curl -fsSL https://get.togather.net | sudo bash -s -- --domain events.mycity.org
```

**Implementation**: `get.togather.net` script that:
1. Detects OS
2. Downloads latest release
3. Extracts and runs `install.sh --non-interactive`

---

## Success Criteria

✅ Installation completes in **one command** (no manual steps)
✅ **Zero configuration** needed (all secrets auto-generated)
✅ **Self-documenting** (shows creds, URLs, next steps)
✅ **Resumable** (can recover from interruption)
✅ **Observable** (clear progress, structured logs)
✅ **Verifiable** (health check before declaring success)
✅ Works on **fresh Ubuntu/Debian** (no prerequisites except Docker)

---

## Files to Modify

1. `deploy/scripts/build-deploy-package.sh` - Bundle migrate, generate smarter install.sh
2. `cmd/server/cmd/setup.go` - Add `--production` mode for non-interactive server setup
3. `docs/deploy/quickstart.md` - Update with new one-command install
4. `docs/deploy/troubleshooting.md` - Add common install issues

---

## Testing Plan

1. **Fresh Ubuntu 22.04 VM** - Run `sudo ./install.sh`, verify server healthy
2. **Interruption Recovery** - Kill install.sh mid-run, resume with `--resume`
3. **Idempotency** - Run `sudo ./install.sh` twice, should succeed both times
4. **Docker Permission Issue** - Test on user without docker group
5. **Port Conflict** - Test with port 8080 already in use

---

## Timeline

**This Session (Phase 1)**: 
- Fix immediate issues (bundle migrate, smart install.sh, auto-config)
- Test on Linode server

**Next Session (Phase 2)**:
- Enhanced UX (interactive mode, pre-flight checks, better errors)
- Installation report

**Future (Phase 3)**:
- One-line remote install script
- CI/CD integration

---

## Questions for User

1. **Priority**: Should we fix Phase 1 now or plan everything first?
2. **Scope**: Do you want one-command install for ALL environments (dev/staging/prod) or just prod?
3. **Secrets**: Should we auto-generate ALL secrets or prompt for some (e.g., admin password)?
4. **Migration**: Should migrations run automatically or require explicit confirmation?
5. **Resumability**: Is state tracking important or can we just fail and ask user to re-run?

---

**Next Step**: Implement Phase 1 fixes to make install.sh bulletproof.
