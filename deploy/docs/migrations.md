# Database Migration Troubleshooting Guide

Comprehensive guide for diagnosing and resolving database migration issues in Togather server deployments.

## Overview

The Togather deployment system uses [golang-migrate](https://github.com/golang-migrate/migrate) for managing database schema changes. Migrations run automatically during deployment, with automatic snapshots created before each migration for rollback safety.

### Migration Workflow

1. **Pre-deployment snapshot** - Automatic backup created before migrations
2. **Migration lock** - Prevents concurrent migrations
3. **Version check** - Validates current migration state
4. **Migration execution** - Applies pending migrations
5. **Version verification** - Confirms successful migration
6. **Lock release** - Removes migration lock

## Quick Diagnosis

### Check Migration Status

```bash
# Get current migration version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# Expected output:
# 20260128001_initial_schema (clean)
```

### Check for Dirty State

```bash
# Query schema_migrations table
psql "$DATABASE_URL" -c "SELECT * FROM schema_migrations;"

# Dirty state indicates failed migration:
# version | dirty
# --------|-------
# 3       | true   ← Database needs manual intervention
```

### View Recent Snapshots

```bash
cd deploy/scripts
./snapshot-db.sh list

# Output shows available restore points:
# Snapshot: togather_production_20260128_143022.sql.gz
#   Created: 2026-01-28 14:30:22
#   Size: 42.5 MB
#   Age: 2 hours ago
```

## Common Issues

### Issue 1: Migration Lock Exists

**Symptom:**
```
[ERROR] Migration lock exists (PID: 12345)
[ERROR] Another migration may be in progress
```

**Cause:** Previous migration process crashed or is still running

**Diagnosis:**
```bash
# Check if process is running
ps -p 12345

# Check lock file
cat /tmp/togather-migration-production.lock
```

**Resolution:**

**If process is still running:**
```bash
# Wait for migration to complete
# Monitor progress in deployment logs
tail -f /var/log/togather/deployments/<deployment-id>.log
```

**If process crashed (stale lock):**
```bash
# Verify no migration running
ps aux | grep migrate

# Remove stale lock
rm /tmp/togather-migration-production.lock

# Retry deployment
cd deploy/scripts
./deploy.sh production
```

---

### Issue 2: Dirty Migration State

**Symptom:**
```
[ERROR] Database is in dirty migration state
[ERROR] A previous migration failed and left the database in an inconsistent state
```

**Cause:** Migration failed mid-execution, leaving database in partial state

**Diagnosis:**
```bash
# Check migration version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# Output shows dirty state:
# 20260128003_add_user_table (dirty)
```

**Resolution:**

**Option 1: Restore from snapshot (RECOMMENDED)**
```bash
# List available snapshots
cd deploy/scripts
./snapshot-db.sh list

# Restore most recent snapshot (before failed migration)
# WARNING: This will restore database to state before migration
psql "$DATABASE_URL" < /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz

# Verify restoration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Should show clean state

# Fix the problematic migration file
nano internal/storage/postgres/migrations/20260128003_add_user_table.up.sql

# Retry deployment
./deploy.sh production
```

**Option 2: Force clean state (if you understand the issue)**
```bash
# Force migration to clean state at version 3
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force 3

# Manually fix database to match expected schema at version 3
psql "$DATABASE_URL" < fix_script.sql

# Retry migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up
```

**Option 3: Rollback and reapply**
```bash
# Force clean, then roll back one version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force 3
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# Fix the migration file
# Reapply
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up
```

---

### Issue 3: Migration Failed

**Symptom:**
```
[ERROR] MIGRATION FAILED
[ERROR] Database migrations encountered an error.
[ERROR] The database may be in an inconsistent state.
```

**Cause:** SQL syntax error, constraint violation, or incompatible schema change

**Diagnosis:**
```bash
# Check deployment logs for error details
tail -100 /var/log/togather/deployments/<deployment-id>.log | grep -A10 "MIGRATION FAILED"

# Check migrate CLI output
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up 2>&1 | tee migration_error.log

# Common errors:
# - Syntax error: Check SQL in .up.sql file
# - Duplicate column: Column already exists
# - Foreign key violation: Referenced table doesn't exist
# - Type mismatch: Incompatible data types
```

**Resolution:**

**Step 1: Restore from snapshot**
```bash
# Find snapshot created before failed migration
cd deploy/scripts
./snapshot-db.sh list

# Restore (replaces current database state)
psql "$DATABASE_URL" < /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz
```

**Step 2: Fix the migration**
```bash
# Locate the failed migration
ls -la internal/storage/postgres/migrations/

# Example: 20260128003_add_user_table.up.sql
# Edit to fix the issue
nano internal/storage/postgres/migrations/20260128003_add_user_table.up.sql

# Common fixes:
# - Add IF NOT EXISTS to CREATE TABLE
# - Add conditional checks for ALTER TABLE
# - Fix SQL syntax errors
# - Ensure foreign key tables exist
```

**Step 3: Test migration locally**
```bash
# Test on development database first
DATABASE_URL="postgresql://user:pass@localhost:5432/togather_dev" \
  migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up

# Verify schema is correct
psql "$DATABASE_URL" -c "\dt"  # List tables
psql "$DATABASE_URL" -c "\d+ users"  # Describe table
```

**Step 4: Redeploy**
```bash
cd deploy/scripts
./deploy.sh production
```

---

### Issue 4: Version Mismatch

**Symptom:**
```
[WARN] Migration version in database (5) does not match expected version (4)
```

**Cause:** Database has migrations applied that don't exist in current codebase

**Diagnosis:**
```bash
# Check migration version in database
psql "$DATABASE_URL" -c "SELECT version FROM schema_migrations;"

# List migration files in codebase
ls -1 internal/storage/postgres/migrations/*.up.sql | wc -l

# Database version > file count indicates missing migrations
```

**Resolution:**

**If database is ahead (database version > codebase):**
```bash
# Option 1: Sync codebase to match database
git fetch
git checkout <commit-with-latest-migrations>

# Option 2: Rollback database to match codebase
# WARNING: This may cause data loss
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down <n>
```

**If codebase is ahead (codebase > database version):**
```bash
# This is normal - just run migrations
cd deploy/scripts
./deploy.sh production
```

---

### Issue 5: Concurrent Migration Detected

**Symptom:**
```
[ERROR] Migration lock exists (PID: 67890)
[ERROR] Another migration may be in progress
```

**Cause:** Two deployments triggered simultaneously

**Diagnosis:**
```bash
# Check if multiple deploy.sh processes running
ps aux | grep deploy.sh

# Check deployment lock
cat /tmp/togather-deploy-production.lock

# Check migration lock
cat /tmp/togather-migration-production.lock
```

**Resolution:**

**Never force concurrent migrations** - this can corrupt the database

```bash
# Wait for first deployment to complete
tail -f /var/log/togather/deployments/<first-deployment-id>.log

# After first deployment finishes, retry second deployment
cd deploy/scripts
./deploy.sh production
```

**If deployment crashed and left stale lock:**
```bash
# Verify no processes running
ps aux | grep -E "(deploy\.sh|migrate)"

# Remove stale locks
rm /tmp/togather-deploy-production.lock
rm /tmp/togather-migration-production.lock

# Retry
./deploy.sh production
```

---

### Issue 6: Snapshot Creation Failed

**Symptom:**
```
[ERROR] Failed to create database snapshot
[ERROR] pg_dump exited with code 1
```

**Cause:** Insufficient disk space, permissions, or database connectivity

**Diagnosis:**
```bash
# Check disk space
df -h /var/lib/togather/db-snapshots

# Check permissions
ls -la /var/lib/togather/db-snapshots

# Test pg_dump manually
pg_dump -Fc -Z9 "$DATABASE_URL" > /tmp/test_dump.sql.gz
echo $?  # Should be 0 for success
```

**Resolution:**

**If disk space insufficient:**
```bash
# Clean up old snapshots
cd deploy/scripts
./snapshot-db.sh --cleanup

# Or manually delete old snapshots
rm /var/lib/togather/db-snapshots/togather_production_20260120_*.sql.gz
```

**If permissions issue:**
```bash
# Fix directory permissions
sudo chown -R $(whoami):$(whoami) /var/lib/togather/db-snapshots
chmod 755 /var/lib/togather/db-snapshots
```

**If database connectivity issue:**
```bash
# Test database connection
psql "$DATABASE_URL" -c "SELECT 1"

# Check database credentials
echo "$DATABASE_URL" | sed 's/:[^:@]*@/:****@/'  # Sanitized output
```

---

## Advanced Troubleshooting

### Manually Inspect Migration Files

```bash
# List all migrations
ls -la internal/storage/postgres/migrations/

# Example structure:
# 20260128001_initial_schema.up.sql    ← Forward migration
# 20260128001_initial_schema.down.sql  ← Rollback migration
# 20260128002_add_users.up.sql
# 20260128002_add_users.down.sql
```

**Validate migration SQL syntax:**
```bash
# Use psql to check syntax without executing
psql "$DATABASE_URL" --echo-errors < internal/storage/postgres/migrations/20260128003_add_user_table.up.sql --single-transaction --dry-run
```

### Check Migration Lock Internals

```bash
# View lock details
cat /tmp/togather-migration-production.lock

# Output: PID of process holding lock
# 12345

# Check if process is alive
ps -p 12345 -o pid,comm,start,etime

# If process doesn't exist, lock is stale
```

### Query Schema Migrations Table

```bash
# View migration history
psql "$DATABASE_URL" << 'SQL'
SELECT version, dirty, 
       to_char(to_timestamp(version), 'YYYY-MM-DD HH24:MI:SS') as applied_at
FROM schema_migrations
ORDER BY version DESC;
SQL

# Check for dirty migrations
psql "$DATABASE_URL" -c "SELECT * FROM schema_migrations WHERE dirty = true;"
```

### Simulate Migration Dry-Run

```bash
# Start transaction, run migration, but rollback
psql "$DATABASE_URL" << 'SQL'
BEGIN;
\i internal/storage/postgres/migrations/20260128003_add_user_table.up.sql
\dt  -- List tables to see changes
ROLLBACK;  -- Undo everything
SQL
```

### Restore Specific Snapshot

```bash
# List all snapshots with details
cd deploy/scripts
./snapshot-db.sh list

# Restore specific snapshot (DESTRUCTIVE - replaces current database)
psql "$DATABASE_URL" < /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz

# Verify restoration success
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM events;"
```

## Prevention Best Practices

### 1. Test Migrations Locally First

```bash
# Always test on development database before production
DATABASE_URL="postgresql://localhost/togather_dev" \
  migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" up

# Verify schema changes
psql "$DATABASE_URL" -c "\dt"
psql "$DATABASE_URL" -c "\d+ table_name"
```

### 2. Use Transactional Migrations

Add to the top of `.up.sql` files:
```sql
-- +migrate Up
-- +migrate StatementBegin
BEGIN;
-- Your migration SQL here
COMMIT;
-- +migrate StatementEnd
```

### 3. Write Idempotent Migrations

Use conditional checks:
```sql
-- Safe: Creates table only if it doesn't exist
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

-- Safe: Adds column only if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name='users' AND column_name='email'
    ) THEN
        ALTER TABLE users ADD COLUMN email TEXT;
    END IF;
END $$;
```

### 4. Always Write Down Migrations

Every `.up.sql` should have a corresponding `.down.sql`:
```bash
# Good migration pair:
20260128003_add_user_table.up.sql
20260128003_add_user_table.down.sql
```

### 5. Review Migration Logs

```bash
# Check recent deployment logs for migration details
tail -100 /var/log/togather/deployments/$(ls -t /var/log/togather/deployments/ | head -1) | grep -i migration
```

### 6. Monitor Snapshot Disk Usage

```bash
# Check snapshot directory size
du -sh /var/lib/togather/db-snapshots

# Set up automatic cleanup
cd deploy/scripts
./snapshot-db.sh --cleanup  # Removes snapshots older than 7 days
```

## Emergency Procedures

### Complete Database Restore

**WARNING: This replaces the entire database. Only use in disaster scenarios.**

```bash
# 1. Stop application to prevent new writes
docker-compose down

# 2. Find appropriate snapshot
cd deploy/scripts
./snapshot-db.sh list

# 3. Restore snapshot (DESTRUCTIVE)
psql "$DATABASE_URL" < /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz

# 4. Verify migration state
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# 5. If needed, manually force migration version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force <version>

# 6. Restart application
docker-compose up -d

# 7. Check health
curl http://localhost:8080/health | jq '.checks.migrations'
```

### Reset Migration State (DANGEROUS)

**Last resort only. This can cause data loss.**

```bash
# 1. Backup current database
pg_dump -Fc -Z9 "$DATABASE_URL" > /tmp/emergency_backup_$(date +%s).sql.gz

# 2. Drop and recreate schema_migrations table
psql "$DATABASE_URL" << 'SQL'
DROP TABLE IF EXISTS schema_migrations;
CREATE TABLE schema_migrations (
    version BIGINT NOT NULL,
    dirty BOOLEAN NOT NULL,
    PRIMARY KEY (version)
);
SQL

# 3. Manually set migration version
# Find the actual schema version by inspecting database
psql "$DATABASE_URL" -c "\dt"  # Check which tables exist

# Insert correct version
psql "$DATABASE_URL" << 'SQL'
INSERT INTO schema_migrations (version, dirty) VALUES (<actual_version>, false);
SQL

# 4. Verify
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
```

## Reference Commands

### Migration Management

```bash
# Check current version
migrate -path <migrations_dir> -database "$DATABASE_URL" version

# Apply all pending migrations
migrate -path <migrations_dir> -database "$DATABASE_URL" up

# Apply specific number of migrations
migrate -path <migrations_dir> -database "$DATABASE_URL" up <n>

# Rollback one migration
migrate -path <migrations_dir> -database "$DATABASE_URL" down 1

# Force version (clears dirty state)
migrate -path <migrations_dir> -database "$DATABASE_URL" force <version>

# Create new migration
migrate create -ext sql -dir <migrations_dir> -seq <migration_name>
```

### Snapshot Management

```bash
# List all snapshots
cd deploy/scripts
./snapshot-db.sh list

# Create manual snapshot
./snapshot-db.sh --reason "before_risky_change"

# Clean up expired snapshots
./snapshot-db.sh --cleanup

# Restore snapshot (manual)
psql "$DATABASE_URL" < /var/lib/togather/db-snapshots/<snapshot_file>
```

### Health Checks

```bash
# Check migration status via health endpoint
curl http://localhost:8080/health | jq '.checks.migrations'

# Expected healthy response:
{
  "status": "pass",
  "message": "Migrations applied successfully (version 5)",
  "latency_ms": 12,
  "details": {
    "version": 5,
    "dirty": false
  }
}
```

## Support

If migrations continue to fail after following this guide:

1. **Check deployment logs:** `/var/log/togather/deployments/<deployment-id>.log`
2. **Review migration files:** `internal/storage/postgres/migrations/`
3. **Inspect database state:** `psql "$DATABASE_URL" -c "\dt"`
4. **Consult specification:** `specs/001-deployment-infrastructure/spec.md`
5. **Test locally first:** Use development database to reproduce issue

## Related Documentation

- **Deployment Guide:** `deploy/README.md`
- **Architecture:** `deploy/ARCHITECTURE.md`
- **Snapshot Script:** `deploy/scripts/snapshot-db.sh`
- **Deploy Script:** `deploy/scripts/deploy.sh`
- **Health Checks:** `internal/api/handlers/health.go`

---

**Version:** 1.0.0  
**Last Updated:** 2026-01-28  
**Applies To:** Togather Server Deployment System Phase 1
