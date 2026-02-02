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

# Caddy configuration template
cp deploy/Caddyfile.example "${PACKAGE_DIR}/deploy/"

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

# Create installation script from template
echo "→ Copying install.sh from template..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_FILE="${SCRIPT_DIR}/install.sh.template"

if [[ ! -f "$TEMPLATE_FILE" ]]; then
    echo "  ✗ Error: install.sh.template not found at: $TEMPLATE_FILE"
    exit 1
fi

cp "$TEMPLATE_FILE" "${PACKAGE_DIR}/install.sh"
echo "  ✓ install.sh copied from template"

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
    # Also create/update .env.backup symlink for setup command compatibility
    sudo ln -sf "$(basename "$BACKUP_FILE")" "${APP_DIR}/.env.backup"
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

# Update Docker Caddyfile if it doesn't exist (but don't overwrite customized ones)
if [[ ! -f "${APP_DIR}/deploy/docker/Caddyfile" ]] && [[ -f "${APP_DIR}/deploy/docker/Caddyfile.example" ]]; then
    echo "→ Creating Docker Caddyfile from example..."
    sudo cp "${APP_DIR}/deploy/docker/Caddyfile.example" "${APP_DIR}/deploy/docker/Caddyfile"
    echo "  ✓ Docker Caddyfile created"
fi

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
