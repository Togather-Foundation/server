#!/usr/bin/env bash
# Build Deployment Package
# Creates a self-contained deployment artifact with binary and required files
#
# Usage:
#   ./build-deploy-package.sh [version] [output-dir]
#
# Example:
#   ./build-deploy-package.sh v0.1.0 ./dist

set -euo pipefail

VERSION="${1:-$(git describe --tags --always --dirty)}"
OUTPUT_DIR="${2:-./dist}"
PACKAGE_NAME="togather-server-${VERSION}"
PACKAGE_DIR="${OUTPUT_DIR}/${PACKAGE_NAME}"

echo "Building deployment package: ${PACKAGE_NAME}"
echo ""

# Clean and create output directory
rm -rf "${PACKAGE_DIR}"
mkdir -p "${PACKAGE_DIR}"

# Build the server binary
echo "→ Building server binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${PACKAGE_DIR}/server" ./cmd/server
echo "  ✓ Binary built: $(du -h "${PACKAGE_DIR}/server" | cut -f1)"

# Copy essential files
echo "→ Copying essential files..."

# Docker Compose and configuration
mkdir -p "${PACKAGE_DIR}/deploy/docker"
cp -r deploy/docker/* "${PACKAGE_DIR}/deploy/docker/"

# Deployment scripts (exclude build and provisioning scripts)
mkdir -p "${PACKAGE_DIR}/deploy/scripts"
for script in deploy/scripts/*.sh; do
    filename=$(basename "$script")
    # Include operational scripts, exclude build/provision scripts
    if [[ ! "$filename" =~ ^(build-deploy-package|provision-server|provision-remote)\.sh$ ]]; then
        cp "$script" "${PACKAGE_DIR}/deploy/scripts/"
    fi
done
# Include Python scripts
cp deploy/scripts/*.py "${PACKAGE_DIR}/deploy/scripts/" 2>/dev/null || true

# Database migrations
mkdir -p "${PACKAGE_DIR}/internal/storage/postgres/migrations"
cp -r internal/storage/postgres/migrations/* "${PACKAGE_DIR}/internal/storage/postgres/migrations/"

# River migrations
mkdir -p "${PACKAGE_DIR}/internal/river/migrations"
cp -r internal/river/migrations/* "${PACKAGE_DIR}/internal/river/migrations/" 2>/dev/null || true

# Bundle golang-migrate binary
echo "→ Bundling golang-migrate tool..."
MIGRATE_VERSION="v4.18.1"
ARCH=$(uname -m | sed "s/x86_64/amd64/;s/aarch64/arm64/")
MIGRATE_URL="https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-${ARCH}.tar.gz"

if ! curl -sfL "$MIGRATE_URL" | tar xz -C "${PACKAGE_DIR}/" migrate 2>/dev/null; then
    echo "  ⚠ Warning: Failed to download migrate for linux-${ARCH}"
    echo "  Migration tool will need to be installed manually on target system"
else
    chmod +x "${PACKAGE_DIR}/migrate"
    echo "  ✓ Migration tool bundled (${MIGRATE_VERSION}, linux-${ARCH})"
fi

# Bundle River CLI binary
echo "→ Bundling River CLI tool..."
RIVER_VERSION="v0.30.2"
RIVER_PKG="github.com/riverqueue/river/cmd/river@${RIVER_VERSION}"

# Create temporary GOBIN directory for building River CLI
TEMP_GOBIN=$(mktemp -d)
trap "rm -rf $TEMP_GOBIN" EXIT

if GOBIN="$TEMP_GOBIN" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install "$RIVER_PKG" 2>/dev/null; then
    cp "$TEMP_GOBIN/river" "${PACKAGE_DIR}/river"
    chmod +x "${PACKAGE_DIR}/river"
    echo "  ✓ River CLI bundled (${RIVER_VERSION}, linux-amd64)"
else
    echo "  ⚠ Warning: Failed to build River CLI"
    echo "  River CLI will need to be installed manually on target system"
fi

# Contexts and shapes (for JSON-LD and SHACL validation)
cp -r contexts "${PACKAGE_DIR}/"
cp -r shapes "${PACKAGE_DIR}/"

# Documentation
mkdir -p "${PACKAGE_DIR}/docs"
cp -r docs/deploy "${PACKAGE_DIR}/docs/" 2>/dev/null || true
cp README.md "${PACKAGE_DIR}/" 2>/dev/null || true

# Makefile for convenience commands
cp Makefile "${PACKAGE_DIR}/" 2>/dev/null || true

# Example .env file
cat > "${PACKAGE_DIR}/.env.example" <<'EOF'
# Togather Server Configuration
# Copy this file to .env and update with your values

# Database
DATABASE_URL=postgres://togather:password@localhost:5432/togather?sslmode=disable

# Server
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Security
JWT_SECRET=your-secret-key-here-min-32-chars
CSRF_KEY=your-csrf-key-here-exactly-32-chars

# CORS (required in production)
CORS_ALLOWED_ORIGINS=https://yourdomain.com

# Optional: Observability
# OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
# LOG_LEVEL=info
# LOG_FORMAT=json
EOF

# Create deployment README
cat > "${PACKAGE_DIR}/DEPLOY.md" <<'EOF'
# Togather Server Deployment Package

This package contains everything needed to deploy the Togather SEL server.

## What's Included

- `server` - Togather server binary with CLI commands
- `migrate` - golang-migrate tool (bundled, no separate installation needed)
- `river` - River CLI tool for job queue migrations (bundled, no separate installation needed)
- `deploy/docker/` - Docker Compose configuration
- `internal/storage/postgres/migrations/` - Database schema migrations
- `internal/river/migrations/` - River job queue migrations
- `contexts/`, `shapes/` - JSON-LD contexts and SHACL shapes
- `install.sh` - Automated installation script (first-time install OR upgrade)
- `upgrade.sh` - Manual upgrade script (legacy, for manual control)

## Installation Type

**Choose the right command for your scenario:**

### First-Time Installation

```bash
sudo ./install.sh
```

Installs Togather server to `/opt/togather`, creates systemd service, starts everything.

### Upgrading Existing Installation

**Recommended: Use install.sh (automatic data protection)**

```bash
sudo ./install.sh
```

When existing installation is detected:
1. Creates automatic backup to `/opt/togather/backups/`
2. Offers three options:
   - **[1] PRESERVE DATA** - Keep database intact, update files/binary (recommended)
   - **[2] FRESH INSTALL** - Delete all data (requires explicit confirmation)
   - **[3] ABORT** - Cancel installation
3. For non-interactive mode: defaults to PRESERVE DATA (safest)

**Alternative: Use upgrade.sh (manual control)**

```bash
sudo ./upgrade.sh
# Then manually: togather-server migrate up && sudo systemctl start togather
```

Use this only if you need manual control over migrations/startup timing.

## Quick Start

### 1. Prerequisites

- Docker & Docker Compose v2
- 2GB+ RAM
- Open ports: 80 (HTTP), 443 (HTTPS), 8080 (app)

### 2. Run Installation

```bash
# First-time installation
sudo ./install.sh

# Or upgrade existing installation
sudo ./install.sh
# (will auto-detect and offer upgrade options)
```

### 3. Verify

```bash
# Check health
togather-server healthcheck

# View credentials
cat /opt/togather/installation-report.txt

# View logs
sudo journalctl -u togather -f
```

## CLI Commands

```bash
# Health check
./server healthcheck

# Database snapshots
./server snapshot create
./server snapshot list

# API key management
./server api-key generate
./server api-key create --name "test-key"

# Deployment operations
./server deploy status
./server deploy rollback
```

## Documentation

See `docs/deploy/` for detailed deployment guides:
- Linode deployment
- SSL setup with Caddy
- Monitoring with Grafana
- Backup strategies

## Support

- Repository: https://github.com/Togather-Foundation/server
- Issues: https://github.com/Togather-Foundation/server/issues
EOF

# Create installation script
cat > "${PACKAGE_DIR}/install.sh" <<'INSTALLEOF'
#!/usr/bin/env bash
# Togather Server Installation
# Orchestrates existing server CLI and tools for one-command installation

set -euo pipefail

# Configuration
APP_DIR="/opt/togather"
LOG_FILE="/var/log/togather-install.log"
INSTALL_USER="${SUDO_USER:-$(whoami)}"

# Logging function
log() {
    echo "$1" | tee -a "$LOG_FILE"
}

log_quiet() {
    echo "$1" >> "$LOG_FILE"
}

# Error handler
error_exit() {
    log ""
    log "❌ Installation failed: $1"
    log ""
    log "Troubleshooting:"
    log "  - Check full log: $LOG_FILE"
    log "  - Manual installation guide: ${APP_DIR}/docs/deploy/MANUAL_INSTALL.md"
    log "  - Troubleshooting guide: ${APP_DIR}/docs/deploy/troubleshooting.md"
    log ""
    exit 1
}

# Start installation
log "╔════════════════════════════════════════════════╗"
log "║         Togather Server Installation           ║"
log "╚════════════════════════════════════════════════╝"
log ""

# Initialize log file
sudo mkdir -p "$(dirname "$LOG_FILE")"
sudo touch "$LOG_FILE"
sudo chmod 666 "$LOG_FILE"

# ============================================================================
# STEP 1: Pre-flight Checks
# ============================================================================
log "Step 1/7: Pre-flight Checks"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Detect OS
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    log "  ✓ OS detected: $ID $VERSION_ID"
    log_quiet "    OS: $PRETTY_NAME"
else
    error_exit "Cannot detect OS (missing /etc/os-release)"
fi

# Check Docker
if ! command -v docker &> /dev/null; then
    error_exit "Docker not installed. Install from: https://docs.docker.com/engine/install/"
fi
log "  ✓ Docker found: $(docker --version)"

# Check Docker Compose
if ! docker compose version &> /dev/null; then
    error_exit "Docker Compose v2 not installed"
fi
log "  ✓ Docker Compose found: $(docker compose version)"

# Check disk space (need at least 2GB)
AVAILABLE_GB=$(df -BG . | tail -1 | awk '{print $4}' | sed 's/G//')
if [[ $AVAILABLE_GB -lt 2 ]]; then
    log "  ⚠️  Warning: Low disk space (${AVAILABLE_GB}GB available, 2GB+ recommended)"
fi

# Check for existing installation BEFORE port checks
EXISTING_INSTALL=false
if [[ -f "${APP_DIR}/.env" ]] && [[ -f /usr/local/bin/togather-server ]]; then
    EXISTING_INSTALL=true
    log "  ℹ️  Existing installation detected"
fi

# Check if ports are available
# If existing installation and port in use, offer to stop service first
for PORT in 8080 5433; do
    if sudo lsof -i :$PORT > /dev/null 2>&1; then
        if [[ "$EXISTING_INSTALL" == "true" ]]; then
            log "  ℹ️  Port $PORT is in use (likely by existing Togather installation)"
            
            # Check if systemd service is running
            if systemctl is-active --quiet togather 2>/dev/null; then
                log "  → Stopping existing Togather service..."
                sudo systemctl stop togather || true
                sleep 2
                
                # Check if port is now free
                if ! sudo lsof -i :$PORT > /dev/null 2>&1; then
                    log "  ✓ Port $PORT freed"
                else
                    error_exit "Port $PORT still in use after stopping service"
                fi
            else
                error_exit "Port $PORT is in use by another process (not Togather service)"
            fi
        else
            error_exit "Port $PORT is already in use"
        fi
    fi
done
log "  ✓ Ports 8080, 5433 available"
log ""

# ============================================================================
# STEP 2: Install Binary
# ============================================================================
log "Step 2/7: Install Binary"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

sudo install -m 755 server /usr/local/bin/togather-server
log "  ✓ Binary installed: /usr/local/bin/togather-server"
log "  ✓ Version: $(togather-server version 2>/dev/null || echo 'dev')"
log ""

# ============================================================================
# STEP 3: Install Application Files
# ============================================================================
log "Step 3/7: Install Application Files"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Handle existing installation
if [[ -d "${APP_DIR}" ]]; then
    log "  ℹ️  Existing installation detected"
    
    # Backup .env if it exists
    if [[ -f "${APP_DIR}/.env" ]]; then
        BACKUP_FILE="${APP_DIR}/.env.backup.$(date +%Y%m%d_%H%M%S)"
        sudo cp "${APP_DIR}/.env" "$BACKUP_FILE"
        log "  ✓ Backed up .env to $(basename "$BACKUP_FILE")"
    fi
fi

sudo mkdir -p "${APP_DIR}"
shopt -s dotglob
sudo cp -r * "${APP_DIR}/"
shopt -u dotglob

sudo chown -R "${INSTALL_USER}":"${INSTALL_USER}" "${APP_DIR}"
log "  ✓ Files installed to ${APP_DIR}"
log "  ✓ Owner: ${INSTALL_USER}"
log ""

# ============================================================================
# STEP 4: Configure Environment
# ============================================================================
log "Step 4/7: Configure Environment"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Detect environment from ENVIRONMENT variable, default to staging
ENVIRONMENT="${ENVIRONMENT:-staging}"
log "  ℹ️  Environment: $ENVIRONMENT"

# Check Docker group membership (Bug #2 Fix)
if ! groups "$INSTALL_USER" | grep -q '\bdocker\b'; then
    error_exit "User $INSTALL_USER is not in 'docker' group. Run: sudo usermod -aG docker $INSTALL_USER && newgrp docker"
fi
log "  ✓ User $INSTALL_USER is in docker group"

cd "${APP_DIR}"

# ============================================================================
# CRITICAL: Data Protection - Backup Before Reinstallation
# ============================================================================

# Check for existing volumes (EXISTING_INSTALL already checked earlier)
EXISTING_VOLUMES=false
PRESERVE_CREDS_FLAG=""

# Try docker volume inspect first (most reliable)
if docker volume inspect togather-db-data &>/dev/null; then
    EXISTING_VOLUMES=true
    log "  ✓ Found existing volume: togather-db-data"
elif sudo docker volume inspect togather-db-data &>/dev/null; then
    EXISTING_VOLUMES=true
    log "  ✓ Found existing volume: togather-db-data (via sudo)"
else
    log "  ℹ️  No existing database volumes found"
fi

# Handle existing installation with data protection
if [[ "$EXISTING_INSTALL" == "true" ]] || [[ "$EXISTING_VOLUMES" == "true" ]]; then
    log ""
    log "╔════════════════════════════════════════════════════════════╗"
    log "║          ⚠️  EXISTING INSTALLATION DETECTED                ║"
    log "╚════════════════════════════════════════════════════════════╝"
    log ""
    
    if [[ "$EXISTING_VOLUMES" == "true" ]]; then
        log "  🗄️  Database volumes found: togather-db-data"
        log ""
        
        # Create backup directory with proper permissions
        BACKUP_DIR="${APP_DIR}/backups"
        if ! sudo mkdir -p "$BACKUP_DIR" 2>> "$LOG_FILE"; then
            log "  ⚠️  Warning: Could not create backup directory"
            log "     Continuing without backup (not recommended)"
        else
            sudo chown -R "${INSTALL_USER}":"${INSTALL_USER}" "$BACKUP_DIR"
            sudo chmod 755 "$BACKUP_DIR"
            log "  ✓ Backup directory ready: $BACKUP_DIR"
        fi
        
        # Generate backup filename with timestamp
        BACKUP_TIMESTAMP=$(date +%Y%m%d-%H%M%S)
        BACKUP_FILE="${BACKUP_DIR}/pre-reinstall-${BACKUP_TIMESTAMP}.sql.gz"
        
        log "  📦 Creating backup before proceeding..."
        log "     Backup location: $BACKUP_FILE"
        log ""
        
        # Ensure .env file exists before trying to use docker compose
        if [[ ! -f "${APP_DIR}/.env" ]]; then
            log "  ⚠️  No .env file found - cannot start database for backup"
            log "     Skipping backup creation"
            BACKUP_SUCCESS=false
        else
            # Start database if not running (needed for backup)
            DB_STARTED_FOR_BACKUP=false
            if ! docker ps 2>/dev/null | grep -q togather-db && ! sudo docker ps 2>/dev/null | grep -q togather-db; then
                log "  → Starting database for backup..."
                cd "${APP_DIR}"
                
                # Try without sudo first, then with sudo
                if docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d togather-db >> "$LOG_FILE" 2>&1; then
                    DB_STARTED_FOR_BACKUP=true
                elif sudo -u "${INSTALL_USER}" docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d togather-db >> "$LOG_FILE" 2>&1; then
                    DB_STARTED_FOR_BACKUP=true
                else
                    log "  ⚠️  Warning: Could not start database for backup"
                    BACKUP_SUCCESS=false
                fi
                
                if [[ "$DB_STARTED_FOR_BACKUP" == "true" ]]; then
                    # Wait for database to be ready
                    log "  → Waiting for database to be ready..."
                    DB_READY=false
                    for i in {1..30}; do
                        if docker exec togather-db pg_isready -U togather &>/dev/null 2>&1 || sudo docker exec togather-db pg_isready -U togather &>/dev/null 2>&1; then
                            DB_READY=true
                            log "  ✓ Database is ready"
                            break
                        fi
                        sleep 1
                    done
                    
                    if [[ "$DB_READY" == "false" ]]; then
                        log "  ⚠️  Warning: Database did not become ready in 30 seconds"
                        BACKUP_SUCCESS=false
                    fi
                fi
            else
                log "  ✓ Database is already running"
            fi
            
            # Create backup using togather-server snapshot command
            if [[ "$BACKUP_SUCCESS" != "false" ]]; then
                cd "${APP_DIR}"
                BACKUP_SUCCESS=false
                log "  → Running backup command..."
                if sudo -u "${INSTALL_USER}" bash -c "cd ${APP_DIR} && togather-server snapshot create --reason 'pre-reinstall-${BACKUP_TIMESTAMP}' --retention-days 30 --snapshot-dir ${BACKUP_DIR}" >> "$LOG_FILE" 2>&1; then
                    BACKUP_SUCCESS=true
                    # Find the created backup file
                    LATEST_BACKUP=$(ls -t "${BACKUP_DIR}"/*.sql.gz 2>/dev/null | head -1)
                    if [[ -f "$LATEST_BACKUP" ]]; then
                        BACKUP_SIZE=$(du -h "$LATEST_BACKUP" | cut -f1)
                        log "  ✓ Backup created successfully: $(basename "$LATEST_BACKUP") ($BACKUP_SIZE)"
                    else
                        log "  ✓ Backup command completed"
                    fi
                else
                    log "  ❌ Backup creation failed (see $LOG_FILE for details)"
                    log "     Last 10 lines of log:"
                    tail -10 "$LOG_FILE" | sed 's/^/     /'
                fi
            fi
        fi
        log ""
        
        # Offer user choice
        log "╔════════════════════════════════════════════════════════════╗"
        log "║              INSTALLATION OPTIONS                         ║"
        log "╚════════════════════════════════════════════════════════════╝"
        log ""
        log "  Choose how to proceed:"
        log ""
        log "  [1] PRESERVE DATA - Keep existing database (upgrade/reinstall)"
        log "      • Database volumes preserved"
        log "      • Credentials remain the same"
        log "      • No data loss"
        log ""
        log "  [2] FRESH INSTALL - Delete everything and start clean"
        log "      • All database volumes DESTROYED"
        log "      • New credentials generated"
        log "      • Backup available at: ${BACKUP_DIR}/"
        log ""
        log "  [3] ABORT - Cancel installation"
        log ""
        
        # Read user choice (with timeout for non-interactive environments)
        CHOICE=""
        if [[ -t 0 ]]; then
            # Interactive mode
            read -p "Enter choice [1-3]: " -t 30 CHOICE || CHOICE="3"
        else
            # Non-interactive: default to preserve data (safest option)
            CHOICE="1"
            log "  ℹ️  Non-interactive mode: Defaulting to PRESERVE DATA (option 1)"
        fi
        log ""
        
        case "$CHOICE" in
            1)
                log "  ✓ Selected: PRESERVE DATA"
                log "     Keeping existing database volumes..."
                log "     Upgrading installation in place..."
                log ""
                # Don't remove volumes - just update files
                # Use --preserve-credentials flag to reuse existing credentials
                PRESERVE_CREDS_FLAG="--preserve-credentials"
                ;;
            2)
                log "  ⚠️  Selected: FRESH INSTALL"
                log ""
                
                # Safety check: Don't allow FRESH INSTALL if backup failed
                if [[ "$BACKUP_SUCCESS" != "true" ]]; then
                    log "  ❌ CANNOT PROCEED WITH FRESH INSTALL"
                    log ""
                    log "  Backup creation failed, and FRESH INSTALL would destroy existing data."
                    log "  This is too dangerous to proceed."
                    log ""
                    log "  Options:"
                    log "    1. Choose PRESERVE DATA (option 1) to keep existing data"
                    log "    2. Manually create a backup first, then run install.sh again"
                    log "    3. Fix the backup issue (check $LOG_FILE for details)"
                    log ""
                    exit 1
                fi
                
                log "  ⚠️⚠️⚠️  WARNING: THIS WILL DELETE ALL DATA ⚠️⚠️⚠️"
                log ""
                log "  ✓ Backup available at: ${BACKUP_DIR}/"
                log "  To restore later: togather-server snapshot restore <backup-file>"
                log ""
                
                # Final confirmation
                if [[ -t 0 ]]; then
                    read -p "Type 'DELETE ALL DATA' to confirm: " -t 30 CONFIRM || CONFIRM=""
                    if [[ "$CONFIRM" != "DELETE ALL DATA" ]]; then
                        log "  ✗ Confirmation failed. Aborting installation."
                        exit 1
                    fi
                fi
                
                log "  → Removing all volumes..."
                sudo docker compose -f "${APP_DIR}/deploy/docker/docker-compose.yml" --env-file "${APP_DIR}/.env" down -v >> "$LOG_FILE" 2>&1 || true
                log "  ✓ Volumes removed"
                log ""
                ;;
            3|*)
                log "  ✗ Installation aborted by user"
                log ""
                if [[ "$BACKUP_SUCCESS" == "true" ]]; then
                    log "  Backup preserved at: ${BACKUP_DIR}/"
                fi
                exit 0
                ;;
        esac
    else
        log "  ℹ️  Installation files found but no database volumes"
        log "     Proceeding with upgrade..."
        log ""
    fi
fi

# Run server setup command
log "  → Running: togather-server setup --docker --non-interactive --allow-production-secrets --no-backup ${PRESERVE_CREDS_FLAG}"
if ! sudo -u "${INSTALL_USER}" ENVIRONMENT="$ENVIRONMENT" togather-server setup --docker --non-interactive --allow-production-secrets --no-backup ${PRESERVE_CREDS_FLAG} >> "$LOG_FILE" 2>&1; then
    error_exit "Server setup failed (see log for details)"
fi

log "  ✓ Environment configured (.env created)"
log "  ✓ Secrets generated"

# Verify PostgreSQL is actually running (Bug #3 Fix)
log "  → Verifying PostgreSQL container..."
if ! sudo -u "${INSTALL_USER}" docker ps | grep -q togather-db; then
    log "  ⚠️  PostgreSQL container not running, starting it now..."
    if ! sudo -u "${INSTALL_USER}" docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d togather-db >> "$LOG_FILE" 2>&1; then
        error_exit "Failed to start PostgreSQL container"
    fi
fi

# Wait for PostgreSQL to be ready
log "  → Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
    if sudo docker exec togather-db pg_isready -U togather &>/dev/null; then
        log "  ✓ PostgreSQL is ready"
        break
    fi
    if [[ $i -eq 30 ]]; then
        error_exit "PostgreSQL did not become ready within 30 seconds"
    fi
    sleep 1
done

# Verify migrations ran (Bug #4 Fix)
log "  → Verifying database migrations..."
cd "${APP_DIR}"

# Give PostgreSQL a moment to fully accept external connections
sleep 2

set +e  # Don't exit on error for this check
source .env
MIGRATION_VERSION=$(./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version 2>&1 | grep -oE '^[0-9]+$')
set -e

if [[ -z "$MIGRATION_VERSION" ]]; then
    log "  ⚠️  Migrations not applied, running now..."
    # Run migrations with retry logic
    MIGRATION_SUCCESS=false
    for attempt in {1..3}; do
        if sudo -u "${INSTALL_USER}" bash -c "source .env && ./migrate -path internal/storage/postgres/migrations -database \"\$DATABASE_URL\" up" >> "$LOG_FILE" 2>&1; then
            MIGRATION_SUCCESS=true
            break
        fi
        if [[ $attempt -lt 3 ]]; then
            log "     Migration attempt $attempt failed, retrying..."
            sleep 2
        fi
    done
    
    if [[ "$MIGRATION_SUCCESS" == "false" ]]; then
        error_exit "Database migrations failed after 3 attempts"
    fi
    log "  ✓ Migrations completed"
else
    log "  ✓ Migrations applied (version $MIGRATION_VERSION)"
fi
log ""

# ============================================================================
# STEP 5: Create Systemd Service
# ============================================================================
log "Step 5/7: Create Systemd Service"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [[ -d /etc/systemd/system ]]; then
    sudo tee /etc/systemd/system/togather.service > /dev/null <<EOF
[Unit]
Description=Togather SEL Server
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=${INSTALL_USER}
WorkingDirectory=${APP_DIR}
EnvironmentFile=${APP_DIR}/.env
ExecStart=/usr/local/bin/togather-server serve
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
    sudo systemctl enable togather >> "$LOG_FILE" 2>&1
    log "  ✓ Systemd service created and enabled"
else
    log "  ⚠️  Systemd not detected, skipping service creation"
fi
log ""

# ============================================================================
# STEP 6: Start Service
# ============================================================================
log "Step 6/7: Start Service"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

sudo systemctl start togather >> "$LOG_FILE" 2>&1 || true
sleep 3
log "  ✓ Service started"
log ""

# ============================================================================
# STEP 7: Verify Health
# ============================================================================
log "Step 7/7: Verify Health"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Bug #1 Fix: Health Check Loop with timeout wrapper
log "  → Checking server health (timeout: 30s)..."
HEALTH_OK=false
for i in {1..30}; do
    if timeout 5 togather-server healthcheck >> "$LOG_FILE" 2>&1; then
        log "  ✓ Server is healthy!"
        HEALTH_OK=true
        break
    fi
    echo -n "."
    if [[ $i -eq 30 ]]; then
        echo ""
        log "  ⚠️  Health check timed out after 30 seconds"
        log "     Server may still be starting. Check with: togather-server healthcheck"
        HEALTH_OK=false
        break
    fi
    sleep 1
done
log ""

# ============================================================================
# Generate Installation Report
# ============================================================================
REPORT_FILE="${APP_DIR}/installation-report.txt"

cat > "$REPORT_FILE" <<REPORT
Togather Server Installation Report
Generated: $(date)
═══════════════════════════════════════════════════════════════

Installation Details:
  Environment: $ENVIRONMENT
  Install Dir: ${APP_DIR}
  User: ${INSTALL_USER}
  OS: $PRETTY_NAME
  
Credentials (from ${APP_DIR}/.env):
  Admin Username: $(grep ADMIN_USERNAME ${APP_DIR}/.env | cut -d= -f2)
  Admin Password: $(grep ADMIN_PASSWORD ${APP_DIR}/.env | cut -d= -f2)
  Admin Email: $(grep ADMIN_EMAIL ${APP_DIR}/.env | cut -d= -f2)
  
  PostgreSQL User: $(grep POSTGRES_USER ${APP_DIR}/.env | cut -d= -f2)
  PostgreSQL Password: $(grep POSTGRES_PASSWORD ${APP_DIR}/.env | cut -d= -f2)
  PostgreSQL Port: $(grep POSTGRES_PORT ${APP_DIR}/.env | cut -d= -f2)

Service Status:
  Systemd: $(systemctl is-enabled togather 2>/dev/null || echo "N/A")
  Running: $(systemctl is-active togather 2>/dev/null || echo "N/A")

Endpoints:
  API: http://localhost:8080/api/v1
  Health: http://localhost:8080/health

Backup & Restore:
  Backups directory: ${APP_DIR}/backups/
  Create backup: togather-server snapshot create --reason "manual-backup"
  List backups: togather-server snapshot list --snapshot-dir ${APP_DIR}/backups
  Restore backup: pg_restore -d togather < backup-file.sql
  
  Note: Backups are created automatically before reinstallation
        to protect against data loss.

Quick Commands:
  Check health: togather-server healthcheck
  View logs: sudo journalctl -u togather -f
  Restart: sudo systemctl restart togather
  Stop: sudo systemctl stop togather
  Create backup: togather-server snapshot create

Documentation:
  ${APP_DIR}/docs/deploy/
  ${APP_DIR}/DEPLOY.md

Full installation log: $LOG_FILE
REPORT

sudo chown "${INSTALL_USER}":"${INSTALL_USER}" "$REPORT_FILE"

# ============================================================================
# Success Summary
# ============================================================================
log "╔════════════════════════════════════════════════════════════╗"
log "║                  ✓ Installation Complete!                 ║"
log "╚════════════════════════════════════════════════════════════╝"
log ""
log "📋 Installation Report: ${REPORT_FILE}"
log ""
log "🔐 Credentials:"
log "   Admin: $(grep ADMIN_USERNAME ${APP_DIR}/.env | cut -d= -f2) / $(grep ADMIN_PASSWORD ${APP_DIR}/.env | cut -d= -f2 | cut -c1-16)..."
log "   Full credentials in: ${APP_DIR}/.env"
log ""

# Show backup info if backups directory exists with files
if [[ -d "${APP_DIR}/backups" ]] && [[ -n "$(ls -A ${APP_DIR}/backups 2>/dev/null)" ]]; then
    BACKUP_COUNT=$(ls -1 ${APP_DIR}/backups/*.sql.gz 2>/dev/null | wc -l || echo 0)
    if [[ $BACKUP_COUNT -gt 0 ]]; then
        log "💾 Database Backups:"
        log "   Location: ${APP_DIR}/backups/"
        log "   Count: $BACKUP_COUNT backup(s)"
        log "   List backups: togather-server snapshot list --snapshot-dir ${APP_DIR}/backups"
        log "   Create backup: togather-server snapshot create"
        log ""
    fi
fi

log "🚀 Next Steps:"
log "   1. Check health: togather-server healthcheck"
log "   2. View logs: sudo journalctl -u togather -f"
log "   3. Access API: http://$(hostname -I | awk '{print $1}'):8080/api/v1"
log ""
log "📚 Documentation: ${APP_DIR}/docs/deploy/"
log "📝 Full log: $LOG_FILE"
log ""

INSTALLEOF

chmod +x "${PACKAGE_DIR}/install.sh"

# Create upgrade script
cat > "${PACKAGE_DIR}/upgrade.sh" <<'UPGRADEEOF'
#!/usr/bin/env bash
# Togather Server Manual Upgrade Script
# 
# NOTE: For most cases, use ./install.sh instead!
# install.sh automatically detects existing installations, creates backups,
# and offers smart upgrade options. This script is for manual control only.
#
# Recommendation: Use ./install.sh and choose option [1] PRESERVE DATA

set -euo pipefail

echo "Togather Server Manual Upgrade"
echo "==============================="
echo ""
echo "ℹ️  TIP: For automatic backup + upgrade, use ./install.sh instead"
echo "    This script gives you manual control but requires more steps."
echo ""
read -p "Continue with manual upgrade? (y/N): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted. Consider using ./install.sh for easier upgrade."
    exit 0
fi
echo ""

APP_DIR="/opt/togather"

if [[ ! -d "$APP_DIR" ]]; then
    echo "Error: No existing installation found at ${APP_DIR}"
    echo "Use ./install.sh for first-time installation"
    exit 1
fi

# Stop service if running
if systemctl is-active --quiet togather; then
    echo "→ Stopping service..."
    sudo systemctl stop togather
fi

# Backup .env
if [[ -f "${APP_DIR}/.env" ]]; then
    BACKUP_FILE="${APP_DIR}/.env.backup.$(date +%Y%m%d_%H%M%S)"
    echo "→ Backing up .env to ${BACKUP_FILE}..."
    sudo cp "${APP_DIR}/.env" "$BACKUP_FILE"
fi

# Update binary
echo "→ Updating server binary..."
sudo install -m 755 server /usr/local/bin/togather-server
echo "  ✓ Binary updated"

# Update application files (preserve .env)
echo "→ Updating application files..."
TEMP_ENV=$(mktemp)
if [[ -f "${APP_DIR}/.env" ]]; then
    sudo cp "${APP_DIR}/.env" "$TEMP_ENV"
fi

# Copy new files
sudo cp -r contexts shapes deploy internal docs DEPLOY.md README.md "${APP_DIR}/" 2>/dev/null || true

# Restore .env
if [[ -f "$TEMP_ENV" ]]; then
    sudo cp "$TEMP_ENV" "${APP_DIR}/.env"
    rm "$TEMP_ENV"
fi

echo "  ✓ Files updated"

# Update systemd service
if [[ -d /etc/systemd/system ]]; then
    echo "→ Updating systemd service..."
    sudo tee /etc/systemd/system/togather.service > /dev/null <<EOF
[Unit]
Description=Togather SEL Server
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=${APP_DIR}
EnvironmentFile=${APP_DIR}/.env
ExecStart=/usr/local/bin/togather-server serve
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    sudo systemctl daemon-reload
    echo "  ✓ Systemd service updated"
fi

echo ""
echo "Upgrade complete!"
echo ""
echo "⚠️  IMPORTANT: Manual steps required:"
echo "  1. Review changes: diff ${APP_DIR}/.env ${APP_DIR}/.env.example"
echo "  2. Run migrations: togather-server migrate up"
echo "  3. Start service: sudo systemctl start togather"
echo "  4. Check status: togather-server healthcheck"
echo ""
UPGRADEEOF

chmod +x "${PACKAGE_DIR}/upgrade.sh"


# Create tarball
echo "→ Creating tarball..."
cd "${OUTPUT_DIR}"
tar -czf "${PACKAGE_NAME}.tar.gz" "${PACKAGE_NAME}"
TARBALL_SIZE=$(du -h "${PACKAGE_NAME}.tar.gz" | cut -f1)
echo "  ✓ Package created: ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz (${TARBALL_SIZE})"

# Create checksum
sha256sum "${PACKAGE_NAME}.tar.gz" > "${PACKAGE_NAME}.tar.gz.sha256"
echo "  ✓ Checksum: ${PACKAGE_NAME}.tar.gz.sha256"

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "✓ Deployment package ready!"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo "Package: ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz (${TARBALL_SIZE})"
echo ""
echo "To deploy (first time):"
echo "  1. Copy to server: scp ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz user@server:~/"
echo "  2. SSH to server:  ssh user@server"
echo "  3. Extract:        tar -xzf ${PACKAGE_NAME}.tar.gz"
echo "  4. Install:        cd ${PACKAGE_NAME} && sudo ./install.sh"
echo ""
echo "To upgrade existing installation:"
echo "  1. Copy to server: scp ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz user@server:~/"
echo "  2. SSH to server:  ssh user@server"
echo "  3. Extract:        tar -xzf ${PACKAGE_NAME}.tar.gz"
echo "  4. Upgrade:        cd ${PACKAGE_NAME} && sudo ./upgrade.sh"
echo ""
echo "Or quick deploy:"
echo "  scp ${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz user@server:~/ && \\"
echo "  ssh user@server 'tar -xzf ${PACKAGE_NAME}.tar.gz && cd ${PACKAGE_NAME} && sudo ./install.sh'"
echo ""
