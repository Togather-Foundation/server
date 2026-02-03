# Deployment Testing Checklist

This document provides a comprehensive checklist for testing deployments of the Togather SEL server. Use this after any deployment to verify everything is working correctly.

## Purpose

This checklist is designed to be used by:
- Developers manually testing deployments
- Automated CI/CD pipelines
- Agents performing "deploy and test" workflows

## Quick Reference

```bash
# For agents: Run full deploy + test workflow
./deploy/scripts/deploy.sh <environment> --remote deploy@<server>
# Then run through this checklist systematically
```

---

## Pre-Deployment Checks

### Local Build Verification

- [ ] **Code compiles locally**
  ```bash
  make build
  ```

- [ ] **Tests pass locally**
  ```bash
  make test-ci
  ```

- [ ] **Linter passes**
  ```bash
  make lint-ci
  ```

- [ ] **Deployment package builds**
  ```bash
  make deploy-package
  ls -lh dist/togather-*.tar.gz
  ```

### Environment Preparation

- [ ] **Target environment configured**
  - Staging: `deploy/config/environments/.env.staging` exists
  - Production: `deploy/config/environments/.env.production` exists

- [ ] **SSH access verified**
  ```bash
  ssh deploy@<server> 'echo "SSH OK"'
  ```

- [ ] **Server has sufficient disk space**
  ```bash
  ssh deploy@<server> 'df -h /opt/togather'
  # Should have at least 2GB free
  ```

---

## Deployment Execution

### Deploy Script Run

- [ ] **Deployment script completes successfully**
  ```bash
  ./deploy/scripts/deploy.sh <environment> --remote deploy@<server>
  # Exit code should be 0
  echo $?
  ```

- [ ] **No critical errors in output**
  - Check for "ERROR:", "FATAL:", "deployment failed"
  - Warnings are OK, errors are not

- [ ] **Blue-green slot switched**
  ```bash
  ssh deploy@<server> 'cat /opt/togather/src/deploy/config/deployment-state.json | jq -r ".current_deployment.active_slot"'
  # Should show new slot (opposite of previous)
  ```

---

## Post-Deployment Health Checks

### Container Health

- [ ] **New slot container is running**
  ```bash
  ssh deploy@<server> 'docker ps --filter name=togather-server-<SLOT>'
  # STATUS should be "Up" and "(healthy)"
  ```

- [ ] **Container logs show no errors**
  ```bash
  ssh deploy@<server> 'docker logs togather-server-<SLOT> --tail 50'
  # Look for: "Server started on port XXXX", no panic/fatal errors
  ```

- [ ] **Old slot container still exists (for rollback)**
  ```bash
  ssh deploy@<server> 'docker ps -a | grep togather-server'
  # Both blue and green should exist, one running, one stopped
  ```

### Database Health

- [ ] **Database container is running**
  ```bash
  ssh deploy@<server> 'docker ps --filter name=togather-db'
  # STATUS should be "Up" and "(healthy)"
  ```

- [ ] **Database is accepting connections**
  ```bash
  ssh deploy@<server> 'docker exec togather-db pg_isready -U togather'
  # Should output: "accepting connections"
  ```

- [ ] **Migrations are clean**
  ```bash
  ssh deploy@<server> 'docker exec togather-server-<SLOT> /app/server db status'
  # Should show current version, no "dirty" state
  ```

### Application Health

- [ ] **Health endpoint responds (internal)**
  ```bash
  ssh deploy@<server> 'curl -s http://localhost:808X/health | jq'
  # Replace 808X with slot port (8080=blue, 8081=green)
  ```

- [ ] **Health status is healthy or degraded**
  ```json
  {
    "status": "healthy",  // or "degraded" (acceptable)
    "timestamp": "...",
    "version": "...",
    "slot": "blue"  // or "green"
  }
  ```

- [ ] **Health checks pass all components**
  ```bash
  ssh deploy@<server> 'curl -s http://localhost:808X/health | jq ".checks"'
  # All critical checks should pass
  ```

