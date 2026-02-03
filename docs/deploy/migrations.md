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
server snapshot list

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

# Check lock directory (uses atomic mkdir for locking)
ls -ld /tmp/togather-migration-production.lock
test -d /tmp/togather-migration-production.lock && echo "Lock exists"
```

**Resolution:**

**If process is still running:**
```bash
# Wait for migration to complete
# Monitor progress in deployment logs
tail -f ~/.togather/logs/deployments/<env>_<timestamp>.log
```

**If process crashed (stale lock):**
```bash
# Verify no migration running
ps aux | grep migrate

# Remove stale lock directory (uses POSIX-atomic mkdir for locking)
rmdir /tmp/togather-migration-production.lock
# Or use rm -rf if directory has unexpected contents
# rm -rf /tmp/togather-migration-production.lock

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
server snapshot list

# Restore most recent snapshot (before failed migration)
# WARNING: This will restore database to state before migration
gunzip -c /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz | psql "$DATABASE_URL"

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
tail -100 ~/.togather/logs/deployments/<env>_<timestamp>.log | grep -A10 "MIGRATION FAILED"

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
server snapshot list

# Restore (replaces current database state)
gunzip -c /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz | psql "$DATABASE_URL"
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

# Check deployment lock directory
test -d /tmp/togather-deploy-production.lock && echo "Deployment lock exists"

# Check migration lock directory
test -d /tmp/togather-migration-production.lock && echo "Migration lock exists"
```

**Resolution:**

**Never force concurrent migrations** - this can corrupt the database

```bash
# Wait for first deployment to complete
tail -f ~/.togather/logs/deployments/<env>_<timestamp>.log

# After first deployment finishes, retry second deployment
cd deploy/scripts
./deploy.sh production
```

**If deployment crashed and left stale lock:**
```bash
# Verify no processes running
ps aux | grep -E "(deploy\.sh|migrate)"

# Remove stale lock directories (atomic directory-based locking)
rmdir /tmp/togather-deploy-production.lock 2>/dev/null || rm -rf /tmp/togather-deploy-production.lock
rmdir /tmp/togather-migration-production.lock 2>/dev/null || rm -rf /tmp/togather-migration-production.lock

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
server snapshot cleanup --retention-days 7

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
# View lock directory (uses POSIX-atomic mkdir for locking)
ls -ld /tmp/togather-migration-production.lock

# Check if lock directory exists
test -d /tmp/togather-migration-production.lock && echo "Lock exists" || echo "No lock"

# Check process that created lock (PID from state file, not lock itself)
# Lock directory provides atomicity via mkdir operation
# Process info stored in deployment state file instead
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
server snapshot list

# Restore specific snapshot (DESTRUCTIVE - replaces current database)
gunzip -c /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz | psql "$DATABASE_URL"

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
tail -100 ~/.togather/logs/deployments/$(ls -t ~/.togather/logs/deployments/ | head -1) | grep -i migration
```

### 6. Monitor Snapshot Disk Usage

```bash
# Check snapshot directory size
du -sh /var/lib/togather/db-snapshots

# Set up automatic cleanup
server snapshot cleanup --retention-days 7  # Removes snapshots older than 7 days
```

## Emergency Procedures

### Complete Database Restore

**WARNING: This replaces the entire database. Only use in disaster scenarios.**

```bash
# 1. Stop application to prevent new writes
docker compose -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml down

# 2. Find appropriate snapshot
server snapshot list

# 3. Restore snapshot (DESTRUCTIVE)
gunzip -c /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz | psql "$DATABASE_URL"

# 4. Verify migration state
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# 5. If needed, manually force migration version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force <version>

# 6. Restart application
docker compose -f /opt/togather/src/deploy/docker/docker-compose.blue-green.yml up -d

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

## Migration Rollback Procedures

This section covers how to manually rollback database migrations when needed.

### When to Rollback Migrations

**Rollback migrations when:**
- A migration caused data corruption or loss
- Schema changes broke critical application functionality
- Need to deploy an older application version that requires older schema
- Migration ran successfully but application logic is incompatible

**DO NOT rollback migrations when:**
- Application deployment failed (migration may be fine)
- Can fix the issue with a new forward migration
- Data has been written that depends on new schema

### Understanding Migration Rollback

Each migration consists of two files:
- `YYYYMMDDHHMMSS_description.up.sql` - Applies changes
- `YYYYMMDDHHMMSS_description.down.sql` - Reverts changes

**Example:**
```
20260128001_create_users_table.up.sql    → Creates users table
20260128001_create_users_table.down.sql  → Drops users table
```

