# Deployment Testing Notes

## Test Date: 2026-02-02

### Test Environment
- **Server**: 192.46.222.199 (Linode, Ubuntu 22.04)
- **Package**: `togather-server-74e36b9.tar.gz` (23MB)
- **Environment**: staging
- **Method**: `sudo ./install.sh`

### Test Results: ⚠️ PARTIAL SUCCESS

**What Worked:**
- ✅ Package build and transfer
- ✅ Binary installation (`/usr/local/bin/togather-server`)
- ✅ File deployment to `/opt/togather`
- ✅ Environment file generation (`.env` with all secrets)
- ✅ Systemd service creation and startup
- ✅ Makefile fix (bundled `./migrate` binary found correctly)
- ✅ Manual migration execution successful (16 migrations applied)
- ✅ Server responds and serves HTTP
- ✅ Database connectivity working
- ✅ JSON-LD context validation passing

**What Failed:**
- ❌ Automated installation hung on health check (never timed out)
- ❌ PostgreSQL not started automatically (Docker permission issues)
- ❌ Migrations not run automatically
- ❌ Server unhealthy until manual intervention

### Critical Bugs Discovered

#### Bug #1: Health Check Loop Hangs
**Location**: `install.sh` lines ~580-595

**Problem**: The health check loop runs 30 iterations but never times out or exits:
```bash
for i in {1..30}; do
    if togather-server healthcheck >> "$LOG_FILE" 2>&1; then
        log "  ✓ Server is healthy!"
        HEALTH_OK=true
        break
    fi
    if [[ $i -eq 30 ]]; then
        log "  ⚠️  Health check timed out"
        HEALTH_OK=false
    fi
    sleep 1
done
```

**Root Cause**: 
- `togather-server healthcheck` may hang waiting for input
- No explicit break on timeout iteration
- No timeout wrapper around healthcheck command

**Fix**:
```bash
TIMEOUT=30
for i in {1..${TIMEOUT}}; do
    if timeout 5 togather-server healthcheck >> "$LOG_FILE" 2>&1; then
        log "  ✓ Server is healthy!"
        HEALTH_OK=true
        break
    fi
    echo -n "."  # Progress indicator
    if [[ $i -eq ${TIMEOUT} ]]; then
        log "  ⚠️  Health check timed out after ${TIMEOUT}s"
        HEALTH_OK=false
        break  # Explicit break!
    fi
    sleep 1
done
```

#### Bug #2: PostgreSQL Never Starts
**Location**: `install.sh` Step 4 (Configure Environment)

**Problem**: Script logs "✓ Docker PostgreSQL started" but PostgreSQL never actually started.

**Root Cause**:
- `deploy` user not in `docker` group
- `togather-server setup` detects this and prints instructions but exits successfully
- `install.sh` doesn't verify PostgreSQL actually started

**Evidence from logs**:
```
⚠️  Docker permission issue detected
After fixing permissions, start Docker with:
  docker compose -f deploy/docker/docker-compose.yml up -d

✓ Docker PostgreSQL started  <-- FALSE!
```

**Fix**:
```bash
# Pre-check before running setup
if ! groups "$INSTALL_USER" | grep -q docker; then
    error_exit "User $INSTALL_USER must be in 'docker' group. Run: sudo usermod -aG docker $INSTALL_USER"
fi

# After server setup, explicitly verify and start PostgreSQL
cd "${APP_DIR}"
if ! sudo -u "${INSTALL_USER}" docker compose -f deploy/docker/docker-compose.yml --env-file .env ps | grep -q togather-db; then
    log "  → Starting PostgreSQL..."
    if ! sudo -u "${INSTALL_USER}" docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d togather-db >> "$LOG_FILE" 2>&1; then
        error_exit "Failed to start PostgreSQL container"
    fi
fi

# Wait for PostgreSQL to be ready
log "  → Waiting for PostgreSQL..."
for i in {1..30}; do
    if sudo docker exec togather-db pg_isready -U togather &>/dev/null; then
        log "  ✓ PostgreSQL ready"
        break
    fi
    [[ $i -eq 30 ]] && error_exit "PostgreSQL failed to become ready"
    sleep 1
done
```

#### Bug #3: Migrations Never Run
**Location**: `togather-server setup` command

**Problem**: Migrations are supposed to run automatically during `server setup` but don't.

**Root Cause**: PostgreSQL wasn't running when `server setup` tried to run migrations.

**Evidence**:
```bash
$ ./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
error: no migration
```

**Fix**: After fixing Bug #2, migrations should run. Add verification:
```bash
# After togather-server setup completes
cd "${APP_DIR}"
source .env
MIGRATION_VERSION=$(./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version 2>&1)
if [[ ! "$MIGRATION_VERSION" =~ ^[0-9]+$ ]]; then
    log "  ⚠️  Migrations not applied, running now..."
    if ! ./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up >> "$LOG_FILE" 2>&1; then
        error_exit "Migration failed"
    fi
    log "  ✓ Migrations completed"
fi
```

#### Bug #4: Docker Compose Environment File Path
**Location**: `deploy/docker/docker-compose.yml`

**Problem**: The compose file uses `env_file: - ../../.env` which doesn't work when called with `-f` flag.