### Caddy Proxy Health

- [ ] **Caddy service is running**
  ```bash
  ssh deploy@<server> 'systemctl status caddy | grep Active'
  # Should be "active (running)"
  ```

- [ ] **Caddy is routing to active slot**
  ```bash
  ssh deploy@<server> 'grep "reverse_proxy localhost:" /etc/caddy/Caddyfile'
  # Should show port matching active slot (8080 or 8081)
  ```

- [ ] **External HTTPS health check**
  ```bash
  curl -s https://<domain>/health | jq
  # Should return same health data as internal check
  ```

- [ ] **X-Togather-Slot header matches active slot**
  ```bash
  curl -I https://<domain>/health | grep X-Togather-Slot
  # Should show current active slot
  ```

---

## Functional Testing

### API Endpoints

- [ ] **Root endpoint responds**
  ```bash
  curl -s https://<domain>/ | jq
  # Should return API info (name, version, links)
  ```

- [ ] **API documentation accessible**
  ```bash
  curl -I https://<domain>/docs
  # Should return 200 OK
  ```

- [ ] **OpenAPI schema available**
  ```bash
  curl -s https://<domain>/openapi.json | jq -r '.openapi'
  # Should return OpenAPI version (e.g., "3.1.0")
  ```

### Events API

- [ ] **Events endpoint responds**
  ```bash
  curl -s https://<domain>/api/v1/events | jq
  # Should return events array (may be empty)
  ```

- [ ] **JSON-LD content negotiation works**
  ```bash
  curl -s -H "Accept: application/ld+json" https://<domain>/api/v1/events | jq
  # Should return JSON-LD formatted response with @context
  ```

### Places and Organizations APIs

- [ ] **Places endpoint responds**
  ```bash
  curl -s https://<domain>/api/v1/places | jq
  # Should return places array (may be empty)
  ```

- [ ] **Organizations endpoint responds**
  ```bash
  curl -s https://<domain>/api/v1/organizations | jq
  # Should return organizations array (may be empty)
  ```

### Admin UI

- [ ] **Admin login page loads**
  ```bash
  curl -I https://<domain>/admin/login
  # Should return 200 OK
  ```

- [ ] **Admin templates rendered correctly**
  ```bash
  curl -s https://<domain>/admin/login | grep -i "<!DOCTYPE html>"
  # Should find HTML doctype (templates are loaded)
  ```

---

## Resource Verification

### Static Files

- [ ] **Context files accessible in container**
  ```bash
  ssh deploy@<server> 'docker exec togather-server-<SLOT> ls -la /app/contexts/sel/'
  # Should show v0.1.jsonld file
  ```

- [ ] **Admin templates accessible in container**
  ```bash
  ssh deploy@<server> 'docker exec togather-server-<SLOT> ls -la /app/web/admin/templates/'
  # Should show dashboard.html, login.html
  ```

### Configuration

- [ ] **.env file exists and is readable**
  ```bash
  ssh deploy@<server> 'test -r /opt/togather/.env && echo "OK" || echo "MISSING"'
  # Should output "OK"
  ```

- [ ] **Slot environment variable set**
  ```bash
  ssh deploy@<server> 'docker exec togather-server-<SLOT> env | grep SLOT'
  # Should show SLOT=blue or SLOT=green
  ```

---

## Security Checks

### TLS/SSL

- [ ] **HTTPS certificate is valid**
  ```bash
  curl -vI https://<domain>/ 2>&1 | grep "SSL certificate verify"
  # Should see "SSL certificate verify ok"
  ```

- [ ] **HTTP redirects to HTTPS**
  ```bash
  curl -I http://<domain>/ | grep -i location
  # Should redirect to https://
  ```

### Access Control

- [ ] **Admin endpoints require authentication**
  ```bash
  curl -I https://<domain>/admin/
  # Should return 302 (redirect to login) or 401 (unauthorized)
  ```

