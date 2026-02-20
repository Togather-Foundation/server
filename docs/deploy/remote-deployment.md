# Remote Deployment Guide

This guide covers using `deploy.sh --remote` for zero-downtime deployments to staging and production servers.

## Overview

Remote deployment enables developers to deploy directly from their local machine to remote servers using SSH. The script automatically:

- Clones/updates the git repository on the server
- Builds Docker images for blue-green deployment
- Runs database migrations safely
- Performs health checks
- Switches traffic via Caddy
- Provides automatic rollback on failure

## Prerequisites

### On Local Machine
- Git repository cloned
- SSH access to target server
- Deploy user credentials (deploy keys configured)

### On Remote Server

**Quick Setup (Recommended):**
```bash
# One command to provision server and install application
./deploy/scripts/provision.sh deploy@server staging --with-app
```

**Manual Setup:**

The server must be prepared before first deployment. Either:

**Option A: Use provision.sh --with-app (Fastest)**
```bash
./deploy/scripts/provision.sh deploy@server staging --with-app
```
This provisions the server AND installs the application in one command.

**Option B: Use provision.sh + install.sh (Manual Control)**
```bash
# 1. Provision server infrastructure
./deploy/scripts/provision.sh deploy@server staging

# 2. Install application
make deploy-package
scp dist/togather-server-*.tar.gz deploy@server:~/
ssh deploy@server "cd togather-*/ && sudo ./install.sh"
```

**What must be in place:**
- Docker and Docker Compose installed
- Caddy reverse proxy configured (for blue-green traffic switching)
- SSH access for deploy user with sudo privileges
- `install.sh` run at least once (sets up directories, database, initial deployment)

**After prerequisites are met:**
- `deploy.sh --remote` can be used for all subsequent deployments
- Git repository will be auto-cloned to `/opt/togather/src/` on first deployment
- Blue-green deployments will work automatically

## Basic Usage

### Deploy Current Commit

```bash
cd ~/togather/server
git pull origin main
./deploy/scripts/deploy.sh staging
```

### Deploy Specific Version

```bash
./deploy/scripts/deploy.sh production --version v1.2.3
```

### Deploy Specific Commit

```bash
./deploy/scripts/deploy.sh staging --version abc1234
```

## How It Works

### 1. Local Validation
- Validates local git repository state
- Detects git remote URL (`git remote get-url origin`)
- Resolves target commit/tag to full commit hash

### 2. SSH Connection
- Connects to remote server as specified user
- Checks for git repository at `/opt/togather/src/`

### 3. Repository Setup
- **If repository doesn't exist**: Clones from detected remote URL
- **If repository exists**: Fetches latest changes
- Checks out target commit

### 4. Deployment Execution
- Runs full `deploy.sh` on remote server
- Streams output back to local terminal
- Preserves exit codes

### 5. Blue-Green Deployment (on server)
- Determines inactive slot (blue or green)
- Builds Docker image for inactive slot
- Creates database snapshot
- Runs database migrations
- Deploys to inactive slot
- Runs health checks on new slot
- Switches Caddy traffic to new slot
- Updates deployment state

## Repository Location

Git repository on server: `/opt/togather/src/`

**First deployment** (auto-clone):
```bash
# deploy.sh will automatically run this on first deployment:
sudo mkdir -p /opt/togather/src
sudo chown deploy:deploy /opt/togather/src
git clone git@github.com:Togather-Foundation/server.git /opt/togather/src
```

**Subsequent deployments** (auto-update):
```bash
# deploy.sh will automatically run this before each deployment:
cd /opt/togather/src
git fetch origin
git checkout <target-commit>
```

## Deployment State

State file location: `/opt/togather/src/deploy/config/deployment-state.json`

Tracks:
- Current active slot (blue or green)
- Last deployment ID
- Last deployed commit
- Deployment timestamp
- Deployment history

## Traffic Switching

Caddy configuration: `/etc/caddy/Caddyfile`

**Blue slot active** (port 8081):
```caddyfile
reverse_proxy localhost:8081 {
    header_down X-Togather-Slot "blue"
}
```

**Green slot active** (port 8082):
```caddyfile
reverse_proxy localhost:8082 {
    header_down X-Togather-Slot "green"
}
```

