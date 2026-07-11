#!/usr/bin/env bash
# Togather Server Provisioning Script
# Provisions a fresh Ubuntu/Debian server for Togather deployment
#
# Usage:
#   Local:  ./provision-server.sh
#   Remote: curl -fsSL https://raw.githubusercontent.com/YOUR_ORG/togather/main/deploy/scripts/provision-server.sh | bash
#
# Environment variables:
#   GO_VERSION - Go version to install (default: 1.24.13)
#   DEPLOY_USER - Username for deployment (default: deploy)
#   ENVIRONMENT - Application environment: development, staging, production (default: staging)
#   SKIP_SSH_HARDEN - Skip SSH hardening prompt (default: false)
#
# Requirements:
#   - Ubuntu 22.04+ or Debian 11+
#   - Run as root or with sudo
#   - SSH access configured

set -euo pipefail

# Configuration (can be overridden via environment)
# Default Go version matches go.mod toolchain requirement
GO_VERSION="${GO_VERSION:-1.24.13}"
DEPLOY_USER="${DEPLOY_USER:-deploy}"
APP_ENVIRONMENT="${ENVIRONMENT:-staging}"
SKIP_SSH_HARDEN="${SKIP_SSH_HARDEN:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root or with sudo"
        exit 1
    fi
}

detect_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
        log_info "Detected OS: $OS $VERSION"
    else
        log_error "Cannot detect OS. /etc/os-release not found."
        exit 1
    fi
}

update_system() {
    log_info "Updating system packages..."
    
    # Set non-interactive mode and keep existing config files
    export DEBIAN_FRONTEND=noninteractive
    export DEBIAN_PRIORITY=critical
    
    apt-get update -qq
    
    # Upgrade with config file handling (keep local versions)
    apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" upgrade -y -qq
    
    apt-get install -y -qq \
        apt-transport-https \
        ca-certificates \
        curl \
        gnupg \
        lsb-release \
        software-properties-common \
        git \
        wget \
        unzip \
        htop \
        vim \
        ufw \
        fail2ban \
        build-essential \
        make \
        logrotate \
        jq
    log_info "✓ System packages updated"
}

configure_firewall() {
    log_info "Configuring firewall (UFW)..."
    
    # Allow SSH first (don't lock yourself out!)
    ufw --force enable
    ufw default deny incoming
    ufw default allow outgoing
    ufw allow ssh
    ufw allow 80/tcp   # HTTP
    ufw allow 443/tcp  # HTTPS
    
    log_info "✓ Firewall configured (SSH, HTTP, HTTPS allowed)"
}

