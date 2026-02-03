# Deployment Guide

This guide covers deploying and upgrading Togather server installations.

## Table of Contents

1. [Which Tool Should I Use?](#which-tool-should-i-use) ⭐
2. [New Installation](#new-installation)
3. [Upgrading Existing Installation](#upgrading-existing-installation)
4. [Deployment Methods](#deployment-methods)
5. [Verification](#verification)
6. [Rollback](#rollback)

---

## Which Tool Should I Use?

Choose the right deployment tool for your scenario:

| Scenario | Tool | Command | Why |
|----------|------|---------|-----|
| **Brand new server** | `provision.sh --with-app` | `./deploy/scripts/provision.sh deploy@server staging --with-app` | One command setup: provisions server + installs app |
| **First install only** | `provision.sh` + `install.sh` | See [New Installation](#new-installation) | Manual control over provisioning and install steps |
| **Simple update (dev)** | `install.sh` | `sudo ./install.sh` | Fast deployment, ~30s downtime OK |
| **Production deployment** | `deploy.sh --remote` | `./deploy/scripts/deploy.sh staging --remote deploy@server` | Zero downtime, automatic health checks + rollback |
| **Rollback** | `deploy.sh` with state | See [Rollback](#rollback) | Uses deployment state to switch slots |

**Key Features:**
- **provision.sh --with-app**: Combines server provisioning and application install in one command
- **install.sh**: Now auto-detects blue-green mode if Caddy is configured; preserves data on existing installs
- **deploy.sh --remote**: Full blue-green deployment from local machine via SSH

**Decision Flow:**
1. **Is this a brand new server?** → Use `provision.sh --with-app` for fastest setup
2. **Already provisioned but no app?** → Use `install.sh` to install the application
3. **Need to update existing installation?**
   - Dev/staging with low traffic? → Use `install.sh` (simple, fast)
   - Production or zero downtime required? → Use `deploy.sh --remote` (blue-green)
4. **Need to rollback?** → Use deployment state or Caddy slot switching

---

## New Installation

For new installations, use the automated installer:

```bash
# On your local machine
make deploy-package
scp dist/togather-server-*.tar.gz deploy@server:~/

# On server
ssh deploy@server
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh
```

See [docs/deploy/quickstart.md](docs/deploy/quickstart.md) for detailed installation guide.

---

## Upgrading Existing Installation

### Method 1: Zero-Downtime (Recommended for Production)

Deploy from your local git repository:

```bash
# From your local machine
cd ~/togather/server
git pull origin main

# Deploy to staging
./deploy/scripts/deploy.sh staging --remote deploy@staging.server.com

# Deploy to production (after testing on staging)
./deploy/scripts/deploy.sh production --remote deploy@prod.server.com
```

**What happens:**
1. Connects to server via SSH
2. Clones/updates git repository at `/opt/togather/src/`
3. Runs blue-green deployment:
   - Builds Docker image for inactive slot
   - Creates database snapshot
   - Runs migrations
   - Deploys to inactive slot
   - Health checks
   - Switches Caddy traffic
4. Zero downtime throughout the process

**Deploy specific version:**
```bash
./deploy/scripts/deploy.sh production --remote deploy@prod.server.com --version v1.2.3
```

**Requirements:**
- SSH access to server
- `install.sh` must have been run once to provision the server
- Git repository access (deploy keys configured)

**See:** [docs/deploy/remote-deployment.md](docs/deploy/remote-deployment.md) for comprehensive guide

### Method 2: Simple Upgrade (Brief Downtime)

Use `install.sh` for simpler upgrades:

```bash
# Copy package to server
scp dist/togather-server-<hash>.tar.gz deploy@server:~/

# On server
ssh deploy@server
cd ~
tar -xzf togather-server-<hash>.tar.gz
cd togather-server-<hash>/
sudo ./install.sh
```

When existing installation detected, choose option **[1] PRESERVE DATA**.

**Characteristics:**
- Brief downtime (~10-30 seconds during restart)
- Simpler than blue-green
- Good for: Development, staging, low-traffic periods

---

## Deployment Methods

### Comparison

| Feature | Simple Deployment | Zero-Downtime |
|---------|------------------|---------------|
| Downtime | ~10-30 seconds | Zero |
| Complexity | Low | Medium |
| Requirements | Deployment package | Git repo + SSH |
| Rollback | Manual | Automatic |
| Use case | Dev/staging | Production |
| Traffic switching | Service restart | Caddy blue-green |

### When to Use Each

**Simple Deployment (`install.sh`):**
- Development environments
- Staging with low traffic
- Scheduled maintenance windows
- First-time installations
- Simple, straightforward upgrades

**Zero-Downtime (`deploy.sh --remote`):**
- Production systems
- High-availability requirements
- Frequent deployments
- Need rollback capability
- Critical services

---

## Verification

### Check Service Status

```bash
# Check systemd service
sudo systemctl status togather-server

# Check health endpoint
curl http://localhost:8080/health

# Check active slot (blue-green deployments)
curl -I https://your-domain.com/health | grep -i slot
```

### Check Logs

```bash
# Service logs
sudo journalctl -u togather-server -n 100 -f

# Docker container logs (blue-green)
docker logs togather-blue
docker logs togather-green

# Deployment logs
ls -la ~/.togather/logs/deployments/
tail -100 ~/.togather/logs/deployments/production_*.log
```

### Check Running Containers

```bash
# For blue-green deployments
docker ps --filter name=togather-

# Expected output shows active slot:
# togather-blue    Up 5 minutes   0.0.0.0:8081->8080/tcp
# togather-green   Up 30 minutes  0.0.0.0:8082->8080/tcp
```

---

## Rollback

### Automatic Rollback

Blue-green deployments automatically roll back on failure:
- Health check failures
- Migration failures
- Deployment errors

No manual intervention required.

### Manual Rollback

#### For Blue-Green Deployments

Switch back to previous slot:

```bash
# 1. Check current active slot
curl -I https://your-domain.com/health | grep slot
# x-togather-slot: blue

# 2. Edit Caddy configuration
sudo nano /etc/caddy/Caddyfile

# 3. Change reverse_proxy port:
#    If blue is active (8081), switch to green (8082)
#    If green is active (8082), switch to blue (8081)

# 4. Reload Caddy
sudo systemctl reload caddy

# 5. Verify
curl -I https://your-domain.com/health | grep slot
```

#### For Simple Deployments

Redeploy previous version:

```bash
# Get previous version
git checkout v1.2.2  # or previous commit

# Build and deploy
make deploy-package
scp dist/togather-server-*.tar.gz deploy@server:~/
ssh deploy@server
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh
```

---

## Best Practices

1. **Always test on staging first**
   - Deploy to staging
   - Run smoke tests
   - Verify functionality
   - Then deploy to production

2. **Use version tags for production**
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ./deploy/scripts/deploy.sh production --remote deploy@prod --version v1.2.3
   ```

3. **Monitor during deployment**
   - Watch deployment logs
   - Monitor metrics/health endpoints
   - Have rollback plan ready

4. **Create database snapshots**
   - Automatic with `deploy.sh`
   - Manual: `togather-server snapshot create`

5. **Verify after deployment**
   ```bash
   # Check health
   curl https://your-domain.com/health
   
   # Check active slot
   curl -I https://your-domain.com/health | grep slot
   
   # Check logs
   docker logs togather-blue
   ```

---

## Troubleshooting

### Deployment Failures

**Check deployment logs:**
```bash
ls ~/.togather/logs/deployments/
tail -100 ~/.togather/logs/deployments/staging_*.log
```

**Check Docker status:**
```bash
docker ps -a
docker logs togather-blue
docker logs togather-green
```

**Check Caddy:**
```bash
sudo systemctl status caddy
sudo journalctl -u caddy -n 100
```

### Migration Failures

Database migrations are automatically backed up before running.

**Check migration status:**
```bash
cd /opt/togather/src
docker exec -it togather-postgres psql -U togather -d togather -c "SELECT * FROM schema_migrations;"
```

**Restore from snapshot:**
```bash
togather-server snapshot list
togather-server snapshot restore <snapshot-name>
```

### Health Check Failures

**Debug health endpoint:**
```bash
curl -v http://localhost:8080/health
curl -v http://localhost:8081/health  # Blue slot
curl -v http://localhost:8082/health  # Green slot
```

**Check database connectivity:**
```bash
docker exec -it togather-postgres psql -U togather -d togather -c "SELECT 1;"
```

### SSH/Git Issues

**Test SSH connection:**
```bash
ssh deploy@server 'echo "Connection successful"'
```

**Test git access:**
```bash
ssh deploy@server 'git ls-remote git@github.com:Togather-Foundation/server.git'
```

**Check repository state:**
```bash
ssh deploy@server 'cd /opt/togather/src && git status'
```

---

## Quick Troubleshooting Reference

| Symptom | Likely Cause | Quick Fix |
|---------|-------------|-----------|
| "Lock file exists" | Stale deployment lock | Wait 30 min or manually remove lock |
| "Port already in use" | Orphaned container | Rerun deploy.sh (auto-cleans orphans) |
| Health check fails | Configuration error | Check logs: `docker logs togather-blue` |
| Caddy not switching | Manual intervention | See rollback.md for manual traffic switch |
| DNS resolution fails | Network misconfiguration | Verify `docker network inspect togather-network` |

For detailed troubleshooting, see [troubleshooting.md](docs/deploy/troubleshooting.md).

---

## Additional Resources

- [Quickstart Guide](docs/deploy/quickstart.md) - Installation and setup
- [Remote Deployment Guide](docs/deploy/remote-deployment.md) - Comprehensive remote deployment guide
- [Troubleshooting](docs/deploy/troubleshooting.md) - Common issues and solutions
- [Best Practices](docs/deploy/best-practices.md) - Deployment best practices
- [Rollback Guide](docs/deploy/rollback.md) - Detailed rollback procedures