### Rollback Methods

#### Method 1: Automatic Rollback (Recommended)

Use `migrate` CLI to rollback migrations safely:

```bash
# Rollback last migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# Rollback multiple migrations (e.g., last 3)
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 3

# Rollback to specific version (e.g., version 5)
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" goto 5
```

**Verification:**
```bash
# Check current version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# Expected output:
# 5 (clean)  ← Successfully rolled back to version 5
```

---

#### Method 2: Manual Rollback with Snapshot Restore

When `.down.sql` files don't exist or are insufficient:

```bash
# 1. List available snapshots
server snapshot list

# Output example:
# Snapshot: togather_production_20260128_143022.sql.gz
#   Created: 2026-01-28 14:30:22 (before migration 20260128003)
#   Size: 42.5 MB

# 2. Stop application to prevent new writes
docker stop togather-production-<slot>

# 3. Restore snapshot
gunzip -c /var/lib/togather/db-snapshots/togather_production_20260128_143022.sql.gz | psql "$DATABASE_URL"

# 4. Verify migration version matches snapshot
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# Should show version from before the problematic migration

# 5. Restart application
docker start togather-production-<slot>

# 6. Verify health
curl http://localhost:8080/health | jq '.checks.migrations'
```

**Data Loss Warning:** This method restores the database to an earlier state. Any data written after the snapshot was created will be lost.

---

#### Method 3: Manual SQL Rollback

When you need fine-grained control:

```bash
# 1. Review the .down.sql file
cat internal/storage/postgres/migrations/20260128003_add_users.down.sql

# Example content:
# DROP TABLE IF EXISTS users CASCADE;

# 2. Create a backup first
pg_dump -Fc -Z9 "$DATABASE_URL" > /tmp/before_manual_rollback_$(date +%s).sql.gz

# 3. Execute the .down.sql in a transaction (safe)
psql "$DATABASE_URL" << 'SQL'
BEGIN;
\i internal/storage/postgres/migrations/20260128003_add_users.down.sql
-- Review changes
\dt
-- If everything looks good, commit
COMMIT;
-- If something is wrong, use ROLLBACK instead
SQL

# 4. Update schema_migrations table
psql "$DATABASE_URL" << 'SQL'
UPDATE schema_migrations SET version = <previous_version>, dirty = false;
SQL

# Replace <previous_version> with the version number before the rolled-back migration
```

---

### Rollback Scenarios

#### Scenario 1: Rollback Last Migration (Recent Deployment)

**Situation:** Just deployed, migration applied, but application is broken.

**Steps:**
```bash
# 1. Verify current version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Output: 8 (clean)

# 2. Rollback one migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# 3. Verify rollback
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Output: 7 (clean)

# 4. Redeploy old application version
cd deploy/scripts
server deploy rollback production  # This rolls back the application deployment
```

---

#### Scenario 2: Rollback Multiple Migrations

**Situation:** Need to rollback to a known-good state from several deployments ago.

**Steps:**
```bash
# 1. Identify target version
# Check deployment history to find last known-good version
cat deploy/config/deployment-state.json | jq '.deployments[-5]'

# 2. Check current migration version
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Output: 12 (clean)

# 3. Determine how many migrations to rollback
# If target version is 8, need to rollback 4 migrations (12 → 8)

# 4. Restore snapshot from that time (SAFER)
cd deploy/scripts
./snapshot-db.sh list | grep "20260128"  # Find snapshot near target date

gunzip -c /var/lib/togather/db-snapshots/togather_production_20260128_120000.sql.gz | psql "$DATABASE_URL"

# OR use migrate down (if .down.sql files exist and are safe)
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" goto 8

# 5. Deploy old application version
git checkout <target-commit>
cd deploy/scripts
./deploy.sh production
```

---

#### Scenario 3: Rollback with Data Preservation

**Situation:** Need to rollback schema but preserve data written after migration.

**Steps:**
```bash
# 1. Export new data to temporary location
psql "$DATABASE_URL" << 'SQL'
-- Create temp table with new data
CREATE TEMP TABLE backup_new_data AS
SELECT * FROM users WHERE created_at > '2026-01-28 14:00:00';

-- Export to file
\copy backup_new_data TO '/tmp/new_data.csv' CSV HEADER;
SQL

# 2. Rollback migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# 3. Re-import data (if compatible with old schema)
psql "$DATABASE_URL" << 'SQL'
\copy users FROM '/tmp/new_data.csv' CSV HEADER;
SQL

# Note: This only works if the old schema can accept the new data
# Often not possible if column names or types changed
```

