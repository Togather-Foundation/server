#!/usr/bin/env bash
# Quick fix script for missing Caddy on staging server
# Run this on the staging server to set up system Caddy properly
#
# Usage:
#   sudo ./fix-caddy-staging.sh
#   or
#   curl -fsSL https://raw.githubusercontent.com/YOUR_ORG/togather/main/deploy/scripts/fix-caddy-staging.sh | sudo bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root or with sudo"
    exit 1
fi

log_info "Fixing Caddy proxy on staging server..."

# 1. Verify Caddy is installed
if ! command -v caddy &> /dev/null; then
    log_error "Caddy is not installed. Run provision-server.sh first."
    exit 1
fi
log_info "✓ Caddy installed: $(caddy version | head -1)"

# 2. Check if app directory exists
APP_DIR="/opt/togather"
if [[ ! -d "$APP_DIR" ]]; then
    log_error "Application directory not found: $APP_DIR"
    log_error "Run install.sh first to deploy the application"
    exit 1
fi

# 3. Deploy staging Caddyfile
CADDYFILE_SOURCE="${APP_DIR}/deploy/config/environments/Caddyfile.staging"
if [[ ! -f "$CADDYFILE_SOURCE" ]]; then
    log_error "Staging Caddyfile not found at: $CADDYFILE_SOURCE"
    exit 1
fi

log_info "Deploying staging Caddyfile to /etc/caddy/Caddyfile..."
cp "$CADDYFILE_SOURCE" /etc/caddy/Caddyfile
log_info "✓ Caddyfile deployed"

# 4. Create log directory and file
log_info "Creating Caddy log directory..."
mkdir -p /var/log/caddy
touch /var/log/caddy/staging.toronto.log
chown -R caddy:caddy /var/log/caddy
chmod 750 /var/log/caddy
chmod 644 /var/log/caddy/staging.toronto.log
log_info "✓ Log directory created"

# 5. Validate Caddyfile
log_info "Validating Caddyfile syntax..."
if caddy validate --config /etc/caddy/Caddyfile 2>&1 | grep -q "Valid"; then
    log_info "✓ Caddyfile is valid"
else
    log_error "Caddyfile validation failed"
    cat /etc/caddy/Caddyfile
    exit 1
fi

# 6. Enable and start Caddy service
log_info "Starting Caddy service..."
systemctl enable caddy
systemctl stop caddy 2>/dev/null || true
sleep 1
systemctl start caddy

# Wait for Caddy to start
sleep 3

if systemctl is-active --quiet caddy; then
    log_info "✓ Caddy service is running"
else
    log_error "Caddy service failed to start"
    systemctl status caddy --no-pager
    exit 1
fi

# 7. Test traffic routing
log_info "Testing traffic routing..."
RESPONSE=$(curl -s -I http://localhost/health 2>&1 || echo "FAILED")
if echo "$RESPONSE" | grep -q "HTTP/1.1 200"; then
    log_info "✓ Health check passed"
    
    # Check which slot is active
    ACTIVE_SLOT=$(echo "$RESPONSE" | grep -i "X-Togather-Slot" | awk '{print $2}' | tr -d '\r\n' || echo "unknown")
    log_info "✓ Active slot: ${ACTIVE_SLOT}"
else
    log_warn "Health check failed (may need to wait for app to start)"
    log_warn "Response: $RESPONSE"
fi

# 8. Show status
echo ""
log_info "=== Caddy Status ==="
systemctl status caddy --no-pager | head -15

echo ""
log_info "=== Next Steps ==="
echo "1. Verify external access: curl -I https://staging.toronto.togather.foundation/health"
echo "2. Check logs: sudo journalctl -u caddy -f"
echo "3. Check Caddy logs: sudo tail -f /var/log/caddy/staging.toronto.log"

log_info "✓ Caddy proxy fix complete!"
