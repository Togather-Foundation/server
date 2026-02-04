# Caddy Proxy Architecture for Togather Server

## Overview

The Togather server uses **different Caddy setups for different environments**:

- **Local Development**: Docker Caddy container (defined in docker-compose.blue-green.yml)
- **Staging/Production**: System Caddy installed via apt (managed by systemd)

This document clarifies the architecture and resolves confusion between the two approaches.

## Architecture Decision

**System Caddy (not containerized) for staging/production deployments**

### Reasons:
1. **Automatic SSL**: Let's Encrypt certificate management is simpler with system Caddy
2. **Service Resilience**: Survives Docker container restarts and Docker daemon issues
3. **Management**: Easier systemctl integration (`systemctl status caddy`)
4. **Consistency**: deploy.sh already implements system Caddy traffic switching
5. **Simplicity**: One less container to manage in production

## Deployment Paths

### Local Development (Docker Caddy)

**Files:**
- `deploy/docker/docker-compose.blue-green.yml` - Defines Caddy container
- `deploy/docker/Caddyfile.example` - Docker Caddy configuration

**Setup:**
```bash
# 1. Copy example Caddyfile
cp deploy/docker/Caddyfile.example deploy/docker/Caddyfile

# 2. Start containers with Caddy
docker compose -f docker-compose.yml -f docker-compose.blue-green.yml up -d

# 3. Access via proxy
curl http://localhost/health

# 4. Access slots directly
curl http://localhost:8081/health  # Blue
curl http://localhost:8082/health  # Green
```

**Traffic Switching:**
```bash
# Edit deploy/docker/Caddyfile to change reverse_proxy target
# Then reload:
docker compose -f docker-compose.blue-green.yml exec caddy caddy reload --config /etc/caddy/Caddyfile
```

### Staging Deployment (System Caddy)

**Files:**
- `deploy/config/environments/Caddyfile.staging` - Staging configuration
- `deploy/scripts/provision-server.sh` - Installs system Caddy
- `deploy/scripts/install.sh.template` - Deploys Caddyfile
- `deploy/scripts/deploy.sh` - Switches traffic between slots

**Setup:**
```bash
# 1. Provision server (installs Caddy via apt)
./deploy/scripts/provision-server.sh

# 2. Deploy application (configures Caddy)
./deploy/scripts/install.sh

# 3. Verify Caddy is running
systemctl status caddy
```

**Configuration Path:**
- Source: `deploy/config/environments/Caddyfile.staging`
- Deployed to: `/etc/caddy/Caddyfile`
- Logs: `/var/log/caddy/staging.toronto.log`

**Traffic Switching:**
- Automatic during deployment via `deploy.sh` (syncs the environment Caddyfile)
- Manual: Edit `/etc/caddy/Caddyfile` and run `systemctl reload caddy`

### Production Deployment (System Caddy)

**Files:**
- `deploy/config/environments/Caddyfile.production` - Production configuration
- Same deployment scripts as staging

**Setup:** Same as staging but with `ENVIRONMENT=production`

## Quick Fixes

### Fix Missing Caddy on Staging Server

If staging server was provisioned but Caddy isn't routing traffic, deploy
the environment Caddyfile and start the service:

```bash
# On staging server
sudo cp /opt/togather/deploy/config/environments/Caddyfile.staging /etc/caddy/Caddyfile
sudo systemctl enable caddy
sudo systemctl start caddy
```

Or manually:
```bash
# 1. Deploy Caddyfile
sudo cp /opt/togather/deploy/config/environments/Caddyfile.staging /etc/caddy/Caddyfile

# 2. Create log directory
sudo mkdir -p /var/log/caddy
sudo touch /var/log/caddy/staging.toronto.log
sudo chown -R caddy:caddy /var/log/caddy

# 3. Start Caddy
sudo systemctl enable caddy
sudo systemctl start caddy

# 4. Verify
systemctl status caddy
curl -I http://localhost/health
```

### Check Which Slot Is Active

```bash
# Via proxy (what users see)
curl -I http://localhost/health | grep X-Togather-Slot

# Or check Caddyfile
grep "reverse_proxy localhost:" /etc/caddy/Caddyfile
```

## File Reference

### Docker Caddy (Local Dev)
| File | Purpose |
|------|---------|
| `deploy/docker/docker-compose.blue-green.yml` | Defines Caddy container service |
| `deploy/docker/Caddyfile.example` | Docker Caddy config template |
| `deploy/docker/Caddyfile` | Active Docker Caddy config (git-ignored) |

### System Caddy (Staging/Prod)
| File | Purpose |
|------|---------|
| `deploy/config/environments/Caddyfile.staging` | Staging config source |
| `deploy/config/environments/Caddyfile.production` | Production config source |
| `/etc/caddy/Caddyfile` | Active system Caddy config (on server) |
| `deploy/scripts/provision-server.sh` | Installs system Caddy |
| `deploy/scripts/install.sh.template` | Deploys Caddyfile |
| `deploy/scripts/deploy.sh` | Switches traffic |

## Troubleshooting

### Caddy Not Running
```bash
# Check status
systemctl status caddy

# View logs
sudo journalctl -u caddy -f

# Restart
sudo systemctl restart caddy
```

### Caddyfile Validation Failed
```bash
# Validate syntax
sudo caddy validate --config /etc/caddy/Caddyfile

# View errors
sudo caddy validate --config /etc/caddy/Caddyfile 2>&1
```

### Health Check Fails
```bash
# Check if slots are running
curl http://localhost:8081/health  # Blue
curl http://localhost:8082/health  # Green

# Check if Caddy can reach them
sudo journalctl -u caddy | tail -50
```

### SSL Certificate Issues (Production)
```bash
# Check certificate status
sudo caddy list-certificates

# Force certificate renewal
sudo systemctl stop caddy
sudo caddy renew --config /etc/caddy/Caddyfile
sudo systemctl start caddy
```

## Common Mistakes

### ❌ Trying to use Docker Caddy in production
**Don't do this.** Use system Caddy for staging/production.

### ❌ Editing docker-compose Caddy config for staging
**Wrong location.** Staging uses `/etc/caddy/Caddyfile`, not Docker config.

### ❌ Assuming Caddy is running after provision
**Not guaranteed.** Run `install.sh` to deploy Caddyfile and start Caddy.

### ❌ Forgetting to reload after Caddyfile changes
**Changes don't apply automatically.** Run `systemctl reload caddy` after editing.

## References

- Blue-green deployment spec: `specs/001-deployment-infrastructure/spec.md` FR-009
- Caddy documentation: https://caddyserver.com/docs/
- System Caddy service: `/etc/systemd/system/caddy.service`