Caddy automatically creates timestamped backups before each configuration change.

## Verification

### Check Active Slot

```bash
curl -I https://staging.toronto.togather.foundation/health | grep -i slot
# x-togather-slot: blue
```

### Check Running Containers

```bash
ssh deploy@server 'docker ps --filter name=togather-'
# togather-blue    Up 5 minutes   0.0.0.0:8081->8080/tcp
# togather-green   Up 30 minutes  0.0.0.0:8082->8080/tcp
```

### Check Deployment State

```bash
ssh deploy@server 'cat /opt/togather/src/deploy/config/deployment-state.json' | jq
```

## Troubleshooting

### SSH Connection Issues

```bash
# Test SSH connection
ssh deploy@server 'echo "Connection successful"'

# Check SSH key
ssh-add -l

# Test git clone
ssh deploy@server 'git ls-remote git@github.com:Togather-Foundation/server.git'
```

### Repository Issues

```bash
# Check repository state on server
ssh deploy@server 'cd /opt/togather/src && git status'

# Avoid force-resetting the repo. Prefer running deploy.sh again or re-cloning if needed.
```

### Deployment Failures

Check deployment logs:
```bash
ssh deploy@server 'ls -la ~/.togather/logs/deployments/'
ssh deploy@server 'tail -100 ~/.togather/logs/deployments/staging_*.log'
```

Check Docker logs:
```bash
ssh deploy@server 'docker logs togather-blue'
ssh deploy@server 'docker logs togather-green'
```

Check Caddy logs:
```bash
ssh deploy@server 'sudo journalctl -u caddy -n 100'
```

## Rollback

Manual rollback to previous slot:

```bash
# Check current active slot
curl -I https://staging.toronto.togather.foundation/health | grep slot

# Edit Caddyfile to switch back
ssh deploy@server 'sudo nano /etc/caddy/Caddyfile'
# Change: reverse_proxy localhost:8082 → localhost:8081

# Reload Caddy
ssh deploy@server 'sudo systemctl reload caddy'
```

Automated rollback is handled by `deploy.sh` when health checks fail.

## Best Practices

1. **Always test on staging first**
   ```bash
   ./deploy.sh staging --remote deploy@staging.server.com
   # Verify, test, then:
   ./deploy.sh production --remote deploy@prod.server.com
   ```

2. **Use version tags for production**
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ./deploy.sh production --remote deploy@prod.server.com --version v1.2.3
   ```

3. **Monitor during deployment**
   - Keep terminal open to see deployment progress
   - Watch metrics/logs in another terminal
   - Have rollback plan ready

4. **Verify after deployment**
   ```bash
   curl https://prod.server.com/health
   curl -I https://prod.server.com/health | grep slot
   ```

## Security

- Uses SSH keys (no passwords)
- Deploy keys with read-only access to repository
- Deploy user has limited sudo permissions (only for Caddy operations)
- All secrets in environment files (never in git)
- Timestamped backups of all configuration changes

## Performance

- Typical deployment time: 2-4 minutes
- Zero downtime throughout deployment
- Database snapshot: ~10-30 seconds (depending on size)
- Docker build: ~1-2 minutes (cached layers speed this up)
- Health checks: ~30 seconds
- Traffic switch: <1 second

## Future Enhancements

- Automated rollback on deployment failure
- Parallel deployments to multiple servers
- Canary deployments (gradual traffic shifting)
- Deployment notifications (Slack, Discord, email)
- Deployment dashboard

## Example Deployment Flow

### Staging Deployment

```bash
# 1. Ensure you're on latest main
cd ~/togather/server
git pull origin main

# 2. Deploy to staging
./deploy/scripts/deploy.sh staging --remote deploy@staging.toronto.togather.foundation

