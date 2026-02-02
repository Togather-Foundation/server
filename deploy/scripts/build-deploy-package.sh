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

# Contexts and shapes (for JSON-LD and SHACL validation)
cp -r contexts "${PACKAGE_DIR}/"
cp -r shapes "${PACKAGE_DIR}/"

# Documentation
mkdir -p "${PACKAGE_DIR}/docs"
cp -r docs/deploy "${PACKAGE_DIR}/docs/" 2>/dev/null || true
cp README.md "${PACKAGE_DIR}/" 2>/dev/null || true

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

## Quick Start

### 1. Prerequisites

- Docker & Docker Compose v2
- 2GB+ RAM
- Open ports: 80 (HTTP), 443 (HTTPS), 8080 (app)

### 2. Configuration

```bash
# Copy and edit environment file
cp .env.example .env
nano .env

# Generate secure secrets
./server api-key generate  # For JWT_SECRET
head -c 32 /dev/urandom | base64  # For CSRF_KEY
```

### 3. Start Services

```bash
# Start database
docker compose -f deploy/docker/docker-compose.yml up -d postgres

# Wait for database
sleep 5

# Run migrations
./server migrate up

# Start all services
docker compose -f deploy/docker/docker-compose.yml up -d
```

### 4. Verify

```bash
# Check health
./server healthcheck

# View logs
docker compose -f deploy/docker/docker-compose.yml logs -f server
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
set -euo pipefail

echo "Togather Server Installation"
echo "============================="
echo ""

# Detect OS
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    OS=$ID
else
    echo "Error: Cannot detect OS"
    exit 1
fi

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed"
    echo "Please install Docker first: https://docs.docker.com/engine/install/"
    exit 1
fi

# Check Docker Compose
if ! docker compose version &> /dev/null; then
    echo "Error: Docker Compose v2 is not installed"
    echo "Please install Docker Compose v2"
    exit 1
fi

# Install binary
echo "→ Installing server binary to /usr/local/bin..."
sudo install -m 755 server /usr/local/bin/togather-server
echo "  ✓ Installed as: togather-server"

# Create application directory
APP_DIR="/opt/togather"
echo "→ Installing to ${APP_DIR}..."

if [[ -d "${APP_DIR}" ]]; then
    echo "  Existing installation detected"
    
    # Backup .env if it exists
    if [[ -f "${APP_DIR}/.env" ]]; then
        echo "  Backing up existing .env..."
        sudo cp "${APP_DIR}/.env" "${APP_DIR}/.env.backup.$(date +%Y%m%d_%H%M%S)"
    fi
    
    # Ask if user wants clean install
    read -p "  Clean install (removes old files)? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "  Removing old installation..."
        sudo rm -rf "${APP_DIR:?}"/*
        # Restore .env if it was backed up
        if ls "${APP_DIR}"/.env.backup.* 1> /dev/null 2>&1; then
            LATEST_BACKUP=$(ls -t "${APP_DIR}"/.env.backup.* | head -1)
            sudo cp "$LATEST_BACKUP" "${APP_DIR}/.env"
            echo "  ✓ Restored .env from backup"
        fi
    else
        echo "  Upgrading in place (preserving existing files)..."
    fi
fi

sudo mkdir -p "${APP_DIR}"
# Use shopt to enable dotglob to include hidden files
shopt -s dotglob
sudo cp -r * "${APP_DIR}/"
shopt -u dotglob
sudo chown -R $(whoami):$(whoami) "${APP_DIR}"
echo "  ✓ Application files installed"

# Create systemd service
if [[ -d /etc/systemd/system ]]; then
    echo "→ Creating systemd service..."
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
    echo "  ✓ Systemd service created"
fi

echo ""
echo "Installation complete!"
echo ""
echo "Next steps:"
echo "  1. cd ${APP_DIR}"
echo "  2. cp .env.example .env"
echo "  3. nano .env  # Configure your settings"
echo "  4. docker compose -f deploy/docker/docker-compose.yml up -d postgres"
echo "  5. togather-server migrate up"
echo "  6. sudo systemctl enable --now togather"
echo ""
echo "Check status: togather-server healthcheck"
INSTALLEOF

chmod +x "${PACKAGE_DIR}/install.sh"

# Create upgrade script
cat > "${PACKAGE_DIR}/upgrade.sh" <<'UPGRADEEOF'
#!/usr/bin/env bash
set -euo pipefail

echo "Togather Server Upgrade"
echo "======================="
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
echo "Next steps:"
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
