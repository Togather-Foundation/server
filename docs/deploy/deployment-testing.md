# Deployment Testing Checklist

This document provides a comprehensive checklist for testing deployments of the Togather SEL server. Use this after any deployment to verify everything is working correctly.

## Purpose

This checklist is designed to be used by:
- Developers manually testing deployments
- Automated CI/CD pipelines
- Agents performing "deploy and test" workflows

## Quick Reference for Agents

**IMPORTANT:** Each environment has a `.deploy.conf.{environment}` file (gitignored) that contains deployment metadata like NODE_DOMAIN, SSH_HOST, etc. Always check for this file first to avoid guessing configuration.

```bash
# 1. Check for deployment config
if [ -f .deploy.conf.staging ]; then
    source .deploy.conf.staging
    echo "Deploying to: $NODE_DOMAIN via $SSH_HOST"
fi

# 2. Deploy (auto-loads .deploy.conf if available)
./deploy/scripts/deploy.sh staging

# 3. Run automated tests (auto-uses NODE_DOMAIN from .deploy.conf)
./deploy/testing/smoke-tests.sh staging
```

See `docs/deploy/deploy-conf.md` for complete .deploy.conf documentation.

---

## Pre-Deployment Checks

### Deployment Configuration

- [ ] **Deployment config exists**
  ```bash
  # Check if .deploy.conf.{environment} exists
  ls -l .deploy.conf.staging
  cat .deploy.conf.staging  # Verify NODE_DOMAIN, SSH_HOST, etc.
  ```
  - If missing, copy from `.deploy.conf.example` and customize
  - Contains: NODE_DOMAIN, SSH_HOST, SSH_USER, CITY, REGION
  - See `docs/deploy/deploy-conf.md` for details

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

- [ ] **Target environment configured on server**
  - Staging: `/opt/togather/.env.staging` exists on staging server
  - Production: `/opt/togather/.env.production` exists on production server
  - Note: These files live on the server, NOT in the local git repo

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
# Replace 808X with slot port (8081=blue, 8082=green)
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
# Should show port matching active slot (8081 or 8082)
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

- [ ] **robots.txt has correct domain**
  ```bash
  curl -s https://<domain>/robots.txt | grep "Sitemap:"
  # Should show: Sitemap: https://<domain>/sitemap.xml
  # Domain should match environment (togather.foundation for prod, staging.toronto.togather.foundation for staging)
  ```

- [ ] **sitemap.xml has correct domain**
  ```bash
  curl -s https://<domain>/sitemap.xml | grep -o "<loc>https://[^<]*" | head -3
  # All URLs should use correct domain for environment
  # Should see: <loc>https://<domain>/...
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
# Should see active slot port (8081 or 8082) in LISTEN state
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

### Issue: Static files (JS/CSS) not updating after deployment
**Symptom:** After deploying new code with updated JavaScript or CSS, files served through Caddy are stale even though the container serves fresh content.

**Root Cause:** Caddy maintains persistent HTTP keep-alive connections to upstream backends (connection pooling). When a new container starts on the same port as the old one, existing connections may still point to stale state.

**Solution (T043 - Configuration Fix):**
Caddyfiles have been updated with `transport http { keepalive off }` to disable connection pooling to upstreams. This ensures:
- Every request creates a fresh connection to the current container
- No stale connections survive container replacements
- Zero-downtime static file updates without manual intervention
- No Caddy reload required

**Trade-off:** Minor performance impact from TCP handshake overhead (negligible for localhost upstreams).

**Diagnosis (if issue persists):**
```bash
# 1. Verify Caddyfile has keepalive disabled
ssh deploy@<server> "grep -A 3 'transport http' /etc/caddy/Caddyfile"
# Should show: keepalive off

# 2. Verify container has new content
ssh deploy@<server> "docker exec togather-server-blue grep 'YOUR_NEW_STRING' /app/web/admin/static/js/FILE.js"

# 3. Verify direct container access works
ssh deploy@<server> "curl -s http://localhost:8081/admin/static/js/FILE.js | grep 'YOUR_NEW_STRING'"