### Headers

- [ ] **Slot identification header present**
  ```bash
  curl -I https://<domain>/health | grep X-Togather-Slot
  # Should show active slot
  ```

---

## Rollback Verification

### Rollback Capability

- [ ] **Previous slot still exists and can start**
  ```bash
  ssh deploy@<server> 'docker ps -a | grep togather-server-<OLD_SLOT>'
  # Should show stopped container with recent stop time
  ```

- [ ] **Deployment state file is valid JSON**
  ```bash
  ssh deploy@<server> 'cat /opt/togather/src/deploy/config/deployment-state.json | jq -r ".version"'
  # Should return version number
  ```

### Rollback Test (Optional - Only in Staging)

- [ ] **Can perform rollback successfully**
  ```bash
  ./deploy/scripts/deploy.sh <environment> --remote deploy@<server> --rollback
  # Should switch back to previous slot
  ```

- [ ] **After rollback, service still healthy**
  ```bash
  curl -s https://<domain>/health | jq -r '.status'
  # Should return "healthy" or "degraded"
  ```

---

## Data Integrity

### Database State

- [ ] **Database migrations completed**
  ```bash
  ssh deploy@<server> 'docker exec togather-server-<SLOT> /app/server db status'
  # Should show latest migration version
  ```

- [ ] **No orphaned connections**
  ```bash
  ssh deploy@<server> 'docker exec togather-db psql -U togather -c "SELECT count(*) FROM pg_stat_activity WHERE application_name != '\'''"'
  # Should show reasonable number of connections (< 20 for new deploy)
  ```

### Sample Data Queries

- [ ] **Can query events table**
  ```bash
  ssh deploy@<server> 'docker exec togather-db psql -U togather -c "SELECT COUNT(*) FROM events;"'
  # Should return count (may be 0)
  ```

---

## Networking

### Docker Network

- [ ] **Containers on same network**
  ```bash
  ssh deploy@<server> 'docker network inspect togather-network --format "{{range .Containers}}{{.Name}} {{end}}"'
  # Should show all containers: blue, green, db
  ```

### Port Bindings

- [ ] **Active slot port is bound**
  ```bash
  ssh deploy@<server> 'netstat -tuln | grep "808[01]"'
  # Should see active slot port (8080 or 8081) in LISTEN state
  ```

---

## Documentation and State

### Deployment State

- [ ] **deployment-state.json is up to date**
  ```bash
  ssh deploy@<server> 'cat /opt/togather/src/deploy/config/deployment-state.json | jq'
  ```
  Check:
  - `current_deployment.active_slot` matches reality
  - `current_deployment.version` matches deployed version
  - `current_deployment.deployed_at` is recent
  - `previous_deployment` shows old slot

- [ ] **Deployment lock is released**
  ```bash
  ssh deploy@<server> 'test -f /tmp/togather-deploy.lock && echo "LOCKED" || echo "OK"'
  # Should output "OK" (no lock file)
  ```

### Version Information

- [ ] **Server reports correct version**
  ```bash
  curl -s https://<domain>/health | jq -r '.version'
  # Should match deployed version
  ```

---

## Common Issues and Solutions

### Issue: Health check returns "degraded"
**Solution:** This is acceptable post-deployment. Check what's degraded:
```bash
curl -s https://<domain>/health | jq '.checks'
```
Common causes: JSON-LD contexts not fully loaded, optional features disabled.

### Issue: Container unhealthy or not starting
**Solution:** Check container logs for errors:
```bash
ssh deploy@<server> 'docker logs togather-server-<SLOT> --tail 100'
```
Look for: database connection errors, missing env vars, port conflicts.

### Issue: Caddy not routing to new slot
**Solution:** Verify Caddyfile and reload:
```bash
ssh deploy@<server> 'cat /etc/caddy/Caddyfile | grep reverse_proxy'
ssh deploy@<server> 'sudo systemctl reload caddy'
```

