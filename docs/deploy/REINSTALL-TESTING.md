# Reinstallation Data Protection Testing Guide

This guide describes how to test the new backup/restore workflow that prevents data loss during reinstallation.

## Problem Being Solved

**Previous behavior**: Running `install.sh` on a system with existing installation would **unconditionally destroy all Docker volumes**, causing complete data loss.

**New behavior**: 
1. Detects existing installation and database volumes
2. Creates automatic backup before any destructive operations
3. Offers user three choices:
   - **Preserve data**: Upgrade in place, keep database intact
   - **Fresh install**: Delete everything (with explicit confirmation) after backup
   - **Abort**: Cancel installation

## Test Server Details

- **SSH alias**: `togather`
- **IP**: `192.46.222.199`
- **Installation directory**: `/opt/togather`
- **Binary location**: `/usr/local/bin/togather-server`

## Testing Procedure

### Prerequisites

1. Build the deployment package with the new changes:
   ```bash
   make deploy-package
   ```

2. Copy package to test server:
   ```bash
   scp ./dist/togather-server-*.tar.gz togather:~/
   ```

### Test Case 1: First-Time Installation (No Existing Data)

**Expected**: Installation proceeds normally without backup prompts.

```bash
ssh togather
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh
```

**Verify**:
- No backup creation messages
- No "EXISTING INSTALLATION DETECTED" warning
- Installation completes successfully
- Server is running: `togather-server healthcheck`

### Test Case 2: Reinstallation with Data Preservation

**Expected**: Backup created, option to preserve data, no data loss.

**Setup** (create some test data):
```bash
ssh togather
# Verify server is running
togather-server healthcheck

# Create test data (events, places, etc.)
# You can use the API or database directly
curl http://localhost:8080/api/v1/events # Check current data
```

**Test reinstallation**:
```bash
# Copy new package
scp ./dist/togather-server-*.tar.gz togather:~/

# SSH and reinstall
ssh togather
tar -xzf togather-server-*.tar.gz
cd togather-server-*/
sudo ./install.sh
```

**During installation**:
1. Should see: "⚠️  EXISTING INSTALLATION DETECTED"
2. Should see: "📦 Creating backup before proceeding..."
3. Should see backup location: `/opt/togather/backups/pre-reinstall-YYYYMMDD-HHMMSS.sql.gz`
4. Should see three options:
   - [1] PRESERVE DATA
   - [2] FRESH INSTALL
   - [3] ABORT
5. **Choose option 1** (PRESERVE DATA)

**Verify after installation**:
```bash
# Check backup was created
ls -lh /opt/togather/backups/
togather-server snapshot list --snapshot-dir /opt/togather/backups

# Verify data is preserved
curl http://localhost:8080/api/v1/events # Should show same data as before

# Check installation report
cat /opt/togather/installation-report.txt
# Should include "Backup & Restore" section with backup location

# Verify server health
togather-server healthcheck
```

### Test Case 3: Fresh Install with Data Destruction

**Expected**: Backup created, explicit confirmation required, volumes deleted.

**Test**:
```bash
ssh togather
cd togather-server-*/
sudo ./install.sh
```

**During installation**:
1. Should see backup creation
2. **Choose option 2** (FRESH INSTALL)
3. Should see: "⚠️⚠️⚠️  WARNING: THIS WILL DELETE ALL DATA ⚠️⚠️⚠️"
4. Should prompt: "Type 'DELETE ALL DATA' to confirm:"
5. Type exactly: `DELETE ALL DATA`
6. Should see: "→ Removing all volumes..."

**Verify after installation**:
```bash
# Backup should exist
ls -lh /opt/togather/backups/

# Data should be gone (new empty database)
curl http://localhost:8080/api/v1/events # Should show empty or new data

# New credentials generated
cat /opt/togather/.env | grep ADMIN_PASSWORD
```

### Test Case 4: Abort Installation

**Expected**: Installation cancelled, backup preserved, no changes made.

**Test**:
```bash
ssh togather
cd togather-server-*/
sudo ./install.sh
```

**During installation**:
1. Should see backup creation
2. **Choose option 3** (ABORT)
3. Should see: "✗ Installation aborted by user"
4. Should see backup location preserved

**Verify**:
```bash
# Backup exists but system unchanged
ls -lh /opt/togather/backups/
togather-server version # Should be old version still
```

### Test Case 5: Non-Interactive Mode

**Expected**: Defaults to PRESERVE DATA (safest option).

**Test**:
```bash
ssh togather
cd togather-server-*/
# Run with input redirected from /dev/null to simulate non-interactive
sudo ./install.sh < /dev/null
```

**Verify**:
- Should see: "ℹ️  Non-interactive mode: Defaulting to PRESERVE DATA (option 1)"
- Data should be preserved
- Backup should be created

### Test Case 6: Backup Failure Handling

**Expected**: Installation continues with warning, still offers user choice.

**Test** (simulate backup failure):
```bash
ssh togather
# Stop database to cause backup to fail
sudo docker stop togather-db

cd togather-server-*/
sudo ./install.sh
```

**Verify**:
- Should see: "⚠️  Warning: Backup creation failed"
- Should still see user options
- If user chooses FRESH INSTALL, should warn: "⚠️  Backup was not successful - data may be lost!"

## Success Criteria

All test cases should pass:

- ✅ First-time installation works without backup prompts
- ✅ Reinstallation with PRESERVE DATA keeps all data intact
- ✅ Reinstallation with FRESH INSTALL creates backup, requires explicit confirmation, deletes volumes
- ✅ ABORT option cancels installation safely
- ✅ Non-interactive mode defaults to PRESERVE DATA
- ✅ Backup failures are handled gracefully
- ✅ Installation report includes backup location and restore instructions
- ✅ Backup directory (`/opt/togather/backups/`) is created and contains backups
- ✅ No data loss occurs without explicit user confirmation

## Rollback

If testing reveals issues, you can restore the previous version:

```bash
ssh togather
# List available backups
togather-server snapshot list --snapshot-dir /opt/togather/backups

# Restore from backup (if needed)
# Note: You'll need to implement snapshot restore command, or use pg_restore directly
```

## Notes

- All test cases should be run on the test server first before deploying to production
- Each test should be documented with screenshots or log output
- Pay special attention to the user prompts and confirmation messages
- Verify that backups are actually created and contain valid data
- Test with both interactive and non-interactive scenarios