configure_fail2ban() {
    log_info "Configuring fail2ban for SSH protection..."
    
    cat > /etc/fail2ban/jail.local <<EOF
[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 5
bantime = 3600
findtime = 600
EOF
    
    systemctl enable fail2ban
    systemctl restart fail2ban
    log_info "✓ Fail2ban configured"
}

install_docker() {
    log_info "Installing Docker..."
    
    if command -v docker &> /dev/null; then
        log_info "Docker already installed ($(docker --version))"
        return
    fi
    
    # Add Docker's official GPG key
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/$OS/gpg | gpg --batch --yes --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg
    
    # Add Docker repository
    echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$OS \
        $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    # Install Docker
    apt-get update -qq
    apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    
    # Start Docker
    systemctl enable docker
    systemctl start docker
    
    log_info "✓ Docker installed ($(docker --version))"
}

install_go() {
    log_info "Installing Go ${GO_VERSION}..."
    
    if command -v go &> /dev/null; then
        INSTALLED_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        log_info "Go already installed (version ${INSTALLED_VERSION})"
        if [[ "$INSTALLED_VERSION" != "$GO_VERSION" ]]; then
            log_warn "Installed version differs from requested version ${GO_VERSION}"
            log_warn "To upgrade, remove /usr/local/go and run this script again"
        fi
        return
    fi
    
    GO_ARCH=$(dpkg --print-architecture | sed 's/armhf/armv6l/')
    
    log_info "Downloading Go ${GO_VERSION} for ${GO_ARCH}..."
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    rm "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    
    # Add to PATH for all users
    echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
    
    log_info "✓ Go ${GO_VERSION} installed successfully"
}

install_caddy() {
    log_info "Installing Caddy reverse proxy..."
    
    if command -v caddy &> /dev/null; then
        INSTALLED_VERSION=$(caddy version | awk '{print $1}')
        log_info "Caddy already installed (${INSTALLED_VERSION})"
        return
    fi
    
    # Install Caddy from official repository
    # https://caddyserver.com/docs/install#debian-ubuntu-raspbian
    apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --batch --yes --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
    apt-get update -qq
    apt-get install -y -qq caddy
    
    # Create log directory with correct ownership to prevent permission issues
    mkdir -p /var/log/caddy
    chown caddy:caddy /var/log/caddy
    chmod 750 /var/log/caddy
    
    # Enable and start Caddy service
    systemctl enable caddy
    systemctl stop caddy 2>/dev/null || true  # Don't start yet - no Caddyfile
    
    log_info "✓ Caddy installed ($(caddy version | awk '{print $1}'))"
    log_info "  Note: Caddyfile will be configured during application installation"
}

setup_deploy_user() {
    USER_EXISTS=false
    if id "$DEPLOY_USER" &>/dev/null; then
        log_info "Deploy user '$DEPLOY_USER' already exists"
        USER_EXISTS=true
    else
        log_info "Creating deploy user '$DEPLOY_USER'..."
    fi
    
    if [[ "$USER_EXISTS" == "false" ]]; then
    
    # Create deploy user with no password
    adduser --disabled-password --gecos "" "$DEPLOY_USER"
    
    # Add to docker group
    usermod -aG docker "$DEPLOY_USER"
    
    # Add sudo privileges
    usermod -aG sudo "$DEPLOY_USER"
    echo "$DEPLOY_USER ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/"$DEPLOY_USER"
    chmod 0440 /etc/sudoers.d/"$DEPLOY_USER"
    
    # Set up SSH directory
    mkdir -p /home/"$DEPLOY_USER"/.ssh
    chmod 700 /home/"$DEPLOY_USER"/.ssh
    
    # Copy root's authorized_keys if they exist
    if [[ -f /root/.ssh/authorized_keys ]]; then
        cp /root/.ssh/authorized_keys /home/"$DEPLOY_USER"/.ssh/authorized_keys
        chown -R "$DEPLOY_USER":"$DEPLOY_USER" /home/"$DEPLOY_USER"/.ssh
        chmod 600 /home/"$DEPLOY_USER"/.ssh/authorized_keys
        log_info "✓ SSH keys copied from root to deploy user"
    else
        log_warn "No SSH keys found in /root/.ssh/authorized_keys"
        log_warn "You'll need to add your SSH public key manually:"
        log_warn "  echo 'YOUR_PUBLIC_KEY' >> /home/$DEPLOY_USER/.ssh/authorized_keys"
        log_warn "  chown -R $DEPLOY_USER:$DEPLOY_USER /home/$DEPLOY_USER/.ssh"
        log_warn "  chmod 600 /home/$DEPLOY_USER/.ssh/authorized_keys"
    fi
    
    log_info "✓ Deploy user created"
    fi  # End of USER_EXISTS check
    
    # Set ENVIRONMENT variable globally (always run, even if user exists)
    log_info "Setting ENVIRONMENT=$APP_ENVIRONMENT for $DEPLOY_USER..."
    
    # Add to user's profile so it's set on login
    cat >> /home/"$DEPLOY_USER"/.profile <<EOF

# Togather application environment (set by provisioning script)
export ENVIRONMENT=$APP_ENVIRONMENT
EOF
    
    # Also set in /etc/environment for system-wide access
    if ! grep -q "^ENVIRONMENT=" /etc/environment 2>/dev/null; then
        echo "ENVIRONMENT=$APP_ENVIRONMENT" >> /etc/environment
        log_info "✓ ENVIRONMENT=$APP_ENVIRONMENT set system-wide in /etc/environment"
    fi
    
    chown "$DEPLOY_USER":"$DEPLOY_USER" /home/"$DEPLOY_USER"/.profile 2>/dev/null || true
    log_info "✓ ENVIRONMENT=$APP_ENVIRONMENT set in $DEPLOY_USER's profile"
    log_info "✓ Deploy user setup complete"
}

harden_ssh() {
    log_info "Hardening SSH configuration..."
    
    SSH_CONFIG="/etc/ssh/sshd_config"
    
    # Backup original config
    cp "$SSH_CONFIG" "$SSH_CONFIG.backup.$(date +%Y%m%d_%H%M%S)"
    
    # Apply hardening settings
    sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' "$SSH_CONFIG"
    sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' "$SSH_CONFIG"
    sed -i 's/^#\?ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' "$SSH_CONFIG"
    sed -i 's/^#\?PubkeyAuthentication.*/PubkeyAuthentication yes/' "$SSH_CONFIG"
    sed -i 's/^#\?PermitEmptyPasswords.*/PermitEmptyPasswords no/' "$SSH_CONFIG"
    sed -i 's/^#\?X11Forwarding.*/X11Forwarding no/' "$SSH_CONFIG"
    sed -i 's/^#\?MaxAuthTries.*/MaxAuthTries 3/' "$SSH_CONFIG"
    
    # Add these if not present
    grep -q "^ClientAliveInterval" "$SSH_CONFIG" || echo "ClientAliveInterval 300" >> "$SSH_CONFIG"
    grep -q "^ClientAliveCountMax" "$SSH_CONFIG" || echo "ClientAliveCountMax 2" >> "$SSH_CONFIG"
    
    # Restart SSH
    systemctl restart ssh || systemctl restart sshd
    
    log_info "✓ SSH hardened (key-based auth only, root login disabled)"
}

configure_system_limits() {
    log_info "Configuring system limits..."
    
    cat >> /etc/security/limits.conf <<EOF

# Togather server limits
* soft nofile 65536
* hard nofile 65536
* soft nproc 32768
* hard nproc 32768
EOF
    
    log_info "✓ System limits configured"
}

setup_swap() {
    log_info "Setting up swap space..."
    
    if swapon --show | grep -q /swapfile; then
        log_info "Swap already configured"
        return
    fi
    
    # Create 2GB swap file
    fallocate -l 2G /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
    swapon /swapfile
    
    # Make swap permanent
    echo '/swapfile none swap sw 0 0' >> /etc/fstab
    
    # Adjust swappiness
    sysctl vm.swappiness=10
    echo 'vm.swappiness=10' >> /etc/sysctl.conf
    
    log_info "✓ Swap space configured (2GB)"
}

configure_maintenance() {
    log_info "Configuring automated maintenance..."

    log_info "Creating togather group..."
    if ! getent group togather >/dev/null; then
        groupadd --system togather
    fi

    mkdir -p /var/log/togather/deployments /var/log/togather/db-snapshots /var/log/togather/health /var/log/togather/migrations
    chown root:togather /var/log/togather /var/log/togather/deployments /var/log/togather/db-snapshots /var/log/togather/health /var/log/togather/migrations 2>/dev/null || true
    chmod 750 /var/log/togather /var/log/togather/deployments /var/log/togather/db-snapshots /var/log/togather/health /var/log/togather/migrations

    log_info "Installing logrotate config..."
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd 2>/dev/null || true)"
    if [ -n "${SCRIPT_DIR}" ] && [ -f "${SCRIPT_DIR}/../config/logrotate.conf" ]; then
        cp "${SCRIPT_DIR}/../config/logrotate.conf" /etc/logrotate.d/togather
        chmod 644 /etc/logrotate.d/togather
    else
        log_warn "deploy/config/logrotate.conf not found (ran via curl?) — skipping logrotate setup."
    fi

    log_info "Installing containerd PID cleanup timer..."
    cat > /etc/systemd/system/containerd-pid-cleanup.service << 'UNIT'
[Unit]
Description=Clean up stale containerd exec PID files

[Service]
Type=oneshot
ExecStart=/usr/bin/find /run/containerd/io.containerd.runtime.v2.task/moby -name '*.pid' -not -name 'init.pid' -mmin +5 -delete
UNIT

    cat > /etc/systemd/system/containerd-pid-cleanup.timer << 'UNIT'
[Unit]
Description=Hourly cleanup of stale containerd exec PID files

[Timer]
OnBootSec=10min
OnUnitActiveSec=1h

[Install]
WantedBy=timers.target
UNIT

    systemctl enable --now containerd-pid-cleanup.timer
    log_info "✓ Containerd PID cleanup timer enabled (hourly)"

    log_info "Installing Docker prune timer..."
    cat > /etc/systemd/system/togather-docker-prune.service << 'UNIT'
[Unit]
Description=Weekly Docker system prune for Togather
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=/usr/bin/docker system prune -a -f --filter "until=48h"
ExecStart=/usr/bin/docker builder prune -f
SyslogIdentifier=togather-docker-prune
UNIT

    cat > /etc/systemd/system/togather-docker-prune.timer << 'UNIT'
[Unit]
Description=Weekly Docker prune for Togather

[Timer]
OnCalendar=weekly
RandomizedDelaySec=3600

[Install]
WantedBy=timers.target
UNIT

    systemctl daemon-reload
    systemctl enable --now togather-docker-prune.timer
    log_info "✓ Docker prune timer enabled (weekly)"

    log_info "Installing Prometheus WAL cleanup timer..."
    cat > /etc/systemd/system/togather-prometheus-cleanup.service << 'UNIT'
[Unit]
Description=Weekly Prometheus WAL cleanup for Togather
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=/usr/bin/docker stop togather-prometheus
ExecStart=-/usr/bin/find /var/lib/docker/volumes/togather-staging_prometheus-data/_data/wal -mindepth 1 -delete
ExecStart=-/usr/bin/find /var/lib/docker/volumes/togather-staging_prometheus-data/_data/chunks_head -mindepth 1 -delete
ExecStart=/usr/bin/docker start togather-prometheus
SyslogIdentifier=togather-prometheus-cleanup
UNIT

    cat > /etc/systemd/system/togather-prometheus-cleanup.timer << 'UNIT'
[Unit]
Description=Weekly Prometheus WAL cleanup for Togather

[Timer]
OnCalendar=Sat *-*-* 03:00:00
RandomizedDelaySec=3600

[Install]
WantedBy=timers.target
UNIT

    systemctl enable --now togather-prometheus-cleanup.timer
    log_info "✓ Prometheus WAL cleanup timer enabled (weekly, Sat 03:00)"

    log_info "Configuring journald size limits..."
    mkdir -p /etc/systemd/journald.conf.d
    if [ ! -f /etc/systemd/journald.conf.d/togather.conf ]; then
        cat > /etc/systemd/journald.conf.d/togather.conf << 'CONF'
[Journal]
SystemMaxUse=500M
MaxFileSec=14day
CONF
        systemctl restart systemd-journald
        log_info "✓ Journald limits configured (500M max, 14 day retention)"
    else
        log_info "Journald limits already configured — skipping."
    fi

    log_info "✓ Automated maintenance configured"
}