# 4. Test through Caddy (should show NEW content)
curl -s "https://${NODE_DOMAIN}/admin/static/js/FILE.js" | grep 'YOUR_NEW_STRING'
```

**Legacy Fix (if using old Caddyfile without keepalive off):**
```bash
# Reload Caddy to close stale connections (causes brief connection reset)
ssh deploy@<server> "sudo systemctl reload caddy"
```

**Why This Happens:**
- Static files are embedded in Docker images at build time (`//go:embed admin/static`)
- Caddy's `keepalive_idle_conns_per_host` (default: 32) pools connections
- Pooled connections survive container replacements on the same port
- Disabling keepalive prevents connection reuse across deployments

**Testing After Deploy:**
```bash
# Verify Caddy serves same content as container (should match)
ssh deploy@<server> "curl -s http://localhost:8081/admin/static/js/FILE.js | sha256sum"
curl -s "https://${NODE_DOMAIN}/admin/static/js/FILE.js" | sha256sum
```

---

## Agent Instructions

When an agent is asked to "deploy and test", they should:

1. **Execute deployment:**
   ```bash
   ./deploy/scripts/deploy.sh <environment> --remote deploy@<server>
   ```

2. **Wait for health stabilization** (30-60 seconds)

3. **Run automated tests:**
   ```bash
   # Run all automated tests (smoke + performance if allowed)
   ./deploy/scripts/test-remote.sh <environment> all
   
   # Or just smoke tests (faster, production-safe)
   ./deploy/scripts/test-remote.sh <environment> smoke
   ```

4. **Report results** - The test script will provide a summary automatically

5. **If any check fails**, capture logs and report:
   ```bash
   ssh deploy@<server> 'docker logs togather-server-<SLOT> --tail 100'
   ```

---

## Automated Testing Scripts

The project includes comprehensive automated test scripts for deployment verification.

### Primary Test Script: `test-remote.sh`

Unified test interface with environment-aware testing:

```bash
# Run all tests on staging (smoke + performance)
./deploy/scripts/test-remote.sh staging all

# Run just smoke tests (fast, production-safe)
./deploy/scripts/test-remote.sh staging smoke

# Run performance tests only (requires ALLOW_LOAD_TESTING=true)
./deploy/scripts/test-remote.sh staging perf

# Test production (read-only, smoke tests only)
./deploy/scripts/test-remote.sh production smoke
```

**Features:**
- Environment-aware configuration (local/staging/production)
- Comprehensive smoke tests (17 checks including health, database, migrations, APIs, security, containers, River workers)
- Optional performance testing (staging only)
- Production safety (read-only mode, no destructive tests)
- Detailed logging with color-coded output
- Container health verification via SSH

**Smoke test coverage:**
1. Health endpoint validation
2. Version endpoint verification
3. Database connectivity check
4. Migration status validation
5. HTTP endpoint health check
6. CORS headers verification
7. Security headers check
8. Response time measurement
9. Events API endpoint
10. Places API endpoint
11. Organizations API endpoint
12. OpenAPI schema validation
13. Admin UI accessibility
14. HTTPS certificate validation
15. Active slot identification
16. River worker job processing (batch ingestion end-to-end)
17. Container health via Docker

### Usage Recommendations

**For agents performing "deploy and test":**
```bash
# 1. Deploy
./deploy/scripts/deploy.sh staging --remote deploy@staging.toronto.togather.foundation

# 2. Run automated tests (all on staging)
./deploy/scripts/test-remote.sh staging all
```

**For production deployments:**
```bash
# Production only allows smoke tests (no load testing)
./deploy/scripts/test-remote.sh production smoke
```

### Configuration

Tests use environment-specific configuration files:
- `deploy/testing/environments/local.test.env`
- `deploy/testing/environments/staging.test.env`
- `deploy/testing/environments/production.test.env`

Each configuration includes:
- `BASE_URL` - Server URL to test
- `SSH_SERVER` - SSH connection for container checks (optional for local)
- `TIMEOUT` - Request timeout settings
- `ALLOW_DESTRUCTIVE` - Whether to allow data modification
- `ALLOW_LOAD_TESTING` - Whether performance tests are allowed
- `API_KEY` - API key for authenticated tests (optional, enables River worker test)