---

## Agent Instructions

When an agent is asked to "deploy and test", they should:

1. **Execute deployment:**
   ```bash
   ./deploy/scripts/deploy.sh <environment> --remote deploy@<server>
   ```

2. **Wait for health stabilization** (30-60 seconds)

3. **Run through critical checks** (at minimum):
   - Container health status
   - External HTTPS health endpoint  
   - API endpoints respond
   - Admin UI loads
   - Active slot matches expected

4. **Report results** in summary format:
   ```
   Deployment: ✅ Success
   Health: ✅ Healthy
   API: ✅ Responding
   Admin UI: ✅ Accessible
   Slot: blue (switched from green)
   Version: v1.2.3
   ```

5. **If any check fails**, capture logs and report:
   ```bash
   ssh deploy@<server> 'docker logs togather-server-<SLOT> --tail 100'
   ```

---

## Automated Testing Script

For automated testing, save this as `deploy/scripts/test-deployment.sh`:

```bash
#!/bin/bash
# test-deployment.sh - Automated deployment testing

set -euo pipefail

ENVIRONMENT="${1:-staging}"
SERVER="${2:-deploy@staging}"
DOMAIN="${3:-staging.toronto.togather.foundation}"

echo "Testing deployment on $DOMAIN..."

FAILED=0

# Health check
if curl -sf "https://$DOMAIN/health" | jq -e '.status == "healthy" or .status == "degraded"' > /dev/null; then
    echo "✓ Health check passed"
else
    echo "✗ Health check failed"
    FAILED=$((FAILED + 1))
fi

# API check
if curl -sf "https://$DOMAIN/api/v1/events" > /dev/null; then
    echo "✓ Events API accessible"
else
    echo "✗ Events API failed"
    FAILED=$((FAILED + 1))
fi

# Admin UI check
if curl -sf "https://$DOMAIN/admin/login" | grep -q "<!DOCTYPE html>"; then
    echo "✓ Admin UI accessible"
else
    echo "✗ Admin UI failed"
    FAILED=$((FAILED + 1))
fi

# Container health
if ssh "$SERVER" 'docker ps --format "{{.Status}}" --filter name=togather-server' | grep -q "(healthy)"; then
    echo "✓ Container healthy"
else
    echo "✗ Container unhealthy"
    FAILED=$((FAILED + 1))
fi

# Slot verification
ACTIVE_SLOT=$(curl -sI "https://$DOMAIN/health" | grep -i "X-Togather-Slot" | cut -d: -f2 | tr -d ' \r')
echo "Active slot: $ACTIVE_SLOT"

# Final result
if [ $FAILED -eq 0 ]; then
    echo "✅ All checks passed!"
    exit 0
else
    echo "❌ $FAILED checks failed"
    exit 1
fi
```

Make it executable:
```bash
chmod +x deploy/scripts/test-deployment.sh
```

Usage:
```bash
# Test staging
./deploy/scripts/test-deployment.sh staging deploy@192.46.222.199 staging.toronto.togather.foundation

# Test production
./deploy/scripts/test-deployment.sh production deploy@prod-server prod.togather.foundation
```

---

## Related Documentation

- [Deployment Quick Start](./quickstart.md) - Getting started with deployments
- [Remote Deployment Guide](./remote-deployment.md) - Detailed remote deployment instructions
- [Troubleshooting](./troubleshooting.md) - Common deployment issues
- [Rollback Procedures](./rollback.md) - How to rollback failed deployments
- [CADDY-ARCHITECTURE.md](../../deploy/CADDY-ARCHITECTURE.md) - Caddy proxy architecture

---

## Summary

This checklist ensures that deployments are:
- ✅ Successful and stable
- ✅ Functionally correct
- ✅ Secure and performant
- ✅ Properly monitored
- ✅ Rollback-capable

Use this checklist after **every** deployment to catch issues early and maintain system reliability.