---

#### Scenario 4: Dirty Migration State + Rollback

**Situation:** Migration failed mid-execution (dirty state), need to rollback.

**Steps:**
```bash
# 1. Check migration status
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Output: 8 (dirty)  ← Failed during migration 8

# 2. OPTION A: Force clean, then rollback
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force 8
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# 3. OPTION B: Restore snapshot (safer)
server snapshot list

gunzip -c /var/lib/togather/db-snapshots/togather_production_<timestamp>.sql.gz | psql "$DATABASE_URL"

# 4. Verify clean state
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version
# Output: 7 (clean)
```

---

### Rollback Best Practices

#### 1. Always Test Rollback Before Production

```bash
# Test on staging first
DATABASE_URL="postgresql://staging-db/togather" \
  migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1

# Verify application works with rolled-back schema
./deploy.sh staging

# Test critical functionality
curl https://staging.example.com/api/v1/events
```

#### 2. Create Snapshot Before Rollback

```bash
# Always create a "just before rollback" snapshot
server snapshot create --reason "before_rollback_migration_8"

# Now safe to rollback
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1
```

#### 3. Coordinate Application and Database Rollback

**Wrong Order (causes errors):**
```bash
# ❌ Rolling back migrations while new app version is running
# App expects new schema, but migration was rolled back → 500 errors
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1
```

**Correct Order:**
```bash
# ✅ Rollback application first, then migrations
cd deploy/scripts
server deploy rollback production  # Rollback app deployment

# Wait for health checks to pass with old app + new schema
sleep 30

# Then rollback migrations if needed
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1
```

#### 4. Document Rollback Reasons

```bash
# Record why rollback was needed
echo "$(date): Rolled back migration 8 due to data corruption in users table" >> \
  /var/log/togather/migration-rollbacks.log

# Include in deployment state
jq '.last_rollback = {
  "timestamp": "'$(date -Iseconds)'",
  "migration_version": 8,
  "reason": "data corruption in users table"
}' deploy/config/deployment-state.json > tmp.json && mv tmp.json deploy/config/deployment-state.json
```

#### 5. Verify Data Integrity After Rollback

```bash
# Run data validation queries
psql "$DATABASE_URL" << 'SQL'
-- Check for orphaned foreign keys
SELECT 'events' AS table, COUNT(*) AS orphaned
FROM events e
WHERE NOT EXISTS (SELECT 1 FROM organizations o WHERE o.id = e.organization_id)

UNION ALL

SELECT 'places' AS table, COUNT(*) AS orphaned
FROM places p
WHERE NOT EXISTS (SELECT 1 FROM organizations o WHERE o.id = p.organization_id);
SQL

# Expected output: All counts should be 0
```

---

### Writing Rollback-Safe Migrations

**Good Practices:**

```sql
-- ✅ Add column with default (can rollback safely)
-- up.sql
ALTER TABLE users ADD COLUMN status TEXT DEFAULT 'active';

-- down.sql
ALTER TABLE users DROP COLUMN IF EXISTS status;

-- ✅ Create new table (can rollback safely)
-- up.sql
CREATE TABLE IF NOT EXISTS user_preferences (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id),
  theme TEXT DEFAULT 'light'
);

-- down.sql
DROP TABLE IF EXISTS user_preferences CASCADE;

-- ✅ Add index (can rollback safely)
-- up.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_events_start_time ON events(start_time);

-- down.sql
DROP INDEX IF EXISTS idx_events_start_time;
```

**Dangerous Patterns:**

```sql
-- ❌ Dropping columns (data loss on rollback)
-- up.sql
ALTER TABLE users DROP COLUMN deprecated_field;

-- down.sql
-- Can't restore data!
ALTER TABLE users ADD COLUMN deprecated_field TEXT;

-- ✅ Better: Deprecate instead of drop
-- up.sql
-- Just stop using the column, drop in future migration after data archived
-- down.sql
-- No changes needed

-- ❌ Renaming tables (breaks rollback)
-- up.sql
ALTER TABLE users RENAME TO accounts;

-- down.sql
ALTER TABLE accounts RENAME TO users;  -- Breaks if "accounts" has new data

-- ✅ Better: Create new table, migrate data, keep old table for rollback window
-- up.sql
CREATE TABLE accounts AS SELECT * FROM users;
-- down.sql
DROP TABLE accounts;

-- ❌ Changing column types (data loss risk)
-- up.sql
ALTER TABLE users ALTER COLUMN age TYPE INTEGER USING age::INTEGER;

-- down.sql
ALTER TABLE users ALTER COLUMN age TYPE TEXT;  -- Data already converted, can't undo

-- ✅ Better: Add new column, migrate data, keep old column
-- up.sql
ALTER TABLE users ADD COLUMN age_int INTEGER;
UPDATE users SET age_int = age::INTEGER WHERE age ~ '^\d+$';
-- down.sql
ALTER TABLE users DROP COLUMN age_int;
```