Optional environment overrides (for test runners):
- `WAIT_TIMEOUT` - Health wait timeout in seconds (default: 60)
- `SKIP_WAIT_FOR_HEALTH` - Set to `true` to skip pre-test health polling

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

---

## Project Structure Reference

Understanding the deployment structure is critical for troubleshooting and verification.

### Docker Compose Files

**Blue-Green Deployment (Staging/Production):**
- **File:** `deploy/docker/docker-compose.blue-green.yml`
- **Location on server:** `/opt/togather/src/deploy/docker/docker-compose.blue-green.yml`
- **Services defined:** `togather-blue`, `togather-green`, `togather-db`
- **Usage:** `docker compose -f deploy/docker/docker-compose.blue-green.yml up -d`

**Local Development:**
- **File:** `deploy/docker/docker-compose.yml` (if exists, or use blue-green file)
- **Note:** For local dev, use single service or blue-green with manual switching

### Container Naming Conventions

**Service names in compose file:**
- `togather-blue` (blue slot)
- `togather-green` (green slot)
- `togather-db` (PostgreSQL database)

**Actual container names on server:**
- **With environment prefix:** `togather-staging-togather-blue`, `togather-staging-togather-green`
- **Or simple names:** `togather-server-blue`, `togather-server-green`
- **Database:** `togather-db` or `togather-staging-togather-db`

**To check actual names:**
```bash
ssh deploy@<server> 'docker ps --format "{{.Names}}"'
```

### Port Mapping Conventions

**Internal Container Ports:**
- **Blue slot:** Port `8080` inside container
- **Green slot:** Port `8080` inside container
- **Database:** Port `5432` inside container

**External Port Mappings:**
- **Blue:** Host `localhost:8081` → Container `8080`
- **Green:** Host `localhost:8082` → Container `8080`
- **Database:** Host `localhost:5432` → Container `5432` (usually not exposed externally)

**Caddy Proxy:**
- **HTTPS (443)** → Active slot port (8081 or 8082)
- **HTTP (80)** → Redirects to HTTPS

**Quick check which slot is active:**
```bash
ssh deploy@<server> 'grep "reverse_proxy localhost:" /etc/caddy/Caddyfile'
# Output: reverse_proxy localhost:8081  (means blue is active)
# Output: reverse_proxy localhost:8082  (means green is active)
```

### Image Build and Tagging Strategy

**Build process:**
1. **Local build:** `make deploy-package` creates `dist/togather-<version>.tar.gz`
2. **Remote build:** `deploy.sh` builds on server with commit hash tag
3. **Image tag format:** `togather-server:<git-commit-hash>`
   - Example: `togather-server:908511e`

**Docker Compose expectations:**
- Compose file uses `build:` directive pointing to `deploy/docker/`
- Built images should be tagged to match service names for proper recreation
- **Known issue:** Images built with commit hash but compose may not detect changes

**Image tagging workaround (if needed):**
```bash
# Tag commit-based image to compose service name
docker tag togather-server:908511e togather-staging-togather-blue:latest

# Force recreation
docker compose -f deploy/docker/docker-compose.blue-green.yml up -d --force-recreate togather-blue
```

### Deployment State File

**Location:** `/opt/togather/src/deploy/config/deployment-state.json`

**Structure:**
```json
{
  "version": "1.0.0",
  "current_deployment": {
    "active_slot": "blue",
    "version": "908511e",
    "deployed_at": "2026-02-03T15:19:23Z",
    "health_status": "healthy"
  },
  "previous_deployment": {
    "active_slot": "green",
    "version": "7641375",
    "deployed_at": "2026-02-03T10:00:00Z"
  }
}
```

**Read current active slot:**
```bash
ssh deploy@<server> 'cat /opt/togather/src/deploy/config/deployment-state.json | jq -r ".current_deployment.active_slot"'
```

**Read current version:**
```bash
ssh deploy@<server> 'cat /opt/togather/src/deploy/config/deployment-state.json | jq -r ".current_deployment.version"'
```

### Directory Structure on Server

