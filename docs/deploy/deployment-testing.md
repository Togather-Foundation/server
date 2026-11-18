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

# 2. Validate the reviewed tree before pushing/merging
scripts/agent-run.sh make ci
scripts/agent-run.sh make test-ci-race

# 3. Deploy the exact commit (auto-loads .deploy.conf if available)
scripts/agent-run.sh ./deploy/scripts/deploy.sh staging --version HEAD

# 4. Verify the deployed commit and save its evidence report
./deploy/scripts/verify-deployment.sh staging \
  --expected-commit "$(git rev-parse HEAD)" > /tmp/staging-verification.json
```

See `docs/deploy/deploy-conf.md` for complete .deploy.conf documentation.

Run the full CI and race gates sequentially. The integration suites currently
reuse database containers, so simultaneous gate processes can reset each other's
fixtures (`t_7af2cb9b`).

### One-command deployment verification

`verify-deployment.sh` reads the host, user and domain from
`.deploy.conf.<environment>`. It emits one JSON report on stdout and a short
summary on stderr. A failed required check produces a nonzero exit status;
inspect the report rather than treating successful command execution as proof
that all evidence was present.

Run it with Bash, Python 3, curl and SSH available locally, and Python 3 plus the
existing deployment tools available on the host. The smoke suite also uses jq.

The report checks the expected full commit and slot against HTTPS, the active
container, deployment state and history. It also checks container health, the
stopped fallback and its image, the released deployment lock, clean migrations,
the deployment's backup evidence and checksum, source-sync evidence and source
audit, the existing smoke suite, and rollback-preview readiness. It compares
deployment-state hashes around the preview and rechecks identity at the end.

This requires a recorded blue/green deployment with a retained previous version,
its normal pre-deploy backup, and the current deployment's sync log. Missing
evidence fails verification; the command does not resync sources or manufacture
a replacement backup. It uses `ADMIN_API_KEY` or `TOGATHER_ADMIN_API_KEY` for the
optional admin probe, falling back to `.agent-keys/<environment>` when available.
No credential values are included in the report.

Light read-only load testing is opt-in on staging:

```bash
./deploy/scripts/verify-deployment.sh staging \
  --expected-commit <deployed-full-commit-sha> --load-test \
  > /tmp/staging-verification.json
```

Production load testing is refused. Optional checks appear as explicit skips
when not requested or when their required optional credential is absent. The
verifier performs no actual rollback, database restore, fixture ingestion or
destructive cleanup. Rollback preview can write a deployment log; it must not
change deployment state or containers. Backup verification confirms the recorded
full archive-read result and current size/checksum, not a restore rehearsal.

Use the reviewed **deployed application commit** as `--expected-commit` when
checking an existing rollout. A later tooling-only commit does not require an
application rebuild or deployment. Keep the report with the kanban deployment
handoff; the verifier does not post it automatically.

Before building, check host disk/inode capacity and confirm the active image can
run `source preflight`. For an older image or Compose credential-interpolation
failure, use the [manual rollout instructions](remote-deployment.md#environment-preparation-for-a-manual-rollout)
instead of bypassing a gate. The app's runtime database URL and the host migration
URL can legitimately differ.

Record the pushed commit and served commit/slot, CI/review results, private backup
path/size/SHA-256/full-read result, migration version, source-sync result, final
source-audit counts, post-deploy test exit status, and remaining disk space in the
ticket handoff. Distinguish a verified archive read from an actual restore rehearsal.
Stop old writers/slots as required by the deployment or repair procedure, remove
temporary rollout files, and report unfinished checks or background tasks explicitly.
Do not redeploy solely to land documentation changes after a verified code rollout;
record that the served code commit precedes the documentation commit.

---

## Pre-Deployment Checks

### Deployment Configuration

- [ ] **Deployment config exists**
  ```bash
  # Check if .deploy.conf.{environment} exists
  ls -l .deploy.conf.staging
  source .deploy.conf.staging
  printf 'Domain: %s; SSH: %s@%s\n' "$NODE_DOMAIN" "$SSH_USER" "$SSH_HOST"
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
  > Runs all test suites without the race detector for fast feedback.
  > Use `make test-ci-race` for full CI parity with race detection (~10min).

- [ ] **Linter passes**
  ```bash
  make lint
  ```

- [ ] **Deployment package builds**
  ```bash
  make deploy-package
  ls -lh dist/togather-*.tar.gz
  ```

### Deployment tooling regression tests

```bash
scripts/agent-run.sh make test-tooling
```

Runs hermetic suites for pre-build configuration checks, snapshot safety, source
sync, read-only smoke checks, and performance-probe behavior. They use temporary
fixtures and command stubs; the real Compose interpolation fixture validates only
configuration, without starting services. No live database, Docker daemon, SSH
connection or deployment is required. Run this when changing deployment tooling;
live post-deploy checks below remain separate.

### Database backup readiness

Before building, `deploy.sh` validates Compose interpolation using the selected
environment file and requires exactly one running database service. With migrations
enabled, it checks the resolved PostgreSQL target and required backup tools too.
Check that an invalid configuration fails without invoking the image build or
creating a backup. Neither resolved Compose output nor connection secrets belong
in logs. The post-build backup must rediscover the target and service and reject
changes since preflight.

`deploy.sh` enforces a private, fully read custom-format backup before migrations.
Check the `.dump.json` evidence: exact artifact path, size, SHA-256, client mode and
`verification: pg_restore_full_read`. It uses the same resolved `DATABASE_URL` as
migrations; loopback targets require host clients, while shared network targets use
the configured database container. Failures stop before migrations/traffic switch.
`--skip-migrations` is an explicit no-migration/no-automatic-backup path; it still
requires valid Compose configuration and one running database service.

Standalone repairs still require the operator to run the
[verified backup helper](remote-deployment.md#snapshot-tooling-and-database-container-fallback)
first. Archive catalog listing alone is insufficient, and full archive reading does
not prove an isolated restore succeeds. Routine readiness checks must not create
fixtures or mutate the target database.

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
  ssh deploy@<server> 'test -e /tmp/togather-deploy-staging.lock && echo "LOCKED" || echo "OK"'
  # Should output "OK" (no lock directory); use the matching environment suffix.
  ```
  Also require `.lock.locked` to be false in deployment state. The verifier
  checks both forms of lock evidence at the beginning and end of its run.

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

**Last Updated:** 2026-02-20