**Fix**: Use `--env-file` flag explicitly when calling `docker compose`:
```bash
docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d
```

#### Bug #5: Persistent Volume Password Mismatch
**Location**: Docker volumes from previous installation

**Problem**: Old volumes persist across installations with different passwords, causing authentication failures.

**Error**:
```
FATAL: password authentication failed for user "togather"
```

**Fix**: Detect and handle existing volumes in install.sh:
```bash
if sudo docker volume ls | grep -q togather-db-data; then
    log "  ⚠️  Existing database volumes detected"
    log "     These may contain data with different credentials."
    log ""
    read -p "  Remove existing volumes and start fresh? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log "  → Removing existing volumes..."
        sudo docker compose -f "${APP_DIR}/deploy/docker/docker-compose.yml" down -v >> "$LOG_FILE" 2>&1
        log "  ✓ Volumes removed"
    else
        log "  ⚠️  Keeping existing volumes. You may need to update passwords manually."
    fi
fi
```

### Manual Recovery Steps Performed

After the automated installation hung, these manual steps were required:

```bash
# 1. Remove old volumes with mismatched passwords
ssh togather 'sudo docker compose -f /opt/togather/deploy/docker/docker-compose.yml --env-file /opt/togather/.env down -v'

# 2. Start fresh PostgreSQL with new credentials
ssh togather 'cd /opt/togather && sudo docker compose -f deploy/docker/docker-compose.yml --env-file .env up -d togather-db'

# 3. Run migrations manually
ssh togather 'cd /opt/togather && source .env && ./migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up'

# 4. Attempt River migrations (optional, requires river CLI)
ssh togather 'cd /opt/togather && source .env && make migrate-river'
# Note: River CLI not bundled, migrations fail (non-critical)

# 5. Verify health
ssh togather 'togather-server healthcheck'
# Result: DEGRADED (acceptable, River optional)
```

### Final Server State

**Status**: ✅ **OPERATIONAL (DEGRADED)**

```json
{
  "status": "degraded",
  "version": "dev",
  "checks": {
    "database": {"status": "pass"},
    "http_endpoint": {"status": "pass"},
    "job_queue": {"status": "warn", "message": "River job queue table not found"},
    "jsonld_contexts": {"status": "pass"},
    "migrations": {"status": "pass", "version": 16}
  }
}
```

**Services Running:**
- ✅ systemd service: `active (running)`
- ✅ PostgreSQL container: `healthy`
- ✅ HTTP server: responding on port 8080
- ✅ Database: 16 migrations applied
- ⚠️ Job queue: River migrations not run (optional)

### Recommendations

**Priority 1 - MUST FIX BEFORE NEXT DEPLOYMENT:**
1. Fix health check loop timeout logic
2. Add Docker group pre-check
3. Explicitly start and verify PostgreSQL
4. Verify migrations ran successfully
5. Use `--env-file` flag with docker compose

**Priority 2 - SHOULD FIX:**
6. Bundle River CLI or make it truly optional
7. Handle persistent volumes gracefully
8. Add more verbose progress indicators during installation
9. Improve error messages with specific remediation steps

**Priority 3 - NICE TO HAVE:**
10. Add dry-run mode for testing
11. Add rollback on failure
12. Add installation resume capability

### Testing Checklist for Next Deployment

Before testing next version of install.sh:

- [ ] Test on **completely clean server** (no Docker volumes)
- [ ] Verify `deploy` user is in `docker` group BEFORE installation
- [ ] Monitor `/var/log/togather-install.log` in real-time during installation
- [ ] Set timeout on SSH command: `timeout 300 ssh togather 'cd ... && sudo ./install.sh'`
- [ ] Have manual recovery commands ready
- [ ] Test with `ENVIRONMENT=staging` and `ENVIRONMENT=production`

### Lessons Learned

1. **Docker permissions are critical** - Must check before attempting any Docker operations
2. **Never assume success** - Verify every critical step actually worked
3. **Health check must timeout** - Use `timeout` command wrapper
4. **Persistent state is tricky** - Docker volumes survive container removal
5. **Orchestration != Verification** - Calling a command doesn't mean it succeeded
6. **Logs are essential** - `/var/log/togather-install.log` saved us

### Positive Outcomes

Despite the bugs, several things worked excellently:

1. ✅ **Makefile fix was correct** - Bundled `./migrate` binary found and used successfully
2. ✅ **Package structure is good** - All files present and correct
3. ✅ **Server setup works** - Generates valid `.env` with all required variables
4. ✅ **Migrations work** - All 16 migrations applied successfully
5. ✅ **Documentation is excellent** - MANUAL_INSTALL.md was crucial for recovery
6. ✅ **Installation report would be useful** - (if installation completed)

### Next Steps

1. Create bug tracking issue: `server-s3uu` ✅
2. Fix all Priority 1 bugs in `install.sh`
3. Test fixed version on clean server
4. Document any new issues found
5. Iterate until truly bulletproof

---

**Test completed by**: OpenCode Agent  
**Issue tracking**: `server-s3uu` - Fix install.sh critical bugs  
**Related epic**: `server-1mcr` - Bulletproof one-command installation
