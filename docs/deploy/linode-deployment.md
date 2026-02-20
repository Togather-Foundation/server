# Linode Deployment Guide

**Deploy Togather SEL Server to a Linode VM**

This guide walks you through deploying the Togather server to a fresh Linode VM from scratch.

---

## Table of Contents

1. [Linode VM Setup](#linode-vm-setup)
2. [Server Preparation](#server-preparation)
3. [Install Dependencies](#install-dependencies)
4. [Deploy Application](#deploy-application)
5. [Configure Domain & SSL](#configure-domain--ssl-optional)
6. [Monitor & Maintain](#monitor--maintain)
7. [Cost Estimation](#cost-estimation)

---

## Linode VM Setup

### Recommended Specs

**For Production (Light Traffic):**
- **Plan**: Linode 4GB ($24/month)
- **CPU**: 2 vCPUs
- **RAM**: 4GB
- **Storage**: 80GB SSD
- **Transfer**: 4TB

**For Development/Testing:**
- **Plan**: Linode 2GB ($12/month) - Adequate for testing
- **CPU**: 1 vCPU
- **RAM**: 2GB
- **Storage**: 50GB SSD
- **Transfer**: 2TB

### Create Your Linode

1. **Log into Linode Cloud Manager**: https://cloud.linode.com

2. **Create a new Linode**:
   - Click "Create" → "Linode"
   - **Distribution**: Ubuntu 24.04 LTS (recommended)
   - **Region**: Choose closest to your users
   - **Linode Plan**: Select based on specs above
   - **Linode Label**: `togather-server` (or your preferred name)
   - **Root Password**: Set a strong password (you'll use SSH keys later)
   - **SSH Keys**: Add your public key if you have one
   - Click **"Create Linode"**

3. **Wait for provisioning** (1-2 minutes)

4. **Note your IP address** - You'll see it in the dashboard

---

## Server Preparation

### Quick Start: Automated Provisioning (Recommended)

The fastest way to set up your server is using our automated provisioning script.

#### Option 1: Run from Your Local Machine (Easiest)

```bash
# From your local machine (in the repo directory)
cd deploy/scripts
./provision-remote.sh root@192.46.222.199

# Or with custom Go version
./provision-remote.sh root@192.46.222.199 1.25.0

# Or with custom deploy username
./provision-remote.sh root@192.46.222.199 1.24.13 togather

# Or using SSH config alias
./provision-remote.sh togather-root
```

This script will SSH into your server, upload the provisioning script, and execute it remotely.

#### Option 2: Run Directly on Server

```bash
# SSH into your Linode as root
ssh root@YOUR_LINODE_IP

# Download and run the provisioning script
curl -fsSL https://raw.githubusercontent.com/Togather-Foundation/server/001-sel-backend/deploy/scripts/provision-server.sh | bash

# Or with environment variables
GO_VERSION=1.25.0 bash <(curl -fsSL https://raw.githubusercontent.com/Togather-Foundation/server/001-sel-backend/deploy/scripts/provision-server.sh)
```

#### Option 3: Clone Repo First, Then Provision

```bash
ssh root@YOUR_LINODE_IP
git clone https://github.com/Togather-Foundation/server.git
cd server/deploy/scripts

# Run with defaults
sudo ./provision-server.sh

# Or customize
GO_VERSION=1.25.0 DEPLOY_USER=togather sudo ./provision-server.sh
```

**Customization via environment variables:**

```bash
# Install a specific Go version
GO_VERSION=1.25.0 sudo ./provision-server.sh

# Use a different deploy username
DEPLOY_USER=togather sudo ./provision-server.sh

# Skip SSH hardening prompt (for automation)
SKIP_SSH_HARDEN=true GO_VERSION=1.24.13 sudo ./provision-server.sh
```

**This script will:**
- Update system packages
- Install Docker, Docker Compose, and Go (configurable version, default: 1.24.13)
- Create a 'deploy' user with sudo access (configurable username)
- Configure firewall (UFW) - allow SSH, HTTP, HTTPS
- Set up fail2ban for SSH protection
- Harden SSH (disable root login, password auth)
- Configure system limits and swap space

After the script completes, reconnect as the deploy user:

```bash
ssh deploy@YOUR_LINODE_IP
```

---

### Manual Setup (Alternative)

If you prefer manual setup or the automated script doesn't work for your environment:

<details>
<summary>Click to expand manual setup instructions</summary>

#### 1. SSH into Your Linode

```bash
# Replace with your Linode's IP address
ssh root@YOUR_LINODE_IP
```

#### 2. Initial Server Hardening

```bash
# Update system packages
apt update && apt upgrade -y

# Create a non-root user (replace 'deploy' with your preferred username)
adduser deploy
usermod -aG sudo deploy

# Set up SSH key authentication for the new user
mkdir -p /home/deploy/.ssh
cp /root/.ssh/authorized_keys /home/deploy/.ssh/
chown -R deploy:deploy /home/deploy/.ssh
chmod 700 /home/deploy/.ssh
chmod 600 /home/deploy/.ssh/authorized_keys

# Disable root SSH login (optional but recommended)
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
systemctl restart ssh

# Set up firewall
ufw allow OpenSSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw --force enable

# Exit and reconnect as the new user
exit
```

#### 3. Reconnect as Your New User

```bash
ssh deploy@YOUR_LINODE_IP
```

#### 4. Install Docker & Docker Compose

```bash
# Install Docker
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER

# Install Docker Compose v2
sudo apt-get update
sudo apt-get install -y docker-compose-plugin

# Log out and back in for group changes to take effect
exit
ssh deploy@YOUR_LINODE_IP

# Verify installations
docker --version        # Should show Docker version 27.x+
docker compose version  # Should show Docker Compose version 2.x+
```

#### 5. Install Go (if building from source)

```bash
# Download and install Go 1.24.13
GO_VERSION="1.24.13"
wget "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export PATH=$PATH:~/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version  # Should show: go version go1.24.13 linux/amd64
```

</details>
```

---

## Deploy Application

### 1. Clone the Repository

```bash
cd ~
git clone https://github.com/Togather-Foundation/server.git
cd server
```

### 2. Build the Server

```bash
# Install build tools
make install-tools

# Build the binary
make build

# Verify it works
./server version
```

### 3. Set Up Environment Configuration

**Option A: Interactive Setup (Recommended for First Time)**

```bash
# Run interactive setup - it will guide you through everything
./server setup --docker

# This will:
# - Generate secure secrets (JWT, CSRF, admin password)
# - Create .env file with all configuration
# - Set up database configuration
# - Create your first API key
```

**Option B: Manual Configuration**

```bash
# Copy example environment file
cp .env.example .env

# Edit with your settings
nano .env
```

**Required environment variables:**

```bash
# Environment
ENVIRONMENT=production

# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
BASE_URL=http://YOUR_LINODE_IP:8080  # Change to your domain later

# Database (Docker PostgreSQL)
DATABASE_URL=postgres://togather:CHANGE_THIS_PASSWORD@togather-db:5432/togather?sslmode=disable

# Security (generate secure random strings)
JWT_SECRET=<generate-32+-char-random-string>
CSRF_KEY=<generate-32+-char-random-string>

# CORS (add your domain)
CORS_ALLOWED_ORIGINS=http://YOUR_LINODE_IP:8080,https://yourdomain.com

# Admin
ADMIN_USERNAME=admin
ADMIN_PASSWORD=<generate-strong-password>

# Optional: Monitoring
METRICS_ENABLED=true
```

**Generate secure secrets:**

```bash
# Generate JWT_SECRET
openssl rand -base64 32

# Generate CSRF_KEY
openssl rand -base64 32

# Generate admin password
openssl rand -base64 24
```

### 4. Start the Application with Docker Compose

```bash
cd ~/server

# Start database + application
docker compose -f deploy/docker/docker-compose.yml up -d

# Check status
docker compose -f deploy/docker/docker-compose.yml ps

# View logs
docker compose -f deploy/docker/docker-compose.yml logs -f app

# Wait for startup (20-30 seconds)
```

### 5. Run Database Migrations

```bash
# The migrations should run automatically, but if needed:
export DATABASE_URL="postgres://togather:YOUR_PASSWORD@localhost:5433/togather?sslmode=disable"

make migrate-up
make migrate-river
```

### 6. Verify Deployment

```bash
# Check health
curl http://localhost:8080/health | jq

# Expected output:
# {
#   "status": "healthy",
#   "checks": {
#     "database": { "status": "pass" },
#     "job_queue": { "status": "pass" }
#   }
# }

# Test externally (from your local machine)
curl http://YOUR_LINODE_IP:8080/health | jq
```

---

## Configure Domain & SSL (Optional)

For production deployments, use a reverse proxy with SSL. **Caddy is recommended** for automatic SSL certificate management.

### Option 1: Caddy (Recommended)

Caddy automatically obtains and renews SSL certificates from Let's Encrypt.

```bash
# Install Caddy
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy

# Create Caddyfile
sudo nano /etc/caddy/Caddyfile
```

**Caddyfile (automatic HTTPS):**

```caddyfile
your-domain.com {
    reverse_proxy localhost:8080
    
    # Optional: Add logging
    log {
        output file /var/log/caddy/access.log
    }
}
```

**Enable and verify:**

```bash
# Create log directory
sudo mkdir -p /var/log/caddy
sudo chown caddy:caddy /var/log/caddy

# Reload Caddy (it will automatically get SSL cert!)
sudo systemctl reload caddy

# Check logs for certificate acquisition
sudo journalctl -u caddy -f

# Update firewall (no longer need port 8080 exposed)
sudo ufw delete allow 8080/tcp

# Update .env to use HTTPS
cd ~/server
nano .env
# Change: BASE_URL=https://your-domain.com
# Change: CORS_ALLOWED_ORIGINS=https://your-domain.com

# Restart application
docker compose -f deploy/docker/docker-compose.yml restart app
```

**See [Caddy Guide](caddy.md) for production blue-green deployments.**

### Option 2: Caddy (Recommended)

Use Caddy for automatic HTTPS and simpler config. See [Caddy Guide](caddy.md).

---

## Monitor & Maintain

### Access Monitoring Dashboards

The application includes Prometheus and Grafana:

```bash
# Grafana: http://YOUR_LINODE_IP:3000
# Default login: admin / admin (change on first login)

# Prometheus: http://YOUR_LINODE_IP:9090
```

**Note:** For security, these should only be accessible via SSH tunnel in production:

```bash
# From your local machine, create SSH tunnel
ssh -L 3000:localhost:3000 -L 9090:localhost:9090 togather@YOUR_LINODE_IP

# Then access:
# - Grafana: http://localhost:3000
# - Prometheus: http://localhost:9090
```

### Regular Maintenance

**Database Snapshots:**

```bash
cd ~/server

# Create snapshot
./server snapshot create --reason "pre-deployment"

# List snapshots
./server snapshot list

# Cleanup old snapshots (keep last 7 days)
./server snapshot cleanup --retention-days 7
```

**Updates:**

```bash
cd ~/server

# Pull latest code
git pull origin main  # or your branch

# Rebuild
make build

# Restart
docker compose -f deploy/docker/docker-compose.yml restart app

# Or full rebuild:
docker compose -f deploy/docker/docker-compose.yml down
docker compose -f deploy/docker/docker-compose.yml build
docker compose -f deploy/docker/docker-compose.yml up -d
```

**View Logs:**

```bash
# Application logs
docker compose -f deploy/docker/docker-compose.yml logs -f app

# Database logs
docker compose -f deploy/docker/docker-compose.yml logs -f togather-db

# All services
docker compose -f deploy/docker/docker-compose.yml logs -f
```

**Cleanup Old Artifacts:**

```bash
# Dry run to see what would be deleted
./server cleanup --dry-run

# Actually cleanup (interactive)
./server cleanup

# Force cleanup without prompts
./server cleanup --force
```

---

## Cost Estimation

### Monthly Costs (Linode)

| Resource | Spec | Cost |
|----------|------|------|
| **Linode 2GB** (Dev/Test) | 1 vCPU, 2GB RAM, 50GB SSD | **$12/month** |
| **Linode 4GB** (Production) | 2 vCPU, 4GB RAM, 80GB SSD | **$24/month** |
| **Linode 8GB** (High Traffic) | 4 vCPU, 8GB RAM, 160GB SSD | **$48/month** |

**Additional costs:**
- **Domain name**: ~$12-15/year (optional)
- **Backups**: Linode Backup Service $2-10/month (optional - you have snapshot command)
- **Block storage**: $0.10/GB/month (optional, if you need more storage)

**Total estimated cost:**
- **Development**: $12-13/month
- **Production (small)**: $24-26/month
- **Production (with domain)**: $25-27/month

---

## Quick Reference

### Essential Commands

```bash
# Server management
cd ~/server
./server version                    # Check version
./server healthcheck http://localhost:8080/health  # Health check

# Docker management
docker compose -f deploy/docker/docker-compose.yml ps      # Status
docker compose -f deploy/docker/docker-compose.yml logs -f # Logs
docker compose -f deploy/docker/docker-compose.yml restart # Restart
docker compose -f deploy/docker/docker-compose.yml down    # Stop
docker compose -f deploy/docker/docker-compose.yml up -d   # Start

# Database snapshots
./server snapshot create --reason "backup"
./server snapshot list
./server snapshot cleanup --retention-days 7

# Deployment management
./server deploy status
./server cleanup --dry-run

# API key management
./server api-key create my-app
./server api-key list
```

### Troubleshooting

**Container won't start:**
```bash
docker compose -f deploy/docker/docker-compose.yml logs app
# Check .env configuration
# Check DATABASE_URL is correct
```

**Can't connect to database:**
```bash
docker compose -f deploy/docker/docker-compose.yml logs togather-db
# Ensure database container is healthy
docker compose -f deploy/docker/docker-compose.yml ps
```

**Port already in use:**
```bash
# Check what's using port 8080
sudo lsof -i :8080
# Kill process or change SERVER_PORT in .env
```

**Out of disk space:**
```bash
# Check disk usage
df -h

# Clean up old Docker images
./server cleanup --images-only --force

# Clean up Docker system
docker system prune -a
```

---

## Next Steps

1. **Set up monitoring alerts** - Configure Grafana alerts for critical metrics
2. **Configure backups** - Set up automated snapshots with `cron`
3. **Set up CI/CD** - Automate deployments (see `docs/deploy/ci-cd.md`)
4. **Performance tuning** - See `docs/deploy/best-practices.md`
5. **Federation setup** - Connect to other SEL nodes

---

## Support

- **Documentation**: `/docs/deploy/`
- **Troubleshooting**: `/docs/deploy/troubleshooting.md`
- **GitHub Issues**: https://github.com/Togather-Foundation/server/issues

---

**Last Updated:** 2026-02-20
