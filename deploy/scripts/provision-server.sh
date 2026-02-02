#!/usr/bin/env bash
# Togather Server Provisioning Script
# Provisions a fresh Ubuntu/Debian server for Togather deployment
#
# Usage:
#   Local:  ./provision-server.sh
#   Remote: curl -fsSL https://raw.githubusercontent.com/YOUR_ORG/togather/main/deploy/scripts/provision-server.sh | bash
#
# Environment variables:
#   GO_VERSION - Go version to install (default: 1.24.12)
#   DEPLOY_USER - Username for deployment (default: deploy)
#   SKIP_SSH_HARDEN - Skip SSH hardening prompt (default: false)
#
# Requirements:
#   - Ubuntu 22.04+ or Debian 11+
#   - Run as root or with sudo
#   - SSH access configured

set -euo pipefail

# Configuration (can be overridden via environment)
# Default Go version matches go.mod toolchain requirement
GO_VERSION="${GO_VERSION:-1.24.12}"
DEPLOY_USER="${DEPLOY_USER:-deploy}"
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
        fail2ban
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
    curl -fsSL https://download.docker.com/linux/$OS/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
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

setup_deploy_user() {
    if id "$DEPLOY_USER" &>/dev/null; then
        log_info "Deploy user '$DEPLOY_USER' already exists"
        return
    fi
    
    log_info "Creating deploy user '$DEPLOY_USER'..."
    
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

print_next_steps() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    log_info "Server provisioning complete! 🚀"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
    echo "Installed versions:"
    echo "  - Docker: $(docker --version 2>/dev/null | awk '{print $3}' | sed 's/,//')"
    echo "  - Docker Compose: $(docker compose version 2>/dev/null | awk '{print $4}')"
    echo "  - Go: ${GO_VERSION}"
    echo ""
    echo "Next steps:"
    echo ""
    echo "1. Test SSH access as $DEPLOY_USER user:"
    echo "   ssh $DEPLOY_USER@$(hostname -I | awk '{print $1}')"
    echo ""
    echo "2. Clone the Togather repository:"
    echo "   git clone https://github.com/YOUR_ORG/togather.git"
    echo "   cd togather/server"
    echo ""
    echo "3. Run the application setup:"
    echo "   ./server setup --docker"
    echo ""
    echo "4. Start the application:"
    echo "   docker compose up -d"
    echo ""
    echo "5. Check application health:"
    echo "   ./server healthcheck"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
    echo "Security notes:"
    echo "  - Root SSH login is now DISABLED"
    echo "  - Password authentication is DISABLED"
    echo "  - Use '$DEPLOY_USER' user for all operations"
    echo "  - Firewall (UFW) is active (SSH, HTTP, HTTPS allowed)"
    echo "  - Fail2ban is protecting SSH"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
}

main() {
    log_info "Starting Togather server provisioning..."
    echo ""
    log_info "Configuration:"
    log_info "  GO_VERSION: ${GO_VERSION}"
    log_info "  DEPLOY_USER: ${DEPLOY_USER}"
    echo ""
    
    check_root
    detect_os
    update_system
    configure_firewall
    configure_fail2ban
    install_docker
    install_go
    setup_deploy_user
    configure_system_limits
    setup_swap
    
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