---

### Rollback Checklist

Before rolling back migrations:

```bash
[ ] Snapshot created before rollback
[ ] Rollback tested on staging environment
[ ] Application deployment rolled back first (if applicable)
[ ] Team notified of rollback in progress
[ ] Rollback reason documented
[ ] Expected data loss understood and accepted
[ ] Rollback procedure reviewed and approved
[ ] On-call engineer available for incident response

# After rollback:
[ ] Migration version verified with: migrate version
[ ] Health checks passing: curl /health
[ ] Critical functionality tested manually
[ ] No error spikes in application logs
[ ] Database size appropriate (no unexpected data loss)
[ ] Deployment state updated
[ ] Post-mortem scheduled
```

---

### Troubleshooting Rollback Issues

#### Rollback Command Fails

**Error:**
```
error: Dirty database version 8. Fix and force version.
```

**Solution:**
```bash
# Force clean state, then retry rollback
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" force 8
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1
```

---

#### Foreign Key Constraint Violations

**Error:**
```
ERROR: update or delete on table "users" violates foreign key constraint
```

**Solution:**
```bash
# Rollback must be done in reverse dependency order
# Example: If events references users, rollback events first

# 1. Identify dependencies
psql "$DATABASE_URL" << 'SQL'
SELECT
  conname AS constraint_name,
  conrelid::regclass AS table_name,
  confrelid::regclass AS foreign_table
FROM pg_constraint
WHERE contype = 'f';
SQL

# 2. Rollback in correct order (children before parents)
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 2  # Rollback events migration
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" down 1  # Now safe to rollback users
```

---

#### .down.sql File Missing

**Error:**
```
error: file does not exist: migrations/20260128003_add_users.down.sql
```

**Solution:**
```bash
# Use snapshot restore instead
cd deploy/scripts
./snapshot-db.sh list

gunzip -c /var/lib/togather/db-snapshots/togather_production_<before_migration>.sql.gz | psql "$DATABASE_URL"

# Manually update schema_migrations table
psql "$DATABASE_URL" << 'SQL'
UPDATE schema_migrations SET version = <previous_version>, dirty = false;
SQL
```

---

### Recovery After Failed Rollback

If rollback fails and leaves database in bad state:

```bash
# 1. Restore from snapshot immediately
cd deploy/scripts
./snapshot-db.sh list

# Find snapshot from before the original migration
gunzip -c /var/lib/togather/db-snapshots/togather_production_<timestamp>.sql.gz | psql "$DATABASE_URL"

# 2. Verify database state
psql "$DATABASE_URL" -c "\dt"  # List tables
migrate -path internal/storage/postgres/migrations -database "$DATABASE_URL" version

# 3. If schema_migrations table is corrupted, reset it
# See "Emergency Procedures > Reset Migration State" section above

# 4. Deploy known-good application version
git checkout <last-known-good-commit>
cd deploy/scripts
./deploy.sh production

# 5. Create incident report and plan fix
```

---

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
server snapshot list

# Create manual snapshot
server snapshot create --reason "before_risky_change"

# Clean up expired snapshots
server snapshot cleanup --retention-days 7

# Restore snapshot (manual)
gunzip -c /var/lib/togather/db-snapshots/<snapshot_file>.sql.gz | psql "$DATABASE_URL"
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

1. **Check deployment logs:** `~/.togather/logs/deployments/<env>_<timestamp>.log`
2. **Review migration files:** `internal/storage/postgres/migrations/`
3. **Inspect database state:** `psql "$DATABASE_URL" -c "\dt"`
4. **Consult specification:** `specs/001-deployment-infrastructure/spec.md`
5. **Test locally first:** Use development database to reproduce issue

## Related Documentation

- **Deployment Guide:** `deploy/README.md`
- **Architecture:** `deploy/ARCHITECTURE.md`
- **Snapshot CLI:** `server snapshot`
- **Deploy Script:** `deploy/scripts/deploy.sh`
- **Health Checks:** `internal/api/handlers/health.go`

---

**Version:** 1.0.0  
**Last Updated:** 2026-01-28  
**Applies To:** Togather Server Deployment System Phase 1
