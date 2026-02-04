# Server Restart Guide

Quick reference for restarting the Togather server in different environments.

## Quick Commands

### Local Development

```bash
# Full restart (rebuild + restart)
make restart

# Quick restart (no rebuild, just restart containers)
make restart-quick
```

### Staging Server

```bash
# Restart staging server (via SSH)
make restart-staging

# Restart specific slot
make restart-slot SLOT=blue ENV=staging
make restart-slot SLOT=green ENV=staging
```

### Production Server

```bash
# Restart production server (via SSH)
make restart-production

# Restart specific slot
make restart-slot SLOT=blue ENV=production
```

## Restart Script Usage

The `deploy/scripts/restart.sh` script provides fine-grained control:

```bash
# Local restart (active slot)
./deploy/scripts/restart.sh local

# Remote restart via SSH
./deploy/scripts/restart.sh staging --remote deploy@staging.example.com

# Restart specific slot
./deploy/scripts/restart.sh staging --slot blue --remote deploy@togather

# Restart both slots
./deploy/scripts/restart.sh staging --slot both --remote deploy@togather

# Hard restart (stop + start instead of graceful)
./deploy/scripts/restart.sh staging --hard --remote deploy@togather
```

## When to Use Which Method

### `make restart` (Full Restart)
- **When:** Code changes, dependency updates
- **Speed:** Slow (~30-60 seconds)
- **What it does:** Rebuilds binary, stops server, starts server
- **Downtime:** Yes (local only)

### `make restart-quick` / `make restart-staging` (Container Restart)
- **When:** Configuration changes (.env), testing
- **Speed:** Fast (~5-10 seconds)
- **What it does:** Gracefully restarts Docker containers
- **Downtime:** Minimal (seconds)

### Blue-Green Slot Restart
- **When:** Need zero-downtime restart, testing new configuration
- **Speed:** Medium (~15-30 seconds)
- **What it does:** Restarts one slot while other serves traffic
- **Downtime:** None

## Common Use Cases

### Applying .env Changes

After editing `.env` files on the server:

```bash
# Staging
ssh togather "nano /opt/togather/.env.staging"
make restart-staging

# Production
ssh production "nano /opt/togather/.env.production"
make restart-production
```

### Changing Admin Password

```bash
# 1. Update password on server
ssh togather
nano /opt/togather/.env.staging  # Edit ADMIN_PASSWORD
exit

# 2. Restart to apply (will re-bootstrap admin user)
make restart-staging
```

### Testing Blue-Green Deployment

```bash
# Deploy to green slot
./deploy/scripts/deploy.sh staging

# Test green slot
curl https://staging.togather.foundation/health -H "X-Togather-Slot: green"

# If good, Caddy already switched traffic automatically
# If issues, restart specific slot
make restart-slot SLOT=green ENV=staging
```

### Emergency Restart

```bash
# Quick restart if server is misbehaving
make restart-staging

# Hard restart if graceful fails
./deploy/scripts/restart.sh staging --hard --remote deploy@togather
```

## Restart Modes

### Graceful Restart (Default)
- Sends SIGTERM to container
- Waits for graceful shutdown (default 30s timeout)
- Kills with SIGKILL if timeout exceeded
- **Recommended** for production

### Hard Restart (`--hard`)
- Explicitly stops container first
- Then starts it
- Use when graceful restart isn't working
- Slightly longer downtime

## Troubleshooting

### Restart Fails - Container Won't Start

```bash
# Check container logs
ssh togather "docker logs togather-server-blue"

# Check .env configuration
ssh togather "cat /opt/togather/.env.staging"

# Try hard restart
./deploy/scripts/restart.sh staging --hard --remote deploy@togather
```

### Health Check Fails After Restart

```bash
# Check health endpoint directly
ssh togather "docker exec togather-server-blue /app/server healthcheck"

# Check container status
ssh togather "docker ps -a | grep togather-server"

# View recent logs
ssh togather "docker logs --tail 100 togather-server-blue"
```

### Both Slots Are Down

```bash
# Emergency: Deploy to both slots
./deploy/scripts/deploy.sh staging --slot both

# Or restart database if that's the issue
ssh togather "docker restart togather-db"
```

## Configuration

Restart behavior is configured via:

- **Deployment config:** `deploy/config/deployment.yml`
- **Environment config:** `.deploy.conf.{environment}`
- **Timeout:** Default 30s, override with `--timeout` flag

Example:

```bash
./deploy/scripts/restart.sh staging --timeout 60 --remote deploy@togather
```

## Integration with Other Tools

### Use with bd (Beads) Issue Tracker

```bash
# Track restart as part of issue resolution
bd update <issue-id> --notes "Restarted staging to apply fix"
make restart-staging
bd close <issue-id>
```

### Use with Deployment Scripts

```bash
# Full deploy then restart to apply env changes
./deploy/scripts/deploy.sh staging
make restart-staging
```

### Use with Health Checks

```bash
# Restart and verify health
make restart-staging
./deploy/scripts/health-check.sh staging
```

## See Also

- **Deployment Guide:** `docs/deploy/deployment-testing.md`
- **Health Checks:** `deploy/scripts/health-check.sh`
- **Rollback:** `deploy/scripts/rollback.sh`
- **Configuration:** `docs/deploy/deploy-conf.md`
