#!/usr/bin/env bash
# Remote Server Provisioning Script
# Provisions a remote server from your local machine via SSH
#
# Usage:
#   ./provision-remote.sh [user@]hostname [GO_VERSION] [DEPLOY_USER]
#
# Examples:
#   ./provision-remote.sh root@192.46.222.199
#   ./provision-remote.sh togather-root  # Using SSH config alias
#   ./provision-remote.sh root@192.46.222.199 1.25.0
#   ./provision-remote.sh root@192.46.222.199 1.24.12 togather

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[LOCAL]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check arguments
if [[ $# -lt 1 ]]; then
    echo "Usage: $0 [user@]hostname [GO_VERSION] [DEPLOY_USER]"
    echo ""
    echo "Examples:"
    echo "  $0 root@192.46.222.199"
    echo "  $0 togather-root"
    echo "  $0 root@192.46.222.199 1.25.0"
    echo "  $0 root@192.46.222.199 1.24.12 togather"
    exit 1
fi

SSH_TARGET="$1"
GO_VERSION="${2:-1.24.12}"
DEPLOY_USER="${3:-deploy}"

log_info "Provisioning remote server: $SSH_TARGET"
log_info "Configuration:"
log_info "  GO_VERSION: $GO_VERSION"
log_info "  DEPLOY_USER: $DEPLOY_USER"
echo ""

# Test SSH connection
log_info "Testing SSH connection..."
if ! ssh -o BatchMode=yes -o ConnectTimeout=5 "$SSH_TARGET" 'exit' 2>/dev/null; then
    log_error "Cannot connect to $SSH_TARGET"
    log_error "Make sure:"
    log_error "  1. The server is running"
    log_error "  2. SSH keys are set up"
    log_error "  3. You can manually connect: ssh $SSH_TARGET"
    exit 1
fi
log_info "✓ SSH connection successful"
echo ""

# Get the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROVISION_SCRIPT="$SCRIPT_DIR/provision-server.sh"

if [[ ! -f "$PROVISION_SCRIPT" ]]; then
    log_error "provision-server.sh not found at: $PROVISION_SCRIPT"
    exit 1
fi

log_info "Uploading and executing provision-server.sh on remote server..."
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Upload script and execute with environment variables
cat "$PROVISION_SCRIPT" | ssh "$SSH_TARGET" "GO_VERSION=$GO_VERSION DEPLOY_USER=$DEPLOY_USER bash -s"

echo ""
echo "═══════════════════════════════════════════════════════════════"
log_info "✓ Remote provisioning complete!"
echo ""
log_info "Next steps:"
log_info "  1. Test SSH access: ssh $DEPLOY_USER@<SERVER_IP>"
log_info "  2. Update your ~/.ssh/config with the $DEPLOY_USER entry"
log_info "  3. SSH in and clone the repo: git clone <repo-url>"
log_info "  4. Run application setup: ./server setup --docker"