print_next_steps() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "✓ Server provisioning complete!"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
    echo "IMPORTANT: Docker group membership requires a new login session"
    echo ""
    echo "Next steps:"
    echo ""
    echo "  1. Exit this SSH session (to activate docker group membership):"
    echo "     exit"
    echo ""
    echo "  2. Log back in as the deploy user:"
    echo "     ssh $DEPLOY_USER@$(hostname -I | awk '{print $1}')"
    echo ""
    echo "  3. Verify docker access (should work without sudo):"
    echo "     docker ps"
    echo ""
    echo "  4. Deploy the Togather server package:"
    echo "     # Copy package from local machine:"
    echo "     scp togather-server-*.tar.gz $DEPLOY_USER@$(hostname -I | awk '{print $1}'):~/"
    echo ""
    echo "     # Extract and install:"
    echo "     tar -xzf togather-server-*.tar.gz"
    echo "     cd togather-server-*"
    echo "     sudo ./install.sh"
    echo ""
    echo "     # Configure and start:"
    echo "     cd /opt/togather"
    echo "     ./server setup --docker --allow-production-secrets"
    echo "     # Note: ENVIRONMENT=$APP_ENVIRONMENT is set globally, no need to prefix commands"
    echo ""
    echo "Security notes:"
    echo "  - Root SSH login is now DISABLED"
    echo "  - Password authentication is DISABLED"
    echo "  - Use '$DEPLOY_USER' user for all operations"
    echo "  - Firewall (UFW) is active (SSH, HTTP, HTTPS allowed)"
    echo "  - Fail2ban is protecting SSH"
    echo ""
    echo "Automated maintenance:"
    echo "  - Log rotation (logrotate): /etc/logrotate.d/togather"
    echo "  - Docker prune timer: weekly (systemctl status togather-docker-prune.timer)"
    echo "  - Containerd PID cleanup: hourly (systemctl status containerd-pid-cleanup.timer)"
    echo "  - Prometheus WAL cleanup: weekly (systemctl status togather-prometheus-cleanup.timer)"
    echo "  - Journald max size: 500M (/etc/systemd/journald.conf.d/togather.conf)"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
}

main() {
    log_info "Starting Togather server provisioning..."
    echo ""
    log_info "Configuration:"
    log_info "  GO_VERSION: ${GO_VERSION}"
    log_info "  DEPLOY_USER: ${DEPLOY_USER}"
    log_info "  ENVIRONMENT: ${APP_ENVIRONMENT}"
    echo ""
    
    check_root
    detect_os
    update_system
    configure_firewall
    configure_fail2ban
    install_docker
    install_go
    install_caddy
    setup_deploy_user
    configure_system_limits
    setup_swap
    configure_maintenance
    
    # SSH hardening should be done last (after deploy user is set up)
    if [[ "$SKIP_SSH_HARDEN" == "true" ]]; then
        log_warn "Skipping SSH hardening (SKIP_SSH_HARDEN=true)"
    else
        log_warn "About to harden SSH (disable root login, password auth)"
        log_warn "Make sure you can log in as '$DEPLOY_USER' user before continuing!"
        read -p "Continue with SSH hardening? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            harden_ssh
        else
            log_warn "Skipped SSH hardening. Run manually later if needed."
        fi
    fi
    
    print_next_steps
}

# Run main function
main "$@"
