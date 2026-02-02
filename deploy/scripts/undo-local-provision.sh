#!/usr/bin/env bash
# Undo local provisioning damage
# Run with: sudo ./undo-local-provision.sh

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run with sudo"
    exit 1
fi

log_info "Cleaning up local provisioning changes..."
echo ""

# 1. Remove system limits additions
if grep -q "Togather server limits" /etc/security/limits.conf 2>/dev/null; then
    log_info "Removing system limits configuration..."
    sed -i '/# Togather server limits/,/\* hard nproc 32768/d' /etc/security/limits.conf
    log_info "✓ System limits cleaned"
else
    log_info "System limits already clean"
fi

# 2. Remove swap file if created
if swapon --show | grep -q /swapfile; then
    log_info "Removing swap file..."
    swapoff /swapfile
    rm -f /swapfile
    sed -i '\|/swapfile none swap|d' /etc/fstab
    sed -i '/vm.swappiness=10/d' /etc/sysctl.conf
    log_info "✓ Swap file removed"
else
    log_info "No swap file found"
fi

# 3. Remove ENVIRONMENT from /etc/environment
if grep -q "^ENVIRONMENT=" /etc/environment 2>/dev/null; then
    log_info "Removing ENVIRONMENT from /etc/environment..."
    sed -i '/^ENVIRONMENT=/d' /etc/environment
    log_info "✓ ENVIRONMENT removed"
else
    log_info "ENVIRONMENT not set in /etc/environment"
fi

# 4. Remove deploy user
if id deploy &>/dev/null; then
    log_info "Removing deploy user..."
    userdel -r deploy 2>/dev/null || true
    rm -f /etc/sudoers.d/deploy
    log_info "✓ Deploy user removed"
else
    log_info "Deploy user not found"
fi

echo ""
log_info "════════════════════════════════════════════════════"
log_info "✓ Cleanup complete"
log_info "════════════════════════════════════════════════════"
echo ""
log_info "What was cleaned:"
log_info "  - System limits configuration removed"
log_info "  - Swap file removed (if it existed)"
log_info "  - ENVIRONMENT variable removed"
log_info "  - Deploy user removed (if it existed)"
echo ""
log_info "What was left alone (as requested):"
log_info "  - fail2ban (already running)"
log_info "  - UFW firewall (already configured)"
log_info "  - Upgraded packages"
echo ""
log_info "All done! Your system is back to normal."
echo ""