# Output:
# ========================================
# Remote Deployment: staging
# ========================================
# Target server: deploy@staging.toronto.togather.foundation
# Git remote: git@github.com:Togather-Foundation/server.git
# Target commit: abc1234def567890...
# 
# Connecting to remote server...
# ✓ Connected successfully
# 
# Checking for git repository...
# ✓ Repository found at /opt/togather/src
# 
# Fetching latest changes...
# ✓ Repository updated
# 
# Checking out target commit...
# ✓ Checked out abc1234
# 
# Starting deployment on remote server...
# ========================================
# Deployment: staging
# ========================================
# Environment: staging
# Deployment ID: 20260202_174532_abc1234
# Target slot: green (blue is active)
# 
# [1/8] Building Docker image...
# ✓ Image built: togather-server:abc1234
# 
# [2/8] Creating database snapshot...
# ✓ Snapshot created: staging_20260202_174532.sql
# 
# [3/8] Running migrations...
# ✓ Migrations complete
# 
# [4/8] Deploying to green slot...
# ✓ Container started: togather-green
# 
# [5/8] Running health checks...
# ✓ Health check passed
# 
# [6/8] Switching traffic to green...
# ✓ Caddy configuration updated
# ✓ Caddy reloaded
# 
# [7/8] Verifying active slot...
# ✓ Verified: green slot active
# 
# [8/8] Updating deployment state...
# ✓ State updated
# 
# ========================================
# Deployment Complete
# ========================================
# Active slot: green (port 8082)
# Previous slot: blue (port 8081)
# Deployment time: 2m 34s
# 
# Verify: curl -I https://staging.toronto.togather.foundation/health

# 3. Verify deployment
curl -I https://staging.toronto.togather.foundation/health | grep slot
# x-togather-slot: green

# 4. Test the deployment
curl https://staging.toronto.togather.foundation/api/v1/events | jq .
```

### Production Deployment

```bash
# 1. Create and push version tag
git tag v1.2.3
git push origin v1.2.3

# 2. Deploy to production
./deploy/scripts/deploy.sh production --remote deploy@prod.toronto.togather.foundation --version v1.2.3

# 3. Monitor deployment
# (keep this terminal open to watch progress)

# 4. In another terminal, monitor logs
ssh deploy@prod.toronto.togather.foundation 'docker logs -f togather-blue'

# 5. After deployment completes, verify
curl -I https://prod.toronto.togather.foundation/health | grep slot
curl https://prod.toronto.togather.foundation/api/v1/events | jq .

# 6. Monitor for any issues
ssh deploy@prod.toronto.togather.foundation 'docker logs -f togather-blue'
```

## Common Scenarios

### First Time Remote Deployment

```bash
# 1. Ensure install.sh was run on server
ssh deploy@server 'ls -la /opt/togather'

# 2. If not, run install.sh first
make deploy-package
scp dist/togather-server-*.tar.gz deploy@server:~/
ssh deploy@server 'cd ~ && tar -xzf togather-server-*.tar.gz && cd togather-server-* && sudo ./install.sh'

# 3. Then run remote deployment
./deploy/scripts/deploy.sh staging --remote deploy@server
```

### Emergency Rollback

```bash
# 1. Check current slot
curl -I https://prod.server.com/health | grep slot
# x-togather-slot: blue

# 2. Previous slot is green, switch back
ssh deploy@prod.server.com 'sudo sed -i "s/localhost:8081/localhost:8082/" /etc/caddy/Caddyfile && sudo systemctl reload caddy'

# 3. Verify
curl -I https://prod.server.com/health | grep slot
# x-togather-slot: green
```

### Deploy Hotfix

```bash
# 1. Create hotfix branch from production tag
git checkout v1.2.3
git checkout -b hotfix/urgent-fix

# 2. Make fix and commit
# ... make changes ...
git add .
git commit -m "fix: urgent hotfix"

# 3. Push hotfix branch
git push origin hotfix/urgent-fix

# 4. Deploy hotfix to staging
./deploy/scripts/deploy.sh staging --remote deploy@staging.server.com --version hotfix/urgent-fix

# 5. Test thoroughly

# 6. Deploy to production
./deploy/scripts/deploy.sh production --remote deploy@prod.server.com --version hotfix/urgent-fix

# 7. Create hotfix tag
git tag v1.2.4
git push origin v1.2.4

# 8. Merge back to main
git checkout main
git merge hotfix/urgent-fix
git push origin main
```

---

For additional help:
- [Deployment Guide](../../DEPLOY.md) - General deployment guide
- [Troubleshooting](troubleshooting.md) - Common issues
- [Rollback Guide](rollback.md) - Detailed rollback procedures
- [Best Practices](best-practices.md) - Deployment best practices

---

**Last Updated:** 2026-02-20