```
/opt/togather/
├── .env                          # Environment variables (DATABASE_URL, etc.)
├── .env.blue                     # Blue slot specific vars
├── .env.green                    # Green slot specific vars
├── contexts/                     # JSON-LD context files (if using volume mount)
│   └── sel/
│       └── v0.1.jsonld
├── src/                          # Git repository clone
│   ├── deploy/
│   │   ├── config/
│   │   │   ├── deployment-state.json    # Active slot tracker
│   │   │   └── environments/
│   │   │       ├── .env.staging
│   │   │       └── .env.production
│   │   ├── docker/
│   │   │   ├── docker-compose.blue-green.yml
│   │   │   └── Dockerfile
│   │   └── scripts/
│   │       ├── deploy.sh
│   │       ├── install.sh
│   │       └── provision.sh
│   └── ...
└── data/                         # PostgreSQL data volume
    └── postgres/
```

### Configuration File Locations

**Environment Files:**
- **Active runtime (on server):** `/opt/togather/.env` (symlink or main file)
- **Slot-specific (on server):** `/opt/togather/.env.blue`, `/opt/togather/.env.green`
- **Templates (in repo):** `deploy/config/environments/.env.staging.example`, `.env.production.example`
- **Note:** Actual `.env.staging` and `.env.production` files exist ONLY on the server at `/opt/togather/`

**Caddy Configuration:**
- **System Caddy (staging/prod):** `/etc/caddy/Caddyfile`
- **Template (staging):** `deploy/config/environments/Caddyfile.staging`
- **Template (production):** `deploy/config/environments/Caddyfile.production`

**Deployment Scripts:**
- **On server:** `/opt/togather/src/deploy/scripts/deploy.sh`
- **In repo:** `deploy/scripts/deploy.sh`

### Common Misunderstandings to Avoid

1. **Docker Compose file is NOT at project root**
   - ❌ Wrong: `./docker-compose.yml`
   - ✅ Correct: `deploy/docker/docker-compose.blue-green.yml`

2. **Container names may have environment prefix**
   - ❌ Wrong: Assuming always `togather-server-blue`
   - ✅ Correct: Check actual names with `docker ps`, may be `togather-staging-togather-blue`

3. **Images need proper tagging for compose to detect changes**
   - ❌ Wrong: Assuming `docker compose up` auto-detects new builds
   - ✅ Correct: Use `--force-recreate` or proper image tags

4. **Port 8080 is INTERNAL to containers**
   - ✅ Blue host port: `localhost:8081`
   - ✅ Green host port: `localhost:8082`
   - ❌ Wrong: Accessing `http://server-ip:8080` directly
   - ✅ Correct: Access via Caddy on port 443, or localhost:8081/8082 from server

5. **Deployment state file is under src/deploy/, not root**
   - ❌ Wrong: `/opt/togather/deployment-state.json`
   - ✅ Correct: `/opt/togather/src/deploy/config/deployment-state.json`

6. **Health endpoint is accessed via HTTPS through Caddy**
   - ❌ Wrong: `http://server-ip:8080/health` (bypasses Caddy)
   - ✅ Correct: `https://domain/health` (through Caddy proxy)

### Quick Reference Commands

```bash
# Check which compose file is being used
ssh deploy@<server> 'ls -la /opt/togather/src/deploy/docker/docker-compose*.yml'

# Check actual container names
ssh deploy@<server> 'docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"'

# Check which ports are exposed
ssh deploy@<server> 'docker ps --format "table {{.Names}}\t{{.Ports}}"'

# Check active slot from state file
ssh deploy@<server> 'cat /opt/togather/src/deploy/config/deployment-state.json | jq -r ".current_deployment.active_slot"'

# Check active slot from Caddy config
ssh deploy@<server> 'grep "reverse_proxy localhost:" /etc/caddy/Caddyfile'

# Check active slot from HTTP header
curl -I https://<domain>/health | grep X-Togather-Slot

# List all environment files
ssh deploy@<server> 'ls -la /opt/togather/.env*'

# Check image tags
ssh deploy@<server> 'docker images | grep togather-server'
```

---
